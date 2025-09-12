package circuitbreaker

import (
	"math/rand/v2"
	"sync"
	"time"

	"github.com/nir0k/logger"
)

// Структура для конфигурации Circuit Breaker
type CircuitBreakerConf struct {
	FailureThreshold int           `yaml:"failure_threshold"`
	RecoveryTimeout  time.Duration `yaml:"recovery_timeout"`
	SuccessThreshold int           `yaml:"success_threshold"`
	HalfOpenPrc      int           `yaml:"half_open_prc"` // Процент пропускаемых запросов
}

// State представляет состояние Circuit Breaker
type State int

// Возможные состояния
const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

var (
	CircuitBreakers   map[string]*CircuitBreaker
	circuitBreakersMu sync.RWMutex
)

// CircuitBreaker реализует паттерн Circuit Breaker
type CircuitBreaker struct {
	mu               sync.RWMutex
	state            State
	failureCount     int
	failureThreshold int
	recoveryTimeout  time.Duration
	lastFailureTime  time.Time
	successCount     int
	successThreshold int
	name             string
	halfOpenPrc      int //процент пропускаемых запросов
	transaction      int //количество переходв из состояния close в open
}

// New создает новый Circuit Breaker
func New(name string, config CircuitBreakerConf) *CircuitBreaker {
	if config.SuccessThreshold == 0 {
		config.SuccessThreshold = 3
	}

	if config.RecoveryTimeout == 0 {
		config.RecoveryTimeout = 30 * time.Second
	}

	if config.HalfOpenPrc <= 0 {
		config.HalfOpenPrc = 20
	} else if config.HalfOpenPrc > 100 {
		config.HalfOpenPrc = 100
	}

	return &CircuitBreaker{
		state:            StateClosed,
		failureThreshold: config.FailureThreshold,
		recoveryTimeout:  config.RecoveryTimeout,
		successThreshold: config.SuccessThreshold,
		name:             name,
		halfOpenPrc:      config.HalfOpenPrc,
	}
}

// Allow проверяет, разрешено ли выполнение запроса
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.RLock()
	state := cb.state
	lastFailureTime := cb.lastFailureTime
	recoveryTimeout := cb.recoveryTimeout
	halfOpenPrc := cb.halfOpenPrc
	name := cb.name
	cb.mu.RUnlock()

	switch state {
	case StateClosed:
		return true
	case StateHalfOpen:
		return rand.IntN(100) < halfOpenPrc
	case StateOpen:
		if time.Since(lastFailureTime) >= recoveryTimeout {
			cb.mu.Lock()
			// Повторная проверка, чтобы избежать гонки
			if cb.state == StateOpen && time.Since(cb.lastFailureTime) >= cb.recoveryTimeout {
				cb.state = StateHalfOpen
				logger.Infof("Circuit breaker %s moved to half-open state", name)
			}
			cb.mu.Unlock()
			// В half-open состоянии пропускаем только часть запросов
			return rand.IntN(100) < halfOpenPrc
		}
		return false
	default:
		return false
	}
}

// Success отмечает успешное выполнение запроса
func (cb *CircuitBreaker) Success() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		// Декрементируем счетчик ошибок при успешных запросах
		if cb.failureCount > 0 {
			cb.failureCount--
		}
	case StateHalfOpen:
		cb.successCount++
		if cb.successCount >= cb.successThreshold {
			cb.state = StateClosed
			cb.failureCount = 0
			cb.successCount = 0
			logger.Infof("Circuit breaker %s moved to closed state", cb.name)
			cb.transaction++
		}
	}
}

// Failure отмечает неудачное выполнение запроса
func (cb *CircuitBreaker) Failure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		cb.failureCount++
		if cb.failureCount >= cb.failureThreshold {
			cb.state = StateOpen
			cb.lastFailureTime = time.Now()
			logger.Warningf("Circuit breaker %s tripped to open state", cb.name)
			cb.transaction++
		}
	case StateHalfOpen:
		// В half-open состоянии любая ошибка возвращает в open
		cb.state = StateOpen
		cb.lastFailureTime = time.Now()
		cb.successCount = 0
		logger.Warningf("Circuit breaker %s moved back to open state", cb.name)
	}
}

// State возвращает текущее состояние
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Stats возвращает статистику
func (cb *CircuitBreaker) Stats() map[string]any {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return map[string]any{
		"state":             cb.state.String(),
		"failure_count":     cb.failureCount,
		"success_count":     cb.successCount,
		"last_failure_time": cb.lastFailureTime,
		"name":              cb.name,
		"transaction":       cb.transaction,
	}
}

// String возвращает текстовое представление состояния
func (s State) String() string {
	switch s {
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
