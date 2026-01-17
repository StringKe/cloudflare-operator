// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package common

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestNewDebouncer(t *testing.T) {
	d := NewDebouncer(100 * time.Millisecond)
	require.NotNil(t, d)
	assert.Equal(t, 100*time.Millisecond, d.GetDelay())
	assert.Equal(t, 0, d.PendingCount())
}

func TestDebouncer_Debounce_SingleCall(t *testing.T) {
	d := NewDebouncer(50 * time.Millisecond)
	var called int32

	d.Debounce("test-key", func() {
		atomic.AddInt32(&called, 1)
	})

	// Should be pending immediately
	assert.True(t, d.IsPending("test-key"))
	assert.Equal(t, 1, d.PendingCount())

	// Wait for debounce to complete
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, int32(1), atomic.LoadInt32(&called))
	assert.False(t, d.IsPending("test-key"))
	assert.Equal(t, 0, d.PendingCount())
}

func TestDebouncer_Debounce_MultipleCallsSameKey(t *testing.T) {
	d := NewDebouncer(50 * time.Millisecond)
	var called int32

	// Make multiple calls with the same key
	for i := 0; i < 5; i++ {
		d.Debounce("test-key", func() {
			atomic.AddInt32(&called, 1)
		})
		time.Sleep(10 * time.Millisecond) // Short sleep to not exceed debounce delay
	}

	// Should still be pending
	assert.True(t, d.IsPending("test-key"))

	// Wait for debounce to complete
	time.Sleep(100 * time.Millisecond)

	// Should have only called once (coalesced)
	assert.Equal(t, int32(1), atomic.LoadInt32(&called))
}

func TestDebouncer_Debounce_DifferentKeys(t *testing.T) {
	d := NewDebouncer(50 * time.Millisecond)
	var called1, called2 int32

	d.Debounce("key1", func() {
		atomic.AddInt32(&called1, 1)
	})
	d.Debounce("key2", func() {
		atomic.AddInt32(&called2, 1)
	})

	assert.Equal(t, 2, d.PendingCount())
	assert.True(t, d.IsPending("key1"))
	assert.True(t, d.IsPending("key2"))

	// Wait for both to complete
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, int32(1), atomic.LoadInt32(&called1))
	assert.Equal(t, int32(1), atomic.LoadInt32(&called2))
	assert.Equal(t, 0, d.PendingCount())
}

func TestDebouncer_DebounceRequest(t *testing.T) {
	d := NewDebouncer(50 * time.Millisecond)
	var receivedReq ctrl.Request

	req := ctrl.Request{
		NamespacedName: reconcile.Request{}.NamespacedName,
	}
	req.Name = "test-resource"
	req.Namespace = "test-ns"

	result := d.DebounceRequest("test-key", req, func(r ctrl.Request) {
		receivedReq = r
	})

	assert.True(t, result)
	assert.True(t, d.IsPending("test-key"))

	// Wait for debounce to complete
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, "test-resource", receivedReq.Name)
	assert.Equal(t, "test-ns", receivedReq.Namespace)
}

func TestDebouncer_Cancel(t *testing.T) {
	d := NewDebouncer(100 * time.Millisecond)
	var called int32

	d.Debounce("test-key", func() {
		atomic.AddInt32(&called, 1)
	})

	assert.True(t, d.IsPending("test-key"))

	// Cancel before it fires
	result := d.Cancel("test-key")
	assert.True(t, result)
	assert.False(t, d.IsPending("test-key"))

	// Wait and verify it didn't fire
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int32(0), atomic.LoadInt32(&called))
}

func TestDebouncer_Cancel_NonExistent(t *testing.T) {
	d := NewDebouncer(50 * time.Millisecond)

	// Cancel non-existent key
	result := d.Cancel("non-existent")
	assert.False(t, result)
}

func TestDebouncer_Flush(t *testing.T) {
	d := NewDebouncer(1 * time.Second) // Long delay

	d.Debounce("key1", func() {})
	d.Debounce("key2", func() {})
	d.Debounce("key3", func() {})

	assert.Equal(t, 3, d.PendingCount())

	// Flush all pending
	d.Flush()

	assert.Equal(t, 0, d.PendingCount())
	assert.False(t, d.IsPending("key1"))
	assert.False(t, d.IsPending("key2"))
	assert.False(t, d.IsPending("key3"))
}

func TestDebouncer_IsPending_NonExistent(t *testing.T) {
	d := NewDebouncer(50 * time.Millisecond)
	assert.False(t, d.IsPending("non-existent"))
}

func TestDebouncer_ConcurrentAccess(t *testing.T) {
	d := NewDebouncer(10 * time.Millisecond)
	var callCount int32

	// Simulate concurrent access from multiple goroutines
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				d.Debounce("shared-key", func() {
					atomic.AddInt32(&callCount, 1)
				})
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete their debounce calls
	for i := 0; i < 10; i++ {
		<-done
	}

	// Wait for the final debounced call to complete
	time.Sleep(50 * time.Millisecond)

	// Should have called at least once but far fewer than 1000 times
	count := atomic.LoadInt32(&callCount)
	assert.GreaterOrEqual(t, count, int32(1))
	assert.Less(t, count, int32(100)) // Significant reduction from 1000 calls
}

func TestDebouncer_TimerReset(t *testing.T) {
	d := NewDebouncer(50 * time.Millisecond)
	var called int32
	var callTime time.Time

	startTime := time.Now()

	// First call
	d.Debounce("test-key", func() {
		atomic.AddInt32(&called, 1)
		callTime = time.Now()
	})

	// Wait a bit but not long enough for debounce to fire
	time.Sleep(30 * time.Millisecond)

	// Second call - should reset the timer
	d.Debounce("test-key", func() {
		atomic.AddInt32(&called, 1)
		callTime = time.Now()
	})

	// Wait for debounce to complete
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, int32(1), atomic.LoadInt32(&called))

	// The call should have happened after the second debounce (at least 80ms from start)
	elapsed := callTime.Sub(startTime)
	assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(70))
}

func TestDefaultDebounceDelay(t *testing.T) {
	assert.Equal(t, 500*time.Millisecond, DefaultDebounceDelay)
}
