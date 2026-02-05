package realtime

import (
	"encoding/json"
	"sync"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v3"
	"go.uber.org/zap"
)

// RTP buffer size (MTU-friendly). Used with sync.Pool to avoid per-packet allocs.
const rtpBufferSize = 1500

var rtpBufferPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, rtpBufferSize)
		return &b
	},
}

// RecordingSink receives a copy of RTP packets for recording (e.g. to ffmpeg).
// WriteRTP is called from the relay goroutine; implementation must be non-blocking.
type RecordingSink interface {
	WriteRTP(kind webrtc.RTPCodecType, packet []byte)
}

// SFU manages WebRTC publisher (speaker) and subscribers (audience) per webinar.
type SFU struct {
	rooms map[uuid.UUID]*sfuRoom
	mu    sync.RWMutex
	log   *zap.Logger
	cfg   webrtc.Configuration
}

type sfuRoom struct {
	webinarID     uuid.UUID
	publisher     *webrtc.PeerConnection
	tracks        []*relayTrack
	subscribers   map[string]*subscriberPeer
	recordingSink RecordingSink
	mu            sync.RWMutex
	log           *zap.Logger
}

type relayTrack struct {
	remote   *webrtc.TrackRemote
	locals   []*webrtc.TrackLocalStaticRTP
	roomRef  *sfuRoom
	mu       sync.Mutex
}

type subscriberPeer struct {
	pc *webrtc.PeerConnection
}

// NewSFU creates an SFU with the given ICE (STUN/TURN) configuration.
func NewSFU(log *zap.Logger, iceServers []webrtc.ICEServer) *SFU {
	cfg := webrtc.Configuration{ICEServers: iceServers}
	if len(cfg.ICEServers) == 0 {
		cfg.ICEServers = []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}}
	}
	return &SFU{
		rooms: make(map[uuid.UUID]*sfuRoom),
		log:   log,
		cfg:   cfg,
	}
}

func (s *SFU) getOrCreateRoom(webinarID uuid.UUID) *sfuRoom {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.rooms[webinarID]; ok {
		return r
	}
	r := &sfuRoom{
		webinarID:   webinarID,
		subscribers: make(map[string]*subscriberPeer),
		log:         s.log.With(zap.String("webinar_id", webinarID.String())),
	}
	s.rooms[webinarID] = r
	return r
}

func (s *SFU) getRoom(webinarID uuid.UUID) *sfuRoom {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rooms[webinarID]
}

// HandlePublisherOffer handles SDP offer from speaker (publisher). Creates publisher PC, returns answer.
func (s *SFU) HandlePublisherOffer(webinarID uuid.UUID, clientID string, role string, sdp webrtc.SessionDescription, sendToClient func(event string, payload interface{})) error {
	if role != "speaker" && role != "admin" {
		return nil // ignore
	}
	r := s.getOrCreateRoom(webinarID)

	r.mu.Lock()
	if r.publisher != nil {
		r.mu.Unlock()
		_ = r.publisher.Close()
		r.mu.Lock()
		r.publisher = nil
		r.tracks = nil
	}

	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		r.mu.Unlock()
		return err
	}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))
	pc, err := api.NewPeerConnection(s.cfg)
	if err != nil {
		r.mu.Unlock()
		return err
	}

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		b, _ := json.Marshal(c.ToJSON())
		sendToClient("webrtc_ice", map[string]interface{}{"target": "publisher", "candidate": json.RawMessage(b)})
	})

	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		relay := &relayTrack{remote: track, locals: nil, roomRef: r}
		r.mu.Lock()
		r.tracks = append(r.tracks, relay)
		r.mu.Unlock()
		r.relayTrackToSubscribers(relay)
		go relay.readAndForward()
	})

	if err := pc.SetRemoteDescription(sdp); err != nil {
		_ = pc.Close()
		r.mu.Unlock()
		return err
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		_ = pc.Close()
		r.mu.Unlock()
		return err
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		_ = pc.Close()
		r.mu.Unlock()
		return err
	}
	r.publisher = pc
	r.mu.Unlock()

	sendToClient("webrtc_publisher_answer", map[string]interface{}{
		"type": answer.Type.String(),
		"sdp":  answer.SDP,
	})
	return nil
}

