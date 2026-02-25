package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

// SkillSyncEvent represents a progress update during skill synchronization.
type SkillSyncEvent struct {
	Type        string `json:"type"`   // always "skill_sync"
	Action      string `json:"action"` // "progress" | "complete" | "error"
	SkillID     int64  `json:"skill_id"`
	SkillName   string `json:"skill_name"`
	CommandName string `json:"command_name"`
	AgentID     int64  `json:"agent_id"`
	Step        string `json:"step"` // "writing_skill_md" | "downloading_file" | "restarting_process" | "cleaning_stale" | "done"
	Message     string `json:"message"`
	Current     int    `json:"current,omitempty"`
	Total       int    `json:"total,omitempty"`
}

// SyncProgressPublisher is the interface used by SyncService to publish progress events
// without hard-depending on Redis.
type SyncProgressPublisher interface {
	Publish(ctx context.Context, userID int64, agentID int64, event SkillSyncEvent)
}

// syncSubscriber is a single WebSocket connection waiting for sync events.
type syncSubscriber struct {
	ch  chan SkillSyncEvent
	key string // "userID:agentID"
}

// SyncHub manages WebSocket subscriptions and Redis Pub/Sub for skill sync progress events.
type SyncHub struct {
	rdb  *redis.Client
	mu   sync.RWMutex
	subs map[*syncSubscriber]struct{}
}

// NewSyncHub creates a new SyncHub.
func NewSyncHub(rdb *redis.Client) *SyncHub {
	return &SyncHub{
		rdb:  rdb,
		subs: make(map[*syncSubscriber]struct{}),
	}
}

// Start listens for Redis Pub/Sub messages and dispatches to subscribers.
func (h *SyncHub) Start(ctx context.Context) {
	pubsub := h.rdb.PSubscribe(ctx, "sac:skill-sync:*")
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			key := strings.TrimPrefix(msg.Channel, "sac:skill-sync:")

			var event SkillSyncEvent
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				log.Warn().Err(err).Msg("SyncHub: bad event payload")
				continue
			}

			h.mu.RLock()
			for sub := range h.subs {
				if sub.key == key {
					select {
					case sub.ch <- event:
					default:
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Publish sends a sync progress event to Redis.
func (h *SyncHub) Publish(ctx context.Context, userID int64, agentID int64, event SkillSyncEvent) {
	event.Type = "skill_sync"
	data, err := json.Marshal(event)
	if err != nil {
		log.Warn().Err(err).Msg("SyncHub: marshal error")
		return
	}
	channel := fmt.Sprintf("sac:skill-sync:%d:%d", userID, agentID)
	if err := h.rdb.Publish(ctx, channel, data).Err(); err != nil {
		log.Warn().Err(err).Msg("SyncHub: publish error")
	}
}

// Subscribe registers a WebSocket connection for a user/agent pair.
// Returns a channel for reading events and an unsubscribe function.
func (h *SyncHub) Subscribe(userID, agentID int64) (<-chan SkillSyncEvent, func()) {
	sub := &syncSubscriber{
		ch:  make(chan SkillSyncEvent, 32),
		key: fmt.Sprintf("%d:%d", userID, agentID),
	}

	h.mu.Lock()
	h.subs[sub] = struct{}{}
	h.mu.Unlock()

	return sub.ch, func() {
		h.mu.Lock()
		delete(h.subs, sub)
		h.mu.Unlock()
	}
}
