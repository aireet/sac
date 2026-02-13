package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/redis/go-redis/v9"
)

// OutputEvent represents a file change in the output workspace.
type OutputEvent struct {
	Action string `json:"action"` // "upload" | "delete"
	Path   string `json:"path"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
}

const ringSize = 64 // per-agent event ring buffer; must be power of 2

// agentSlot is a shared broadcast buffer for one user:agent pair.
// All SSE connections for the same agent share this single slot.
type agentSlot struct {
	mu     sync.Mutex
	ring   [ringSize]OutputEvent
	head   uint64        // monotonic write index
	notify chan struct{} // closed-and-replaced on each push to wake all readers
	refs   int32         // active cursor count (atomic)
}

func newAgentSlot() *agentSlot {
	return &agentSlot{
		notify: make(chan struct{}),
	}
}

// push appends an event to the ring buffer and wakes all waiting cursors.
func (s *agentSlot) push(e OutputEvent) {
	s.mu.Lock()
	s.ring[s.head%ringSize] = e
	s.head++
	ch := s.notify
	s.notify = make(chan struct{})
	s.mu.Unlock()
	close(ch) // broadcast: all select{} on old ch unblock
}

// Cursor tracks a single SSE connection's read position in an agentSlot.
// Lightweight: just a pointer + uint64. No channel allocation per connection.
type Cursor struct {
	slot *agentSlot
	pos  uint64 // next index to read
}

// Next blocks until a new event is available or ctx is cancelled.
// Returns (event, true) on success, or (zero, false) on cancellation.
func (c *Cursor) Next(ctx context.Context) (OutputEvent, bool) {
	for {
		c.slot.mu.Lock()
		if c.pos < c.slot.head {
			// Slow reader fell behind — skip to oldest available event
			if c.slot.head-c.pos > ringSize {
				c.pos = c.slot.head - ringSize
			}
			e := c.slot.ring[c.pos%ringSize]
			c.pos++
			c.slot.mu.Unlock()
			return e, true
		}
		// No new events; grab notify channel to wait on
		notify := c.slot.notify
		c.slot.mu.Unlock()

		select {
		case <-ctx.Done():
			return OutputEvent{}, false
		case <-notify:
			// new event(s) pushed, loop to read
		}
	}
}

// OutputHub manages SSE subscriptions and Redis Pub/Sub for output workspace events.
// Each user:agent pair shares a single agentSlot regardless of how many tabs are open.
type OutputHub struct {
	rdb   *redis.Client
	mu    sync.RWMutex
	slots map[string]*agentSlot // key: "userID:agentID"
}

// NewOutputHub creates a new OutputHub.
func NewOutputHub(rdb *redis.Client) *OutputHub {
	return &OutputHub{
		rdb:   rdb,
		slots: make(map[string]*agentSlot),
	}
}

// Start listens for Redis Pub/Sub messages and dispatches to local agentSlots.
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
			slot := h.slots[key]
			h.mu.RUnlock()

			if slot != nil {
				slot.push(event)
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

// Subscribe registers a Cursor for a user/agent pair.
// Returns the cursor and an unsubscribe function.
// Multiple calls for the same agent share one agentSlot.
func (h *OutputHub) Subscribe(userID, agentID int64) (*Cursor, func()) {
	key := fmt.Sprintf("%d:%d", userID, agentID)

	h.mu.Lock()
	slot := h.slots[key]
	if slot == nil {
		slot = newAgentSlot()
		h.slots[key] = slot
	}
	atomic.AddInt32(&slot.refs, 1)
	// New cursor starts from current head — no replay of old events
	cursor := &Cursor{slot: slot, pos: slot.head}
	h.mu.Unlock()

	return cursor, func() {
		if atomic.AddInt32(&slot.refs, -1) == 0 {
			h.mu.Lock()
			// Double-check: another goroutine may have subscribed between AddInt32 and Lock
			if atomic.LoadInt32(&slot.refs) == 0 {
				delete(h.slots, key)
			}
			h.mu.Unlock()
		}
	}
}