func (rt *relayTrack) readAndForward() {
	for {
		// Reuse buffer from pool to avoid per-packet allocs and bound memory.
		ptr := rtpBufferPool.Get().(*[]byte)
		buf := *ptr
		n, _, err := rt.remote.Read(buf)
		if err != nil {
			rtpBufferPool.Put(ptr)
			return
		}
		// Copy list of subscribers under lock, then write without holding lock
		// so one slow subscriber doesn't block others and we minimize contention.
		rt.mu.Lock()
		locals := make([]*webrtc.TrackLocalStaticRTP, len(rt.locals))
		copy(locals, rt.locals)
		rt.mu.Unlock()
		for _, local := range locals {
			_, _ = local.Write(buf[:n])
		}
		// Recording sink: pass a copy the sink can own (sink may be async); avoid pool so we don't reuse before sink is done.
		if rt.roomRef != nil {
			rt.roomRef.mu.RLock()
			sink := rt.roomRef.recordingSink
			rt.roomRef.mu.RUnlock()
			if sink != nil {
				packetCopy := make([]byte, n)
				copy(packetCopy, buf[:n])
				sink.WriteRTP(rt.remote.Kind(), packetCopy)
			}
		}
		rtpBufferPool.Put(ptr)
	}
}

func (r *sfuRoom) relayTrackToSubscribers(relay *relayTrack) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, sub := range r.subscribers {
		if sub.pc == nil {
			continue
		}
		local, err := webrtc.NewTrackLocalStaticRTP(relay.remote.Codec().RTPCodecCapability, relay.remote.ID(), relay.remote.StreamID())
		if err != nil {
			continue
		}
		relay.mu.Lock()
		relay.locals = append(relay.locals, local)
		relay.mu.Unlock()
		_, _ = sub.pc.AddTrack(local)
	}
}

// HandlePublisherICE adds ICE candidate to the publisher PC.
func (s *SFU) HandlePublisherICE(webinarID uuid.UUID, clientID string, candidate webrtc.ICECandidateInit) error {
	r := s.getRoom(webinarID)
	if r == nil {
		return nil
	}
	r.mu.RLock()
	pc := r.publisher
	r.mu.RUnlock()
	if pc != nil {
		return pc.AddICECandidate(candidate)
	}
	return nil
}

// HandleSubscribe creates a subscriber PC for the audience and sends offer.
func (s *SFU) HandleSubscribe(webinarID uuid.UUID, clientID string, sendToClient func(event string, payload interface{})) error {
	r := s.getRoom(webinarID)
	if r == nil {
		sendToClient("webrtc_error", map[string]string{"message": "no_stream"})
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.publisher == nil || len(r.tracks) == 0 {
		sendToClient("webrtc_error", map[string]string{"message": "no_stream"})
		return nil
	}

	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return err
	}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))
	pc, err := api.NewPeerConnection(s.cfg)
	if err != nil {
		return err
	}

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		b, _ := json.Marshal(c.ToJSON())
		sendToClient("webrtc_ice", map[string]interface{}{"target": "subscriber", "candidate": json.RawMessage(b)})
	})

	for _, relay := range r.tracks {
		local, err := webrtc.NewTrackLocalStaticRTP(relay.remote.Codec().RTPCodecCapability, relay.remote.ID(), relay.remote.StreamID())
		if err != nil {
			continue
		}
		relay.mu.Lock()
		relay.locals = append(relay.locals, local)
		relay.mu.Unlock()
		_, _ = pc.AddTrack(local)
	}

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		_ = pc.Close()
		return err
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		_ = pc.Close()
		return err
	}
	r.subscribers[clientID] = &subscriberPeer{pc: pc}
	sendToClient("webrtc_subscriber_offer", map[string]interface{}{
		"type": offer.Type.String(),
		"sdp":  offer.SDP,
	})
	return nil
}

