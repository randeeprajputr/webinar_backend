package ads

import (
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/aura-webinar/backend/pkg/storage"
)

// RotatorRegistry holds running ad rotators per webinar (thread-safe).
type RotatorRegistry struct {
	mu       sync.RWMutex
	rotators map[string]*Rotator
}

// NewRotatorRegistry creates a new rotator registry.
func NewRotatorRegistry() *RotatorRegistry {
	return &RotatorRegistry{rotators: make(map[string]*Rotator)}
}

// Start starts the rotator for webinarID if not already running. Creates rotator with adRepo, hub, s3, interval, logger.
func (reg *RotatorRegistry) Start(webinarID uuid.UUID, adRepo *AdvertisementRepository, hub HubBroadcaster, s3 *storage.S3, rotationInterval int, logger *zap.Logger) {
	key := webinarID.String()
	reg.mu.Lock()
	defer reg.mu.Unlock()
	if reg.rotators[key] != nil {
		return
	}
	rotator := NewRotator(webinarID, adRepo, hub, s3, rotationInterval, logger)
	reg.rotators[key] = rotator
	rotator.Start()
}

// Stop stops the rotator for webinarID and removes it from the registry.
func (reg *RotatorRegistry) Stop(webinarID uuid.UUID) {
	key := webinarID.String()
	reg.mu.Lock()
	rotator := reg.rotators[key]
	delete(reg.rotators, key)
	reg.mu.Unlock()
	if rotator != nil {
		rotator.Stop()
	}
}

// Reload signals the rotator for webinarID to reload ads.
func (reg *RotatorRegistry) Reload(webinarID uuid.UUID) {
	reg.mu.RLock()
	rotator := reg.rotators[webinarID.String()]
	reg.mu.RUnlock()
	if rotator != nil {
		rotator.Reload()
	}
}
