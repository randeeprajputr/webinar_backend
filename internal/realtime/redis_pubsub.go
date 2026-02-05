package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	channelPrefix = "webinar:"
	eventTTL      = 5 * time.Second
)

// redisPayload is the message published to Redis for cross-instance broadcast.
type redisPayload struct {
	Event   string          `json:"event"`
	Data    json.RawMessage `json:"data"`
	At      int64           `json:"at"`
}

// RedisPubSub implements RedisPublisher using Redis pub/sub.
type RedisPubSub struct {
	client *redis.Client
	logger *zap.Logger
}

// NewRedisPubSub creates a Redis pub/sub bridge for webinar events.
func NewRedisPubSub(client *redis.Client, logger *zap.Logger) *RedisPubSub {
	return &RedisPubSub{client: client, logger: logger}
}

// PublishWebinarEvent publishes an event to the webinar's Redis channel.
func (r *RedisPubSub) PublishWebinarEvent(webinarID uuid.UUID, event string, payload []byte) error {
	channel := channelPrefix + webinarID.String()
	body, err := json.Marshal(redisPayload{Event: event, Data: payload, At: time.Now().Unix()})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), eventTTL)
	defer cancel()
	return r.client.Publish(ctx, channel, body).Err()
}

// SubscribeWebinar subscribes to a webinar's Redis channel and calls handler for each message.
// Returns a cancel function to stop the subscription.
func (r *RedisPubSub) SubscribeWebinar(webinarID uuid.UUID, handler func(event string, payload []byte)) (cancel func(), err error) {
	channel := channelPrefix + webinarID.String()
	ctx, cancelCtx := context.WithCancel(context.Background())
	pubsub := r.client.Subscribe(ctx, channel)
	_, err = pubsub.Receive(ctx)
	if err != nil {
		cancelCtx()
		return nil, fmt.Errorf("subscribe: %w", err)
	}
	ch := pubsub.Channel()
	go func() {
		defer pubsub.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				var p redisPayload
				if err := json.Unmarshal([]byte(msg.Payload), &p); err != nil {
					continue
				}
				handler(p.Event, p.Data)
			}
		}
	}()
	cancel = func() { cancelCtx() }
	return cancel, nil
}
