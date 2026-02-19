package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"
)

// OutputEvent represents a file change in the output workspace.
type OutputEvent struct {
	Action string `json:"action"` // "upload" | "delete"
	Path   string `json:"path"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
}

// subscriber is a single WebSocket connection waiting for events.
type subscriber struct {
	ch  chan OutputEvent
	key string // "userID:agentID"
}

// OutputHub manages WebSocket subscriptions and Redis Pub/Sub for output workspace events.
// Each Subscribe call creates one channel; only the latest connection per agent matters.
type OutputHub struct {
	rdb  *redis.Client
	mu   sync.RWMutex
	subs map[*subscriber]struct{}
}

// NewOutputHub creates a new OutputHub.
func NewOutputHub(rdb *redis.Client) *OutputHub {
	return &OutputHub{
		rdb:  rdb,
		subs: make(map[*subscriber]struct{}),
	}
}

// Start listens for Redis Pub/Sub messages and dispatches to subscribers.
func (h *OutputHub) Start(ctx context.Context) {
	pubsub := h.rdb.PSubscribe(ctx, "sac:output:*")
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
			key := strings.TrimPrefix(msg.Channel, "sac:output:")

			var event OutputEvent
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				log.Printf("OutputHub: bad event payload: %v", err)
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

// Publish sends an output event to Redis.
func (h *OutputHub) Publish(ctx context.Context, userID, agentID int64, event OutputEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("OutputHub: marshal error: %v", err)
		return
	}
	channel := fmt.Sprintf("sac:output:%d:%d", userID, agentID)
	if err := h.rdb.Publish(ctx, channel, data).Err(); err != nil {
		log.Printf("OutputHub: publish error: %v", err)
	}
}

// Subscribe registers a WebSocket connection for a user/agent pair.
// Returns a channel for reading events and an unsubscribe function.
func (h *OutputHub) Subscribe(userID, agentID int64) (<-chan OutputEvent, func()) {
	sub := &subscriber{
		ch:  make(chan OutputEvent, 16),
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
