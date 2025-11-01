package circuitbreaker

import "sync"

type CBManager struct {
	breakers map[string]*circuitBreaker
	mu       sync.RWMutex
}

// NewManager создает новый менеджер circuit breakers
func NewCBManager() *CBManager {
	return &CBManager{
		breakers: make(map[string]*circuitBreaker),
	}
}

// InitCircuitBreakers инициализирует Circuit Breakers для серверов
func (m *CBManager) InitCircuitBreakers(servers []string, cfg CircuitBreakerConf) (cbInitErr []error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, srv := range servers {
		cb, err := new(srv, cfg)
		if cb == nil && err != nil {
			cbInitErr = append(cbInitErr, err)
			continue
		}
		m.breakers[srv] = cb
	}
	return cbInitErr
}

// GetCircuitBreaker возвращает Circuit Breaker для сервера
func (m *CBManager) GetCircuitBreaker(serverURL string) *circuitBreaker {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.breakers[serverURL]
}

// AllowRequest проверяет, разрешен ли запрос к серверу
func (m *CBManager) AllowRequest(serverURL string) (bool, State) {
	cb := m.GetCircuitBreaker(serverURL)
	if cb == nil {
		//logger.Warningf("Circuit breaker not configured for %s, allowing request", serverURL)
		return true, notConfigured // Если CB не настроен, разрешаем запрос
	}
	return cb.allow()

	/*
		allowed := cb.Allow()
		state := cb.State()

		if state == StateHalfOpen {
			logger.Debugf("Circuit breaker %s: half-open state, request allowed: %t",
				serverURL, allowed)
		}

		return allowed
	*/
}

// ReportSuccess отмечает успешный запрос
func (m *CBManager) ReportSuccess(serverURL string) {
	cb := m.GetCircuitBreaker(serverURL)
	if cb != nil {
		cb.success()
	}
}

// ReportFailure отмечает неудачный запрос
func (m *CBManager) ReportFailure(serverURL string) {
	cb := m.GetCircuitBreaker(serverURL)
	if cb != nil {
		cb.failure()
	}
}

// GetCircuitBreakerStats возвращает статистику всех Circuit Breakers
func (m *CBManager) GetCircuitBreakerStats() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]any)
	for srv, cb := range m.breakers {
		stats[srv] = cb.stats()
	}
	return stats
}

// GetCircuitBreakerstate возвращает текстовое состояние Circuit Breaker
func (m *CBManager) GetCircuitBreakerState(serverURL string) string {
	cb := m.GetCircuitBreaker(serverURL)
	if cb == nil {
		return "disabled"
	}
	return cb.curState().String()
	/*
		state := cb.curState()
		switch state {
		case stateClosed:
			return "closed"
		case stateOpen:
			return "open"
		case stateHalfOpen:
			return "half-open"
		case notConfigured:
			return "not configured"
		default:
			return "unknown"
		}

	*/
}
