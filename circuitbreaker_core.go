// Circuit Breaker паттерн для управления отказами в распределенных системах.
// Используется для предотвращения повторных неудачных запросов к сервисам, которые могут быть временно недоступны.
// Позволяет снизить нагрузку на проблемные сервисы.
// Имеет три состояния:
//
//						closed(замкнутое состояние, когда запросы пропускаются к сервису),
//	 					open (разомкнутое состо]ние, когда запросы не пропускаются к проблемному сервису)
//						half-open(частично разомкнутое состояние, когда часть запросов пропускается для проверки доступности сервиса).
package circuitbreaker

import (
	"errors"
	"math/rand/v2"
	"sync"
	"time"
)

// Структура для конфигурации Circuit Breaker
type CircuitBreakerConf struct {
	FailureThreshold int           `yaml:"failure_threshold"` // Количество неудач до срабатывания
	RecoveryTimeout  time.Duration `yaml:"recovery_timeout"`  // Время до попытки восстановления
	SuccessThreshold int           `yaml:"success_threshold"` // Количество успешных запросов для восстановления
	HalfOpenPrc      int           `yaml:"half_open_prc"`     // Процент пропускаемых запросов
}

// State представляет состояние Circuit Breaker
type State uint8

// Возможные состояния
const (
	stateClosed State = iota
	stateOpen
	stateHalfOpen
	notConfigured
)

// circuitBreaker реализует паттерн Circuit Breaker
type circuitBreaker struct {
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
func new(name string, config CircuitBreakerConf) (*circuitBreaker, error) {
	if name == "" {
		return nil, errors.New("circuit breaker name cannot be empty")
	}

	// Установка значений по умолчанию, если не заданы или заданы некорректно
	if config.SuccessThreshold <= 0 {
		config.SuccessThreshold = 3
	}

	if config.RecoveryTimeout <= 0 {
		config.RecoveryTimeout = 30 * time.Second
	}

	if config.FailureThreshold <= 0 {
		config.FailureThreshold = 5
	}

	if config.HalfOpenPrc <= 0 {
		config.HalfOpenPrc = 20
	} else if config.HalfOpenPrc > 100 {
		config.HalfOpenPrc = 100
	}

	return &circuitBreaker{
		state:            stateClosed,
		failureThreshold: config.FailureThreshold,
		recoveryTimeout:  config.RecoveryTimeout,
		successThreshold: config.SuccessThreshold,
		name:             name,
		halfOpenPrc:      config.HalfOpenPrc,
	}, nil
}

// Allow проверяет, разрешено ли выполнение запроса
func (cb *circuitBreaker) allow() (bool, State) {
	cb.mu.RLock()
	state := cb.state
	lastFailureTime := cb.lastFailureTime
	recoveryTimeout := cb.recoveryTimeout
	halfOpenPrc := cb.halfOpenPrc
	//name := cb.name
	cb.mu.RUnlock()

	switch state {
	case stateClosed:
		return true, state
	case stateHalfOpen:
		return rand.IntN(100) < halfOpenPrc, state
	case stateOpen:
		if time.Since(lastFailureTime) >= recoveryTimeout {
			cb.mu.Lock()
			defer cb.mu.Unlock()
			// Повторная проверка, чтобы избежать гонки
			if cb.state == stateOpen && time.Since(cb.lastFailureTime) >= cb.recoveryTimeout {
				cb.state = stateHalfOpen
			}

			// В half-open состоянии пропускаем только часть запросов
			return rand.IntN(100) < halfOpenPrc, stateHalfOpen
		}
		return false, state
	default:
		return false, state
	}
}

// Success отмечает успешное выполнение запроса
func (cb *circuitBreaker) success() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case stateClosed:
		// Декрементируем счетчик ошибок при успешных запросах
		if cb.failureCount > 0 {
			cb.failureCount--
		}
	case stateHalfOpen:
		// В half-open состоянии считаем успешные запросы
		cb.successCount++
		// Если достигнут порог успешных запросов, переходим в closed
		if cb.successCount >= cb.successThreshold {
			cb.state = stateClosed
			cb.failureCount = 0
			cb.successCount = 0
			cb.transaction++
		}
	}
}

// Failure отмечает неудачное выполнение запроса
func (cb *circuitBreaker) failure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case stateClosed:
		cb.failureCount++
		// Если достигнут порог ошибок, переходим в open
		if cb.failureCount >= cb.failureThreshold {
			cb.state = stateOpen
			cb.lastFailureTime = time.Now()
			//Инициализируем счетчики переходов состояний
			cb.transaction++
		}
	case stateHalfOpen:
		// В half-open состоянии любая ошибка возвращает в open
		cb.state = stateOpen
		cb.lastFailureTime = time.Now()
		cb.successCount = 0
	}
}

// State возвращает текущее состояние
func (cb *circuitBreaker) curState() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Stats возвращает статистику
func (cb *circuitBreaker) stats() map[string]any {
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
}
