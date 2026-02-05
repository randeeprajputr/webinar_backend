package realtime

import (
	"encoding/json"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	// PingInterval and PongWait are used for heartbeat.
	PingInterval = 30
	PongWait     = 60
)

// AudienceChangeHandler is called when audience count changes for a webinar (e.g. for peak tracking).
type AudienceChangeHandler func(webinarID uuid.UUID, count int)

// Hub maintains webinar_id -> set of connections and broadcasts messages.
// Uses Redis pub/sub for horizontal scaling: local broadcast + publish to Redis.
type Hub struct {
	// webinarID -> map[clientID]*Client
	webinars    map[uuid.UUID]map[string]*Client
	subs        map[uuid.UUID]func() // cancel Redis subscription per webinar
	mu          sync.RWMutex
	logger      *zap.Logger
	redis       RedisPublisher
	redisSub    RedisSubscriber
	onAudience  AudienceChangeHandler
}

// RedisPublisher is the interface for publishing to Redis (for cross-instance broadcast).
type RedisPublisher interface {
	PublishWebinarEvent(webinarID uuid.UUID, event string, payload []byte) error
}

// RedisSubscriber subscribes to webinar channels and invokes handler for incoming events.
type RedisSubscriber interface {
	SubscribeWebinar(webinarID uuid.UUID, handler func(event string, payload []byte)) (cancel func(), err error)
}

// NewHub creates a new WebSocket hub.
func NewHub(logger *zap.Logger, redisPub RedisPublisher, redisSub RedisSubscriber) *Hub {
	return &Hub{
		webinars:  make(map[uuid.UUID]map[string]*Client),
		subs:      make(map[uuid.UUID]func()),
		logger:    logger,
		redis:     redisPub,
		redisSub:  redisSub,
		onAudience: nil,
	}
}

// SetAudienceChangeHandler sets the callback for audience count changes (e.g. peak viewers).
func (h *Hub) SetAudienceChangeHandler(fn AudienceChangeHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onAudience = fn
}

// Register adds a client to a webinar room. Starts Redis subscription for this webinar if first client.
func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	if h.webinars[c.WebinarID] == nil {
		h.webinars[c.WebinarID] = make(map[string]*Client)
		if h.redisSub != nil {
			cancel, err := h.redisSub.SubscribeWebinar(c.WebinarID, func(event string, payload []byte) {
				h.BroadcastToWebinar(c.WebinarID, event, json.RawMessage(payload))
			})
			if err == nil {
				h.subs[c.WebinarID] = cancel
			}
		}
	}
	h.webinars[c.WebinarID][c.ID] = c
	count := len(h.webinars[c.WebinarID])
	onAudience := h.onAudience
	h.mu.Unlock()
	if onAudience != nil {
		onAudience(c.WebinarID, count)
	}
	h.logger.Debug("client joined webinar", zap.String("client_id", c.ID), zap.String("webinar_id", c.WebinarID.String()))
}

// Unregister removes a client from a webinar room. Cancels Redis subscription when last client leaves.
func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	var count int
	if m, ok := h.webinars[c.WebinarID]; ok {
		delete(m, c.ID)
		count = len(m)
		if count == 0 {
			delete(h.webinars, c.WebinarID)
			if cancel, ok := h.subs[c.WebinarID]; ok {
				cancel()
				delete(h.subs, c.WebinarID)
			}
		}
	}
	onAudience := h.onAudience
	h.mu.Unlock()
	if onAudience != nil && count > 0 {
		onAudience(c.WebinarID, count)
	}
	h.logger.Debug("client left webinar", zap.String("client_id", c.ID), zap.String("webinar_id", c.WebinarID.String()))
}

// BroadcastToWebinar sends a message to all clients in a webinar (local only).
func (h *Hub) BroadcastToWebinar(webinarID uuid.UUID, event string, payload interface{}) {
	var data []byte
	switch v := payload.(type) {
	case []byte:
		data = v
	case json.RawMessage:
		data = v
	default:
		data, _ = json.Marshal(payload)
	}
	msg := WSMessage{Event: event, Data: data}

	h.mu.RLock()
	clients := h.webinars[webinarID]
	h.mu.RUnlock()

	if clients == nil {
		return
	}
	for _, c := range clients {
		select {
		case c.send <- msg:
		default:
			// buffer full, skip
		}
	}
}

// BroadcastToWebinarAndPublish sends to local clients and publishes to Redis for other instances.
func (h *Hub) BroadcastToWebinarAndPublish(webinarID uuid.UUID, event string, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.BroadcastToWebinar(webinarID, event, payload)
	if h.redis != nil {
		_ = h.redis.PublishWebinarEvent(webinarID, event, data)
	}
}

// PublishToWebinarOnly publishes to Redis only (no local broadcast). Used for events like chat_message
// so that the Redis subscriber callback performs the broadcast once for all instances (including this one),
// avoiding duplicate delivery to local clients.
func (h *Hub) PublishToWebinarOnly(webinarID uuid.UUID, event string, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if h.redis != nil {
		_ = h.redis.PublishWebinarEvent(webinarID, event, data)
		return
	}
	h.BroadcastToWebinar(webinarID, event, payload)
}

// AudienceCount returns the number of connected clients in a webinar.
func (h *Hub) AudienceCount(webinarID uuid.UUID) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.webinars[webinarID])
}

// SendToClient sends a message to a single client in a webinar (for WebRTC signaling).
func (h *Hub) SendToClient(webinarID uuid.UUID, clientID string, event string, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	msg := WSMessage{Event: event, Data: data}
	h.mu.RLock()
	clients := h.webinars[webinarID]
	c, ok := clients[clientID]
	h.mu.RUnlock()
	if !ok || c == nil {
		return
	}
	select {
	case c.send <- msg:
	default:
	}
}
