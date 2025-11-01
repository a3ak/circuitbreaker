package circuitbreaker

import (
	"sync"
	"testing"
	"time"
)

func TestInitCircuitBreakers(t *testing.T) {

	servers := []string{"server1", "server2", "server3"}
	cfg := CircuitBreakerConf{
		FailureThreshold: 3,
		RecoveryTimeout:  10 * time.Second,
		SuccessThreshold: 2,
		HalfOpenPrc:      30,
	}

	m := NewCBManager()

	m.InitCircuitBreakers(servers, cfg)

	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.breakers) != len(servers) {
		t.Errorf("Expected %d circuit breakers, got %d", len(servers), len(m.breakers))
	}

	for _, server := range servers {
		if cb, exists := m.breakers[server]; !exists || cb == nil {
			t.Errorf("Circuit breaker for server %s not found or is nil", server)
		}
	}
}

func TestGetCircuitBreaker(t *testing.T) {
	servers := []string{"test-server"}
	cfg := CircuitBreakerConf{
		FailureThreshold: 2,
		RecoveryTimeout:  5 * time.Second,
	}

	m := NewCBManager()
	m.InitCircuitBreakers(servers, cfg)

	// Тест существующего сервера
	cb := m.GetCircuitBreaker("test-server")

	if cb == nil {
		t.Error("Expected circuit breaker, got nil")
	}

	// Тест несуществующего сервера
	cb = m.GetCircuitBreaker("non-existent-server")
	if cb != nil {
		t.Error("Expected nil for non-existent server")
	}
}

func TestAllowRequest(t *testing.T) {

	servers := []string{"test-server"}
	cfg := CircuitBreakerConf{
		FailureThreshold: 1,
		RecoveryTimeout:  200 * time.Millisecond,
		HalfOpenPrc:      100, // 100% для предсказуемости тестов
	}

	m := NewCBManager()

	m.InitCircuitBreakers(servers, cfg)

	// Тест: CB не настроен для сервера (должен разрешить запрос)
	allowed, _ := m.AllowRequest("unknown-server")
	if !allowed {
		t.Error("Expected allowed request for server without CB")
	}

	// Тест: CB в закрытом состоянии (должен разрешить запрос)
	allowed, _ = m.AllowRequest("test-server")
	if !allowed {
		t.Error("Expected allowed request for closed CB")
	}

	// Переводим CB в открытое состояние
	for i := 0; i < cfg.FailureThreshold; i++ {
		m.ReportFailure("test-server")
	}

	// Тест: CB в открытом состоянии (должен запретить запрос)
	allowed, _ = m.AllowRequest("test-server")
	if allowed {
		t.Error("Expected denied request for open CB")
	}

	// Ждем истечения recovery timeout
	time.Sleep(cfg.RecoveryTimeout + 50*time.Millisecond)

	// Тест: CB перешел в half-open после таймаута
	allowed, _ = m.AllowRequest("test-server")
	if !allowed {
		t.Error("Expected allowed request for half-open CB")
	}
}

func TestReportSuccess(t *testing.T) {

	servers := []string{"test-server"}
	cfg := CircuitBreakerConf{
		FailureThreshold: 1,
		RecoveryTimeout:  100 * time.Millisecond,
		SuccessThreshold: 2,
		HalfOpenPrc:      100,
	}

	m := NewCBManager()

	m.InitCircuitBreakers(servers, cfg)

	// Переводим CB в half-open состояние
	m.ReportFailure("test-server") // failureCount = 1 → open
	time.Sleep(110 * time.Millisecond)
	_, _ = m.AllowRequest("test-server") // Переводит в half-open

	// Сообщаем об успехах
	m.ReportSuccess("test-server") // successCount = 1
	m.ReportSuccess("test-server") // successCount = 2 → closed

	// Проверяем, что CB вернулся в closed состояние
	cb := m.GetCircuitBreaker("test-server")
	if cb.curState() != stateClosed {
		t.Errorf("Expected closed state, got %v", cb.curState())
	}

	// Тест: ReportSuccess для сервера без CB (не должно паниковать)
	m.ReportSuccess("unknown-server")
}

