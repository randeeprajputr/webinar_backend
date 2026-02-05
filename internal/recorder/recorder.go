package recorder

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v3"
	"go.uber.org/zap"

	"github.com/aura-webinar/backend/internal/realtime"
)

const (
	// RTP payload types we use in the SDP sent to ffmpeg (must match rewrite in WriteRTP).
	payloadTypeVideo = 96
	payloadTypeAudio = 97
	// Default max recording duration (2 hours).
	defaultMaxDurationSec = 7200
)

// Session represents an active recording session for one webinar.
type Session struct {
	webinarID   uuid.UUID
	recordingID uuid.UUID
	outputPath  string
	sdpPath     string
	cmd         *exec.Cmd
	videoConn   *net.UDPConn
	audioConn   *net.UDPConn
	videoAddr   *net.UDPAddr
	audioAddr   *net.UDPAddr
	mu          sync.Mutex
	log         *zap.Logger
}

// Sink implements realtime.RecordingSink by sending RTP to ffmpeg's UDP ports.
type Sink struct {
	session *Session
}

// WriteRTP sends a copy of the RTP packet to ffmpeg (rewriting payload type to match SDP).
func (s *Sink) WriteRTP(kind webrtc.RTPCodecType, packet []byte) {
	if len(packet) < 2 {
		return
	}
	s.session.mu.Lock()
	defer s.session.mu.Unlock()
	pt := byte(payloadTypeVideo)
	if kind == webrtc.RTPCodecTypeAudio {
		pt = payloadTypeAudio
	}
	// Rewrite payload type (lower 7 bits of second byte).
	rewritten := make([]byte, len(packet))
	copy(rewritten, packet)
	rewritten[1] = (packet[1] & 0x80) | pt

	var conn *net.UDPConn
	var addr *net.UDPAddr
	if kind == webrtc.RTPCodecTypeVideo {
		conn, addr = s.session.videoConn, s.session.videoAddr
	} else {
		conn, addr = s.session.audioConn, s.session.audioAddr
	}
	if conn != nil && addr != nil {
		_, _ = conn.WriteToUDP(rewritten, addr)
	}
}

// Service starts and stops recording sessions (tap into SFU publisher stream).
type Service struct {
	sfu       *realtime.SFU
	outputDir string
	maxDurSec int
	log       *zap.Logger
	mu        sync.Mutex
	sessions  map[uuid.UUID]*Session
}

// NewService creates a recording service that uses the SFU to tap RTP and ffmpeg to mux.
func NewService(sfu *realtime.SFU, outputDir string, log *zap.Logger) *Service {
	if outputDir == "" {
		outputDir = os.TempDir()
	}
	return &Service{
		sfu:       sfu,
		outputDir: outputDir,
		maxDurSec: defaultMaxDurationSec,
		log:       log,
	}
}

// SetMaxDuration sets the maximum recording duration in seconds (for ffmpeg -t).
func (svc *Service) SetMaxDuration(sec int) { svc.maxDurSec = sec }

// buildSDP generates an SDP file that ffmpeg will use to receive RTP (we send with payload 96=video, 97=audio).
func buildSDP(tracks []realtime.TrackInfo, videoPort, audioPort int) string {
	// SDP with fixed payload types 96 (video) and 97 (audio) to match our WriteRTP rewrite.
	s := "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n"
	for _, t := range tracks {
		port := videoPort
		pt := payloadTypeVideo
		codec := "VP8"
		clock := "90000"
		if t.Kind == webrtc.RTPCodecTypeAudio {
			port = audioPort
			pt = payloadTypeAudio
			codec = "opus"
			clock = "48000"
		}
		switch t.MimeType {
		case "video/VP8", "video/vp8":
			codec = "VP8"
			clock = "90000"
		case "video/VP9", "video/vp9":
			codec = "VP9"
			clock = "90000"
		case "video/H264", "video/h264":
			codec = "H264"
			clock = "90000"
		case "audio/opus", "audio/OPUS":
			codec = "opus"
			clock = "48000"
		case "audio/PCMU":
			codec = "PCMU"
			clock = "8000"
		}
		s += fmt.Sprintf("m=%s %d RTP/AVP %d\r\na=rtpmap:%d %s/%s\r\n",
			map[webrtc.RTPCodecType]string{webrtc.RTPCodecTypeVideo: "video", webrtc.RTPCodecTypeAudio: "audio"}[t.Kind],
			port, pt, pt, codec, clock)
	}
	return s
}

