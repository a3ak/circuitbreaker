package circuitbreaker

import "github.com/nir0k/logger"

// InitCircuitBreakers инициализирует Circuit Breakers для серверов
func InitCircuitBreakers(servers []string, cfg CircuitBreakerConf) {
	circuitBreakersMu.Lock()
	defer circuitBreakersMu.Unlock()

	CircuitBreakers = make(map[string]*CircuitBreaker)

	for _, srv := range servers {
		cb := New(srv, cfg)
		CircuitBreakers[srv] = cb
	}
}

// GetCircuitBreaker возвращает Circuit Breaker для сервера
func GetCircuitBreaker(serverURL string) *CircuitBreaker {
	circuitBreakersMu.RLock()
	defer circuitBreakersMu.RUnlock()

	return CircuitBreakers[serverURL]
}

// AllowRequest проверяет, разрешен ли запрос к серверу
func AllowRequest(serverURL string) bool {
	cb := GetCircuitBreaker(serverURL)
	if cb == nil {
		logger.Warningf("Circuit breaker not configured for %s, allowing request", serverURL)
		return true // Если CB не настроен, разрешаем запрос
	}

	allowed := cb.Allow()
	state := cb.State()

	if state == StateHalfOpen {
		logger.Debugf("Circuit breaker %s: half-open state, request allowed: %t",
			serverURL, allowed)
	}

	return allowed
}

// ReportSuccess отмечает успешный запрос
func ReportSuccess(serverURL string) {
	cb := GetCircuitBreaker(serverURL)
	if cb != nil {
		cb.Success()
	}
}

// ReportFailure отмечает неудачный запрос
func ReportFailure(serverURL string) {
	cb := GetCircuitBreaker(serverURL)
	if cb != nil {
		cb.Failure()
	}
}

// GetCircuitBreakerStats возвращает статистику всех Circuit Breakers
func GetCircuitBreakerStats() map[string]any {
	circuitBreakersMu.RLock()
	defer circuitBreakersMu.RUnlock()

	stats := make(map[string]any)
	for srv, cb := range CircuitBreakers {
		stats[srv] = cb.Stats()
	}
	return stats
}

// GetCircuitBreakerstate возвращает текстовое состояние Circuit Breaker
func GetCircuitBreakerState(serverURL string) string {
	cb := GetCircuitBreaker(serverURL)
	if cb == nil {
		return "disabled"
	}

	state := cb.State()
	switch state {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}
