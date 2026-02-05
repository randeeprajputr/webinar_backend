package realtime

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // allow all origins in dev; restrict in production
	},
}

// WSMessage is the WebSocket message envelope.
type WSMessage struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// Client represents a single WebSocket connection in a webinar.
type Client struct {
	ID        string
	WebinarID uuid.UUID
	UserID    uuid.UUID
	Role      string
	JoinedAt  time.Time // set on Register for session log
	hub       *Hub
	sfu       *SFU
	conn      *websocket.Conn
	send      chan WSMessage
	logger    *zap.Logger
}

// ServeWs handles the WebSocket upgrade and runs the client loop.
func ServeWs(hub *Hub, logger *zap.Logger, jwtValidate func(token string) (userID, role string, err error), sfu *SFU) gin.HandlerFunc {
	return func(c *gin.Context) {
		webinarIDStr := c.Query("webinar_id")
		token := c.Query("token")
		if webinarIDStr == "" || token == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "webinar_id and token required"})
			return
		}
		webinarID, err := uuid.Parse(webinarIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webinar_id"})
			return
		}
		userIDStr, role, err := jwtValidate(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		userID, _ := uuid.Parse(userIDStr)

		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			logger.Warn("websocket upgrade failed", zap.Error(err))
			return
		}

		client := &Client{
			ID:        uuid.New().String(),
			WebinarID: webinarID,
			UserID:    userID,
			Role:      role,
			JoinedAt:  time.Now(),
			hub:       hub,
			sfu:       sfu,
			conn:      conn,
			send:      make(chan WSMessage, 256),
			logger:    logger,
		}
		hub.Register(client)
		go client.writePump()
		client.readPump()
	}
}

func (c *Client) readPump() {
	defer func() {
		if c.sfu != nil {
			c.sfu.UnregisterClient(c.WebinarID, c.ID)
		}
		c.hub.Unregister(c)
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(65536)
	_ = c.conn.SetReadDeadline(time.Now().Add(PongWait * time.Second))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(PongWait * time.Second))
		return nil
	})

	sendToMe := func(event string, payload interface{}) {
		c.hub.SendToClient(c.WebinarID, c.ID, event, payload)
	}

	for {
		var msg WSMessage
		if err := c.conn.ReadJSON(&msg); err != nil {
			break
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(PongWait * time.Second))

		switch msg.Event {
		case "join":
			c.hub.BroadcastToWebinarAndPublish(c.WebinarID, "audience_count", map[string]int{
				"count": c.hub.AudienceCount(c.WebinarID),
			})
			c.hub.BroadcastToWebinarAndPublish(c.WebinarID, "join", map[string]string{
				"user_id": c.UserID.String(),
				"role":    c.Role,
			})
		case "webrtc_publisher_offer":
			if c.sfu != nil {
				var payload struct {
					Type string `json:"type"`
					SDP  string `json:"sdp"`
				}
				if err := json.Unmarshal(msg.Data, &payload); err == nil && payload.SDP != "" {
					sdp := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: payload.SDP}
					_ = c.sfu.HandlePublisherOffer(c.WebinarID, c.ID, c.Role, sdp, sendToMe)
				}
			}
		case "webrtc_subscribe":
			if c.sfu != nil {
				_ = c.sfu.HandleSubscribe(c.WebinarID, c.ID, sendToMe)
			}
		case "webrtc_subscriber_answer":
			if c.sfu != nil {
				var payload struct {
					Type string `json:"type"`
					SDP  string `json:"sdp"`
				}
				if err := json.Unmarshal(msg.Data, &payload); err == nil && payload.SDP != "" {
					sdp := webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: payload.SDP}
					_ = c.sfu.HandleSubscriberAnswer(c.WebinarID, c.ID, sdp)
				}
			}
		case "webrtc_ice":
			if c.sfu != nil {
				var payload struct {
					Target    string          `json:"target"`
					Candidate json.RawMessage `json:"candidate"`
				}
				if err := json.Unmarshal(msg.Data, &payload); err == nil && len(payload.Candidate) > 0 {
					var cand webrtc.ICECandidateInit
					if json.Unmarshal(payload.Candidate, &cand) == nil {
						if payload.Target == "publisher" {
							_ = c.sfu.HandlePublisherICE(c.WebinarID, c.ID, cand)
						} else if payload.Target == "subscriber" {
							_ = c.sfu.HandleSubscriberICE(c.WebinarID, c.ID, cand)
						}
					}
				}
			}
		case "ask_question", "approve_question", "launch_poll", "answer_poll", "rotate_ad":
			c.hub.BroadcastToWebinarAndPublish(c.WebinarID, msg.Event, json.RawMessage(msg.Data))
		case "chat_message":
			// Real-time chat: publish only so Redis subscriber broadcasts once (avoids duplicate for local clients).
			c.hub.PublishToWebinarOnly(c.WebinarID, msg.Event, json.RawMessage(msg.Data))
		default:
			// ignore
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(PingInterval * time.Second)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteJSON(msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
