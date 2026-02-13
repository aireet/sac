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

// OutputHub manages SSE subscriptions and Redis Pub/Sub for output workspace events.
type OutputHub struct {
	rdb  *redis.Client
	mu   sync.RWMutex
	subs map[string]map[chan OutputEvent]struct{} // key: "userID:agentID"
}

// NewOutputHub creates a new OutputHub.
func NewOutputHub(rdb *redis.Client) *OutputHub {
	return &OutputHub{
		rdb:  rdb,
		subs: make(map[string]map[chan OutputEvent]struct{}),
	}
}

// Start listens for Redis Pub/Sub messages and dispatches to local subscribers.
// Should be run as a goroutine.
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
			// channel format: sac:output:{userID}:{agentID}
			key := strings.TrimPrefix(msg.Channel, "sac:output:")

			var event OutputEvent
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				log.Printf("OutputHub: bad event payload: %v", err)
				continue
			}

			h.mu.RLock()
			listeners := h.subs[key]
			h.mu.RUnlock()

			for ch := range listeners {
				// Non-blocking send; drop if slow consumer
				select {
				case ch <- event:
				default:
				}
			}
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

// Subscribe registers a local listener for a user/agent pair.
// Returns the event channel and an unsubscribe function.
func (h *OutputHub) Subscribe(userID, agentID int64) (chan OutputEvent, func()) {
	key := fmt.Sprintf("%d:%d", userID, agentID)
	ch := make(chan OutputEvent, 16)

	h.mu.Lock()
	if h.subs[key] == nil {
		h.subs[key] = make(map[chan OutputEvent]struct{})
	}
	h.subs[key][ch] = struct{}{}
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		delete(h.subs[key], ch)
		if len(h.subs[key]) == 0 {
			delete(h.subs, key)
		}
		h.mu.Unlock()
		close(ch)
	}
}