func TestReportFailure(t *testing.T) {

	servers := []string{"test-server"}
	cfg := CircuitBreakerConf{
		FailureThreshold: 2,
		RecoveryTimeout:  100 * time.Millisecond,
		HalfOpenPrc:      100,
	}

	m := NewCBManager()

	m.InitCircuitBreakers(servers, cfg)

	// Сообщаем о неудачах
	m.ReportFailure("test-server") // failureCount = 1
	if m.GetCircuitBreaker("test-server").curState() != stateClosed {
		t.Error("Expected closed state after first failure")
	}

	m.ReportFailure("test-server") // failureCount = 2 → open
	if m.GetCircuitBreaker("test-server").curState() != stateOpen {
		t.Error("Expected open state after threshold failures")
	}

	// Переводим в half-open
	time.Sleep(110 * time.Millisecond)
	_, _ = m.AllowRequest("test-server")

	// Неудача в half-open возвращает в open
	m.ReportFailure("test-server")
	if m.GetCircuitBreaker("test-server").curState() != stateOpen {
		t.Error("Expected open state after failure in half-open")
	}

	// Тест: ReportFailure для сервера без CB (не должно паниковать)
	m.ReportFailure("unknown-server")
}

func TestGetCircuitBreakerStats(t *testing.T) {
	servers := []string{"server1", "server2"}
	cfg := CircuitBreakerConf{
		FailureThreshold: 3,
		RecoveryTimeout:  30 * time.Second,
	}

	m := NewCBManager()
	m.InitCircuitBreakers(servers, cfg)

	stats := m.GetCircuitBreakerStats()

	if len(stats) != len(servers) {
		t.Errorf("Expected stats for %d servers, got %d", len(servers), len(stats))
	}

	for _, server := range servers {
		if serverStats, exists := stats[server]; !exists {
			t.Errorf("Stats for server %s not found", server)
		} else {
			statsMap := serverStats.(map[string]any)
			if statsMap["name"] != server {
				t.Errorf("Expected name %s, got %s", server, statsMap["name"])
			}
		}
	}
}

func TestGetCircuitBreakerState(t *testing.T) {
	servers := []string{"test-server"}
	cfg := CircuitBreakerConf{
		FailureThreshold: 1,
		RecoveryTimeout:  100 * time.Millisecond,
	}

	m := NewCBManager()
	m.InitCircuitBreakers(servers, cfg)

	// Тест: сервер без CB
	state := m.GetCircuitBreakerState("unknown-server")
	if state != "disabled" {
		t.Errorf("Expected 'disabled', got '%s'", state)
	}

	// Тест: закрытое состояние
	state = m.GetCircuitBreakerState("test-server")
	if state != "closed" {
		t.Errorf("Expected 'closed', got '%s'", state)
	}

	// Тест: открытое состояние
	m.ReportFailure("test-server")
	state = m.GetCircuitBreakerState("test-server")
	if state != "open" {
		t.Errorf("Expected 'open', got '%s'", state)
	}

	// Тест: half-open состояние
	time.Sleep(110 * time.Millisecond)
	_, _ = m.AllowRequest("test-server") // Переводит в half-open
	state = m.GetCircuitBreakerState("test-server")
	if state != "half-open" {
		t.Errorf("Expected 'half-open', got '%s'", state)
	}
}

func TestConcurrentAccess(t *testing.T) {
	servers := []string{"concurrent-server"}
	cfg := CircuitBreakerConf{
		FailureThreshold: 10,
		RecoveryTimeout:  50 * time.Millisecond,
		HalfOpenPrc:      50,
	}

	m := NewCBManager()

	m.InitCircuitBreakers(servers, cfg)

	var wg sync.WaitGroup
	iterations := 100

	// Многопоточный доступ к CB
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(iteration int) {
			defer wg.Done()

			// Чередуем успешные и неуспешные запросы
			if iteration%2 == 0 {
				allowed, _ := m.AllowRequest("concurrent-server")
				if allowed {
					m.ReportSuccess("concurrent-server")
				}
			} else {
				allowed, _ := m.AllowRequest("concurrent-server")
				if allowed {
					m.ReportFailure("concurrent-server")
				}
			}

			// Читаем состояние
			_ = m.GetCircuitBreakerState("concurrent-server")
			_ = m.GetCircuitBreakerStats()
		}(i)
	}

	wg.Wait()

	// После всех операций CB должен быть в валидном состоянии
	cb := m.GetCircuitBreaker("concurrent-server")
	state := cb.curState()
	if state != stateClosed && state != stateOpen && state != stateHalfOpen {
		t.Errorf("Invalid state after concurrent access: %v", state)
	}
}

func TestEdgeCases(t *testing.T) {
	// Тест: инициализация с пустым списком серверов
	m := NewCBManager()
	m.InitCircuitBreakers([]string{}, CircuitBreakerConf{})
	stats := m.GetCircuitBreakerStats()
	if len(stats) != 0 {
		t.Error("Expected no circuit breakers for empty servers list")
	}

	// Тест: инициализация с nil конфигом
	m.InitCircuitBreakers([]string{"test"}, CircuitBreakerConf{})
	cb := m.GetCircuitBreaker("test")
	if cb == nil {
		t.Error("Expected circuit breaker with default config")
	}
}
