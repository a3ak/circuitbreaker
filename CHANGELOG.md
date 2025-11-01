## Changelog

Current release: 0.2.0 — 2025-11-01  
Previous release: 0.1.0

### 0.2.0
- Переход на manager-based API:
  - Добавлен circuitbreaker.NewCBManager и набор методов для управления множеством CB.
  - Основные публичные методы: InitCircuitBreakers, AllowRequest, ReportSuccess, ReportFailure, GetCircuitBreakerStats, GetCircuitBreakerState.
- Конфигурация:
  - circuitbreaker.CircuitBreakerConf: FailureThreshold, RecoveryTimeout, SuccessThreshold, HalfOpenPrc.
  - Добавлена валидация и разумные значения по умолчанию.
- Надёжность:
  - Улучшена потокобезопасность через sync.RWMutex.
  - Исправлена логика переходов состояний и счётчиков в конкурентных сценариях.
- Тесты:
  - Добавлены и/или обновлены юнит-тесты для менеджера и конкурентных случаев.
- Мелкие исправления:
  - Улучшены текстовые представления состояний и обработка граничных значений.

### Миграционные заметки
- Ранее использовались прямые экземпляры CircuitBreaker. Теперь рекомендуется работать через менеджер:
  - Инициализация множества сервисов: mgr.InitCircuitBreakers([...], cfg)
  - Проверка и отчёт: mgr.AllowRequest(name) -> mgr.ReportSuccess(name) / mgr.ReportFailure(name)
- Если необходим прямой доступ к внутреннему экземпляру — смотрите исходники и тесты для примеров использования.
