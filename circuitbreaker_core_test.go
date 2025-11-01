package circuitbreaker

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Helper to create a CircuitBreaker for testing
func newTestCB(name string, FailureThreshold int, recoveryTimeout time.Duration, successThreshold int, halfOpenPrc int) (*circuitBreaker, error) {
	config := CircuitBreakerConf{
		FailureThreshold: FailureThreshold,
		RecoveryTimeout:  recoveryTimeout,
		SuccessThreshold: successThreshold,
		HalfOpenPrc:      halfOpenPrc,
	}
	return new(name, config)
}

func TestCircuitBreaker_ClosedState(t *testing.T) {
	cb, _ := newTestCB("test", 3, 1*time.Second, 2, 50)

	// Test allow in Closed
	allowed, _ := cb.allow()
	if !allowed {
		t.Error("Expected allow to return true in Closed state")
	}

	// Test failure accumulation
	cb.failure()
	cb.failure()
	if cb.curState() != stateClosed {
		t.Error("Expected state to remain Closed after 2 failures")
	}
	cb.failure()
	if cb.curState() != stateOpen {
		t.Error("Expected state to be Open after 3 failures")
	}

	// Test success resets failure count
	cb, _ = newTestCB("test", 3, 100*time.Millisecond, 2, 50)
	cb.failure()
	cb.failure()
	cb.success()
	if cb.failureCount != 1 {
		t.Error("Expected failure count to be 1 after success")
	}
	cb.failure()
	cb.failure()
	time.Sleep(150 * time.Millisecond)
	_, st := cb.allow()
	if st != stateHalfOpen {
		t.Errorf("Expected state to be HalfOpen after timeout. Now State: %s\n", st)
	}
}

func TestCircuitBreaker_OpenState(t *testing.T) {
	cb, _ := newTestCB("test", 2, 100*time.Millisecond, 2, 50)

	// Force to Open
	cb.failure()
	cb.failure()

	// Test allow returns false
	allowed, _ := cb.allow()
	if allowed {
		t.Error("Expected allow to return false in Open state")
	}

	// Wait for recovery timeout
	time.Sleep(150 * time.Millisecond)

	// Now allow should transition to Half-Open and return based on halfOpenPrc
	// Since it's random, run multiple times and check statistically
	allowedCount := 0
	total := 100
	for i := 0; i < total; i++ {
		a, _ := cb.allow()
		if a {
			allowedCount++
		}
	}
	percentage := float64(allowedCount) / float64(total) * 100
	if percentage < 30 || percentage > 70 { // allow some variance
		t.Errorf("Expected ~50%% allowed in Half-Open, got %.2f%%", percentage)
	}
}

func TestCircuitBreaker_HalfOpenState(t *testing.T) {
	cb, _ := newTestCB("test", 2, 1*time.Second, 2, 50)

	// Force to Half-Open
	cb.mu.Lock()
	cb.state = stateHalfOpen
	cb.mu.Unlock()

	// Test allow: Check percentage
	allowed := 0
	total := 1000
	for i := 0; i < total; i++ {
		a, _ := cb.allow()
		if a {
			allowed++
		}
	}
	percentage := float64(allowed) / float64(total) * 100
	if percentage < 40 || percentage > 60 {
		t.Errorf("Expected ~50%% allowed in Half-Open, got %.2f%%", percentage)
	}

	// Test success transition
	cb.success()
	if cb.curState() != stateHalfOpen {
		t.Error("Expected state to remain Half-Open after 1 success")
	}
	cb.success()
	if cb.curState() != stateClosed {
		t.Error("Expected state to be Closed after 2 successes")
	}

	// Test failure transition
	cb, _ = newTestCB("test", 2, 1*time.Second, 2, 50)
	cb.mu.Lock()
	cb.state = stateHalfOpen
	cb.mu.Unlock()
	cb.failure()
	if cb.curState() != stateOpen {
		t.Error("Expected state to be Open after failure in Half-Open")
	}
}

func TestCircuitBreaker_StatsAndState(t *testing.T) {
	cb, _ := newTestCB("test", 3, 1*time.Second, 2, 50)

	// Test State
	if cb.curState() != stateClosed {
		t.Error("Expected initial state to be Closed")
	}

	// Test Stats
	stats := cb.stats()
	if stats["state"] != "closed" || stats["name"] != "test" {
		t.Error("Stats not matching expected values")
	}
}

