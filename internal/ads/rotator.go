package ads

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/aura-webinar/backend/pkg/storage"
)

// Rotator runs ad rotation for a single webinar: ticker, load active ads, broadcast ad_changed.
type Rotator struct {
	webinarID uuid.UUID
	adRepo    *AdvertisementRepository
	hub       HubBroadcaster
	s3        *storage.S3
	logger    *zap.Logger
	interval  time.Duration
	mu        sync.Mutex
	cancel    context.CancelFunc
	done      chan struct{}
	reloadCh  chan struct{}
}

// NewRotator creates an ad rotator for a webinar.
func NewRotator(webinarID uuid.UUID, adRepo *AdvertisementRepository, hub HubBroadcaster, s3 *storage.S3, intervalSec int, logger *zap.Logger) *Rotator {
	if intervalSec <= 0 {
		intervalSec = 30
	}
	return &Rotator{
		webinarID: webinarID,
		adRepo:    adRepo,
		hub:       hub,
		s3:        s3,
		logger:    logger,
		interval:  time.Duration(intervalSec) * time.Second,
		done:      make(chan struct{}),
		reloadCh:  make(chan struct{}, 1),
	}
}

// Start begins the rotation loop. Call Stop() to release resources.
func (r *Rotator) Start() {
	r.mu.Lock()
	if r.cancel != nil {
		r.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.mu.Unlock()

	go r.run(ctx)
	r.logger.Info("ad rotator started", zap.String("webinar_id", r.webinarID.String()), zap.Duration("interval", r.interval))
}

// Stop stops the rotation and releases resources.
func (r *Rotator) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel == nil {
		return
	}
	r.cancel()
	r.cancel = nil
	<-r.done
	r.logger.Info("ad rotator stopped", zap.String("webinar_id", r.webinarID.String()))
}

// Reload signals the rotator to reload the ad list on next tick (e.g. when new ad added).
func (r *Rotator) Reload() {
	select {
	case r.reloadCh <- struct{}{}:
	default:
	}
}

func (r *Rotator) run(ctx context.Context) {
	defer close(r.done)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	var (
		ads   []adItem
		index int
	)
	load := func() {
		list, err := r.adRepo.ListActiveByWebinar(ctx, r.webinarID)
		if err != nil {
			r.logger.Warn("ad rotator load ads failed", zap.Error(err), zap.String("webinar_id", r.webinarID.String()))
			return
		}
		now := time.Now()
		var filtered []adItem
		for _, a := range list {
			ok, _ := r.adRepo.IsAdScheduledNow(ctx, a.ID, now)
			if ok {
				filtered = append(filtered, adItem{id: a.ID, fileURL: a.FileURL, fileType: a.FileType, s3Key: a.S3Key})
			}
		}
		ads = filtered
		index = 0
	}
	load()

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.reloadCh:
			load()
			continue
		case <-ticker.C:
			if len(ads) == 0 {
				load()
				continue
			}
			cur := ads[index%len(ads)]
			index++
			fileURL := cur.fileURL
			if r.s3 != nil && cur.s3Key != "" {
				fileURL = r.s3.PublicObjectURL(r.s3.UploadAdPresignedBucket(), cur.s3Key)
			}
			if r.hub != nil {
				r.hub.BroadcastToWebinarAndPublish(r.webinarID, "ad_changed", map[string]interface{}{
					"ad_id": cur.id, "file_url": fileURL, "type": cur.fileType,
				})
			}
		}
	}
}

type adItem struct {
	id      uuid.UUID
	fileURL string
	fileType string
	s3Key   string
}