// HandleSubscriberAnswer sets the remote description (answer) for the subscriber PC.
func (s *SFU) HandleSubscriberAnswer(webinarID uuid.UUID, clientID string, sdp webrtc.SessionDescription) error {
	r := s.getRoom(webinarID)
	if r == nil {
		return nil
	}
	r.mu.Lock()
	sub, ok := r.subscribers[clientID]
	r.mu.Unlock()
	if !ok || sub.pc == nil {
		return nil
	}
	return sub.pc.SetRemoteDescription(sdp)
}

// HandleSubscriberICE adds ICE candidate to the subscriber PC.
func (s *SFU) HandleSubscriberICE(webinarID uuid.UUID, clientID string, candidate webrtc.ICECandidateInit) error {
	r := s.getRoom(webinarID)
	if r == nil {
		return nil
	}
	r.mu.RLock()
	sub, ok := r.subscribers[clientID]
	r.mu.RUnlock()
	if !ok || sub.pc == nil {
		return nil
	}
	return sub.pc.AddICECandidate(candidate)
}

// UnregisterClient removes a subscriber and closes their PC. Call when client leaves.
func (s *SFU) UnregisterClient(webinarID uuid.UUID, clientID string) {
	r := s.getRoom(webinarID)
	if r == nil {
		return
	}
	r.mu.Lock()
	if sub, ok := r.subscribers[clientID]; ok {
		delete(r.subscribers, clientID)
		if sub.pc != nil {
			_ = sub.pc.Close()
		}
	}
	r.mu.Unlock()
}

// ClosePublisher closes the publisher PC for a webinar (e.g. when speaker leaves).
func (s *SFU) ClosePublisher(webinarID uuid.UUID) {
	r := s.getRoom(webinarID)
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.publisher != nil {
		_ = r.publisher.Close()
		r.publisher = nil
	}
	r.tracks = nil
	r.mu.Unlock()
}

// TrackInfo describes a track for building recording SDP (codec, kind).
type TrackInfo struct {
	Kind     webrtc.RTPCodecType
	MimeType string
	ClockRate uint32
}

// GetTrackInfo returns current publisher track info for the room (for recording SDP).
func (s *SFU) GetTrackInfo(webinarID uuid.UUID) []TrackInfo {
	r := s.getRoom(webinarID)
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.tracks) == 0 {
		return nil
	}
	out := make([]TrackInfo, 0, len(r.tracks))
	for _, relay := range r.tracks {
		c := relay.remote.Codec()
		out = append(out, TrackInfo{
			Kind:      relay.remote.Kind(),
			MimeType:  c.MimeType,
			ClockRate: c.ClockRate,
		})
	}
	return out
}

// RegisterRecordingSink sets the sink that receives a copy of RTP for recording. Only one sink per room.
func (s *SFU) RegisterRecordingSink(webinarID uuid.UUID, sink RecordingSink) {
	r := s.getRoom(webinarID)
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recordingSink = sink
}

// UnregisterRecordingSink removes the recording sink for the room.
func (s *SFU) UnregisterRecordingSink(webinarID uuid.UUID) {
	r := s.getRoom(webinarID)
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recordingSink = nil
}

// ICE config helpers
var defaultICE = []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}}

func parseICEServers(urls []string) []webrtc.ICEServer {
	if len(urls) == 0 {
		return defaultICE
	}
	out := make([]webrtc.ICEServer, 0, len(urls))
	for _, u := range urls {
		if u == "" {
			continue
		}
		out = append(out, webrtc.ICEServer{URLs: []string{u}})
	}
	if len(out) == 0 {
		return defaultICE
	}
	return out
}