func TestCircuitBreaker_ConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		srvName string
		config  CircuitBreakerConf
		wantErr bool
		want    *circuitBreaker
	}{
		{
			name:    "empty name",
			srvName: "",
			config: CircuitBreakerConf{
				FailureThreshold: 3,
				RecoveryTimeout:  time.Second,
			},
			wantErr: true,
		},
		{
			name:    "negative failure threshold",
			srvName: "test-cb",
			config: CircuitBreakerConf{
				FailureThreshold: -1,
				RecoveryTimeout:  time.Second,
			},
			want: &circuitBreaker{
				recoveryTimeout: 1 * time.Second,
			},
			wantErr: false,
		},
		{
			name:    "zero recovery timeout",
			srvName: "test-cb",
			config: CircuitBreakerConf{
				FailureThreshold: 3,
				RecoveryTimeout:  0,
			},
			want: &circuitBreaker{
				recoveryTimeout: 30 * time.Second,
			},
			wantErr: false,
		},
		{
			name:    "valid config",
			srvName: "test-cb",
			config: CircuitBreakerConf{
				FailureThreshold: 3,
				RecoveryTimeout:  2 * time.Second,
				SuccessThreshold: 2,
				HalfOpenPrc:      50,
			},
			want: &circuitBreaker{
				recoveryTimeout: 2 * time.Second,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fmt.Println("Running test:", tt.name)
			got, err := new(tt.srvName, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("new() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got != nil && got.recoveryTimeout != tt.want.recoveryTimeout {
				t.Errorf("new() recoveryTimeout = %v, want %v",
					got.recoveryTimeout, tt.want.recoveryTimeout)
			}
		})
	}
}

func TestCircuitBreaker_Concurrency(t *testing.T) {
	cb, _ := newTestCB("test", 5, 1*time.Second, 3, 50)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, _ = cb.allow()
				cb.success()
			}
		}()
	}
	wg.Wait()

	// Just ensure no panics or deadlocks
	if cb.curState() != stateClosed {
		t.Log("State may have changed due to concurrency, but no errors")
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cb, _ := newTestCB("test", 5, time.Second, 3, 50)

	// Количество горутин и операций
	workers := 10
	operations := 1000

	var (
		successCount int32
		failureCount int32
		wg           sync.WaitGroup
	)

	// Запускаем горутины для параллельного выполнения операций
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				if allowed, _ := cb.allow(); allowed {
					if j%2 == 0 {
						cb.success()
						atomic.AddInt32(&successCount, 1)
					} else {
						cb.failure()
						atomic.AddInt32(&failureCount, 1)
					}
				}
				// Добавляем небольшую задержку для лучшего перемешивания
				time.Sleep(time.Microsecond)
			}
		}()
	}

	wg.Wait()

	stats := cb.stats()
	t.Logf("Final state: %v", stats["state"])
	t.Logf("Success count: %d", successCount)
	t.Logf("Failure count: %d", failureCount)
}

func TestCircuitBreaker_StatsAccuracy(t *testing.T) {
	cb, _ := newTestCB("test", 3, 50*time.Microsecond, 2, 100)

	// Выполняем последовательность операций
	operations := []struct {
		action string
		sleep  time.Duration
	}{
		{"fail", 0},
		{"fail", 0},
		{"fail", 0},
		{"wait", 1100 * time.Millisecond},
		{"allow", 0},
		{"success", 0},
		{"success", 0},
		{"fail", 0},
	}

	for _, op := range operations {
		switch op.action {
		case "fail":
			cb.failure()
		case "success":
			cb.success()
		case "wait":
			time.Sleep(op.sleep)
		case "allow":
			cb.allow()
		}
	}

	stats := cb.stats()

	// Проверяем все поля статистики
	if stats["failure_count"].(int) != 1 {
		t.Errorf("failure_count = %v, want 1", stats["failure_count"])
	}

	if stats["transaction"].(int) < 2 {
		t.Error("expected at least one transaction")
	}

	lastFailure := stats["last_failure_time"].(time.Time)
	if time.Since(lastFailure) > 2*time.Second {
		t.Error("last_failure_time seems incorrect")
	}
}