// StartRecording starts a recording session for the webinar (speaker view).
// Requires the publisher to already be connected. Returns the output file path when stopped.
func (svc *Service) StartRecording(_ context.Context, webinarID, recordingID uuid.UUID) (outputPath string, err error) {
	tracks := svc.sfu.GetTrackInfo(webinarID)
	if len(tracks) == 0 {
		return "", fmt.Errorf("no publisher tracks: start recording after speaker is live")
	}

	// Allocate ports (use loopback and random port 0 to get free ports, then use them in SDP)
	videoPort, audioPort := 0, 0
	listener, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if listener != nil {
		videoPort = listener.LocalAddr().(*net.UDPAddr).Port
		listener.Close()
	}
	listener2, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if listener2 != nil {
		audioPort = listener2.LocalAddr().(*net.UDPAddr).Port
		listener2.Close()
	}
	if videoPort == 0 {
		videoPort = 5000
	}
	if audioPort == 0 {
		audioPort = 5002
	}

	sdp := buildSDP(tracks, videoPort, audioPort)
	dir := filepath.Join(svc.outputDir, "recordings")
	_ = os.MkdirAll(dir, 0750)
	outputPath = filepath.Join(dir, recordingID.String()+".mp4")
	sdpPath := filepath.Join(dir, recordingID.String()+".sdp")
	if err := os.WriteFile(sdpPath, []byte(sdp), 0600); err != nil {
		return "", fmt.Errorf("write sdp: %w", err)
	}

	// Start ffmpeg: -f sdp -i sdp -c copy -t N -y output.mp4 (do not use request ctx so stop is explicit)
	cmd := exec.Command("ffmpeg",
		"-f", "sdp", "-i", sdpPath,
		"-c", "copy",
		"-t", fmt.Sprintf("%d", svc.maxDurSec),
		"-y",
		outputPath,
	)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		_ = os.Remove(sdpPath)
		return "", fmt.Errorf("start ffmpeg: %w", err)
	}

	videoAddr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", videoPort))
	audioAddr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", audioPort))
	videoConn, err1 := net.DialUDP("udp", nil, videoAddr)
	audioConn, err2 := net.DialUDP("udp", nil, audioAddr)
	if err1 != nil || err2 != nil {
		_ = cmd.Process.Kill()
		if videoConn != nil {
			videoConn.Close()
		}
		if audioConn != nil {
			audioConn.Close()
		}
		_ = os.Remove(sdpPath)
		return "", fmt.Errorf("udp dial: %v / %v", err1, err2)
	}

	session := &Session{
		webinarID:   webinarID,
		recordingID: recordingID,
		outputPath:  outputPath,
		sdpPath:     sdpPath,
		cmd:         cmd,
		videoConn:   videoConn,
		audioConn:   audioConn,
		videoAddr:   videoAddr,
		audioAddr:   audioAddr,
		log:         svc.log,
	}
	sink := &Sink{session: session}
	svc.sfu.RegisterRecordingSink(webinarID, sink)

	// Store session so we can stop it later (by webinarID)
	svc.mu.Lock()
	if svc.sessions == nil {
		svc.sessions = make(map[uuid.UUID]*Session)
	}
	svc.sessions[webinarID] = session
	svc.mu.Unlock()

	svc.log.Info("recording started", zap.String("webinar_id", webinarID.String()), zap.String("recording_id", recordingID.String()), zap.String("output", outputPath))
	return outputPath, nil
}

// StopRecording stops the recording for the webinar and returns the path to the output file.
func (svc *Service) StopRecording(webinarID uuid.UUID) (outputPath string, err error) {
	svc.mu.Lock()
	session, ok := svc.sessions[webinarID]
	if !ok {
		svc.mu.Unlock()
		return "", fmt.Errorf("no active recording for webinar %s", webinarID)
	}
	delete(svc.sessions, webinarID)
	svc.mu.Unlock()

	svc.sfu.UnregisterRecordingSink(webinarID)

	session.mu.Lock()
	cmd := session.cmd
	videoConn := session.videoConn
	audioConn := session.audioConn
	session.videoConn = nil
	session.audioConn = nil
	session.cmd = nil
	session.mu.Unlock()

	if videoConn != nil {
		_ = videoConn.Close()
	}
	if audioConn != nil {
		_ = audioConn.Close()
	}

	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Signal(os.Interrupt)
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
			// ok
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
		}
	}

	_ = os.Remove(session.sdpPath)
	svc.log.Info("recording stopped", zap.String("webinar_id", webinarID.String()), zap.String("output", session.outputPath))
	return session.outputPath, nil
}

// HasActiveRecording returns whether the webinar currently has an active recording.
func (svc *Service) HasActiveRecording(webinarID uuid.UUID) bool {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	_, ok := svc.sessions[webinarID]
	return ok
}
