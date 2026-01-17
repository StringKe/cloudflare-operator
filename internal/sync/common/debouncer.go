// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package common

import (
	"sync"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

// DefaultDebounceDelay is the default delay before executing debounced operations.
// This allows multiple rapid changes to be coalesced into a single API call.
const DefaultDebounceDelay = 500 * time.Millisecond

// Debouncer coalesces multiple events into a single operation.
// When an event occurs, it waits for the delay period. If another event
// occurs during this period, the timer resets. The operation only executes
// when no new events occur within the delay period.
//
// This is useful for:
// - Reducing Cloudflare API calls when multiple resources change rapidly
// - Handling bulk operations (e.g., creating multiple Ingresses)
// - Avoiding rate limiting from the Cloudflare API
type Debouncer struct {
	delay   time.Duration
	pending map[string]*debouncedItem
	mu      sync.Mutex
}

type debouncedItem struct {
	timer   *time.Timer
	request ctrl.Request
}

// NewDebouncer creates a new Debouncer with the specified delay.
// Use DefaultDebounceDelay for the standard 500ms delay.
func NewDebouncer(delay time.Duration) *Debouncer {
	return &Debouncer{
		delay:   delay,
		pending: make(map[string]*debouncedItem),
	}
}

// Debounce schedules a function to be called after the delay.
// If called again with the same key before the delay expires,
// the timer is reset and only the latest function will be called.
//
// The key should uniquely identify the operation (e.g., SyncState name).
func (d *Debouncer) Debounce(key string, fn func()) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Cancel existing timer if present
	if item, ok := d.pending[key]; ok {
		item.timer.Stop()
	}

	// Schedule new timer
	d.pending[key] = &debouncedItem{
		timer: time.AfterFunc(d.delay, func() {
			d.mu.Lock()
			delete(d.pending, key)
			d.mu.Unlock()
			fn()
		}),
	}
}

// DebounceRequest schedules a reconciliation request after the delay.
// Returns true if a new debounce was scheduled, false if one was already pending.
func (d *Debouncer) DebounceRequest(key string, req ctrl.Request, enqueue func(ctrl.Request)) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Cancel existing timer if present
	if item, ok := d.pending[key]; ok {
		item.timer.Stop()
		delete(d.pending, key)
	}

	// Schedule new timer
	d.pending[key] = &debouncedItem{
		timer: time.AfterFunc(d.delay, func() {
			d.mu.Lock()
			delete(d.pending, key)
			d.mu.Unlock()
			enqueue(req)
		}),
		request: req,
	}

	return true
}

// Cancel cancels a pending debounced operation.
// Returns true if an operation was cancelled, false if none was pending.
func (d *Debouncer) Cancel(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if item, ok := d.pending[key]; ok {
		item.timer.Stop()
		delete(d.pending, key)
		return true
	}
	return false
}

// Flush immediately executes all pending operations.
// This is useful during shutdown or testing.
func (d *Debouncer) Flush() {
	d.mu.Lock()
	pending := make(map[string]*debouncedItem)
	for k, v := range d.pending {
		pending[k] = v
	}
	d.pending = make(map[string]*debouncedItem)
	d.mu.Unlock()

	// Stop all timers (functions may have already been scheduled)
	for _, item := range pending {
		item.timer.Stop()
	}
}

// PendingCount returns the number of pending debounced operations.
func (d *Debouncer) PendingCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.pending)
}

// IsPending checks if a specific key has a pending operation.
func (d *Debouncer) IsPending(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.pending[key]
	return ok
}

// GetDelay returns the debounce delay duration.
func (d *Debouncer) GetDelay() time.Duration {
	return d.delay
}
