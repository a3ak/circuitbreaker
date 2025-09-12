package circuitbreaker

import (
	"sync"
	"testing"
	"time"
)

// Helper to create a CircuitBreaker for testing
func newTestCB(name string, failureThreshold int, recoveryTimeout time.Duration, successThreshold int, halfOpenPrc int) *CircuitBreaker {
	config := CircuitBreakerConf{
		FailureThreshold: failureThreshold,
		RecoveryTimeout:  recoveryTimeout,
		SuccessThreshold: successThreshold,
		HalfOpenPrc:      halfOpenPrc,
	}
	return New(name, config)
}

func TestCircuitBreaker_ClosedState(t *testing.T) {
	cb := newTestCB("test", 3, 1*time.Second, 2, 50)

	// Test Allow in Closed
	if !cb.Allow() {
		t.Error("Expected Allow to return true in Closed state")
	}

	// Test failure accumulation
	cb.Failure()
	cb.Failure()
	if cb.State() != StateClosed {
		t.Error("Expected state to remain Closed after 2 failures")
	}
	cb.Failure()
	if cb.State() != StateOpen {
		t.Error("Expected state to be Open after 3 failures")
	}

	// Test success resets failure count
	cb = newTestCB("test", 3, 100*time.Millisecond, 2, 50)
	cb.Failure()
	cb.Failure()
	cb.Success()
	if cb.failureCount != 1 {
		t.Error("Expected failure count to be 1 after success")
	}
	cb.Failure()
	cb.Failure()
	time.Sleep(150 * time.Millisecond)
	if cb.Allow(); cb.State() != StateHalfOpen {
		t.Errorf("Expected state to be HalfOpen after 2 seconds. Now State: %s\n", cb.State())
	}
}

func TestCircuitBreaker_OpenState(t *testing.T) {
	cb := newTestCB("test", 2, 100*time.Millisecond, 2, 50)

	// Force to Open
	cb.Failure()
	cb.Failure()

	// Test Allow returns false
	if cb.Allow() {
		t.Error("Expected Allow to return false in Open state")
	}

	// Wait for recovery timeout
	time.Sleep(150 * time.Millisecond)

	// Now Allow should transition to Half-Open and return based on halfOpenPrc
	// Since it's random, run multiple times and check statistically
	allowed := 0
	total := 100
	for i := 0; i < total; i++ {
		if cb.Allow() {
			allowed++
		}
	}
	percentage := float64(allowed) / float64(total) * 100
	if percentage < 30 || percentage > 70 { // Allow some variance
		t.Errorf("Expected ~50%% allowed in Half-Open, got %.2f%%", percentage)
	}
}

func TestCircuitBreaker_HalfOpenState(t *testing.T) {
	cb := newTestCB("test", 2, 1*time.Second, 2, 50)

	// Force to Half-Open
	cb.mu.Lock()
	cb.state = StateHalfOpen
	cb.mu.Unlock()

	// Test Allow: Check percentage
	allowed := 0
	total := 1000
	for i := 0; i < total; i++ {
		if cb.Allow() {
			allowed++
		}
	}
	percentage := float64(allowed) / float64(total) * 100
	if percentage < 40 || percentage > 60 {
		t.Errorf("Expected ~50%% allowed in Half-Open, got %.2f%%", percentage)
	}

	// Test Success transition
	cb.Success()
	if cb.State() != StateHalfOpen {
		t.Error("Expected state to remain Half-Open after 1 success")
	}
	cb.Success()
	if cb.State() != StateClosed {
		t.Error("Expected state to be Closed after 2 successes")
	}

	// Test Failure transition
	cb = newTestCB("test", 2, 1*time.Second, 2, 50)
	cb.mu.Lock()
	cb.state = StateHalfOpen
	cb.mu.Unlock()
	cb.Failure()
	if cb.State() != StateOpen {
		t.Error("Expected state to be Open after failure in Half-Open")
	}
}

func TestCircuitBreaker_StatsAndState(t *testing.T) {
	cb := newTestCB("test", 3, 1*time.Second, 2, 50)

	// Test State
	if cb.State() != StateClosed {
		t.Error("Expected initial state to be Closed")
	}

	// Test Stats
	stats := cb.Stats()
	if stats["state"] != "closed" || stats["name"] != "test" {
		t.Error("Stats not matching expected values")
	}
}

func TestCircuitBreaker_ConfigValidation(t *testing.T) {
	cb := New("test", CircuitBreakerConf{FailureThreshold: 3, RecoveryTimeout: 0, SuccessThreshold: 0, HalfOpenPrc: 150})
	if cb.halfOpenPrc != 100 {
		t.Error("Expected halfOpenPrc to be clamped to 100")
	}

	cb = New("test", CircuitBreakerConf{FailureThreshold: 3, RecoveryTimeout: 0, SuccessThreshold: 0, HalfOpenPrc: -1})
	if cb.halfOpenPrc != 20 {
		t.Error("Expected halfOpenPrc to be set to 20")
	}

	cb = New("test", CircuitBreakerConf{FailureThreshold: 3, RecoveryTimeout: 0, SuccessThreshold: 0, HalfOpenPrc: 1})
	if cb.halfOpenPrc != 1 {
		t.Error("Expected halfOpenPrc to be set to 1")
	}
}

func TestCircuitBreaker_Concurrency(t *testing.T) {
	cb := newTestCB("test", 5, 1*time.Second, 3, 50)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				cb.Allow()
				cb.Success()
			}
		}()
	}
	wg.Wait()

	// Just ensure no panics or deadlocks
	if cb.State() != StateClosed {
		t.Log("State may have changed due to concurrency, but no errors")
	}
}
