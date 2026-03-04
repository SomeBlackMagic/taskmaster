# Taskmaster — План апгрейда

## Общий контекст

Taskmaster — минималистичный менеджер процессов на Go: запускает команду в бесконечном цикле и пробрасывает Linux-сигналы в дочерний процесс. Текущая реализация — MVP с рядом структурных, конкурентных и инфраструктурных проблем.

---

## Этап 1. Фундамент: модуль, зависимости, инфраструктура

### 1.1 Переименовать Go-модуль

**Файл:** `go.mod`

```
# было
module test

# стало
module github.com/SomeBlackMagic/taskmaster
```

**Обоснование:** Имя `test` — зарезервированное слово в Go-тулчейне. Это ломает `go test ./...` и импорты пакетов.

### 1.2 Обновить версию Go

```
# было
go 1.17

# стало
go 1.23
```

Убрать неиспользуемую зависимость `golang.org/x/crypto`.

### 1.3 Обновить Dockerfile

**Файл:** `.docker/Dockerfile`

Перейти на multi-stage build:

```dockerfile
# Stage 1: сборка
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/taskmaster ./cmd/taskmaster

# Stage 2: минимальный runtime
FROM scratch
COPY --from=builder /bin/taskmaster /bin/taskmaster
ENTRYPOINT ["/bin/taskmaster"]
```

**Что меняется:**
- `golang:1.16-alpine` → `golang:1.23-alpine`
- Multi-stage вместо single-stage (финальный образ ~5 MB вместо ~300 MB)
- `FROM scratch` — нет shell, нет лишних бинарей
- Кешированный `go mod download` ускоряет пересборку

### 1.4 Реструктурировать проект по стандартному layout

```
taskmaster/
├── cmd/
│   └── taskmaster/
│       └── main.go          # точка входа (тонкая)
├── internal/
│   └── runner/
│       ├── runner.go        # основная логика
│       └── runner_test.go   # unit-тесты
├── test/
│   └── testapp/
│       └── main.go          # тестовое приложение
├── docs/
│   └── plan.md
├── .docker/
│   └── Dockerfile
├── go.mod
├── go.sum
└── Makefile
```

---

## Этап 2. Конкурентность: устранение race conditions

### 2.1 Защита переменной `cmd` мьютексом

**Файл:** `src/main.go`, строки 12, 64, 95–97

Текущая проблема:

```go
// ПРОБЛЕМА: cmd пишется в горутине execute(), читается в main() без синхронизации
var cmd *exec.Cmd

// goroutine execute():
cmd = exec.Command(...)   // запись

// main() select-loop:
if cmd == nil { continue } // чтение — data race!
if err := cmd.Process.Signal(sig); err != nil { ... }
```

Решение — инкапсулировать в struct с `sync.RWMutex`:

```go
type Runner struct {
    mu  sync.RWMutex
    cmd *exec.Cmd
}

func (r *Runner) setCmd(c *exec.Cmd) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.cmd = c
}

func (r *Runner) getCmd() *exec.Cmd {
    r.mu.RLock()
    defer r.mu.RUnlock()
    return r.cmd
}
```

### 2.2 Использовать буферизованный канал bus

```go
// было: deadlock если никто не читает
bus = make(chan int)

// стало: не блокирует горутину execute при завершении
bus = make(chan int, 1)
```

### 2.3 Использовать `context.Context` для управления жизненным циклом

Заменить флаг `isShutdown bool` на `context.WithCancel`:

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

go runner.Execute(ctx, command)
```

При получении сигнала завершения — вызывать `cancel()`. Горутина `execute` проверяет `ctx.Done()` перед каждым рестартом.

---

## Этап 3. Качество кода: рефакторинг

### 3.1 Убрать глобальные переменные

**Было:**
```go
var (
    cmd     *exec.Cmd
    bus     chan int
    signals chan os.Signal
    ...
)
```

**Стало:** всё состояние — поля struct `Runner`. Глобальных переменных нет.

### 3.2 Исправить слайс аргументов

**Было (строки 39–40):**
```go
command := os.Args
command = append(command[:0], command[1:]...)
```

**Стало:**
```go
command := os.Args[1:]
```

Оба варианта дают одинаковый результат, но первый выделяет лишнюю память и мутирует `os.Args`.

### 3.3 Исправить обработку сигналов

**Проблема 1:** `os.Kill` нельзя перехватить — ядро не даёт перехватить SIGKILL userspace-процессам. `signal.Notify` молча игнорирует эту подписку.

```go
// убрать из списка listen:
os.Kill,
syscall.SIGKILL,
```

**Проблема 2:** Дублирующая подписка через цикл.

```go
// было: каждый сигнал отдельным вызовом в цикле
for _, s := range listen {
    signal.Notify(signals, s)
}

// стало: один вызов
signal.Notify(signals, listen...)
```

**Проблема 3:** `WarningLogger.Fatalf` при ошибке проброса сигнала завершает процесс немедленно — это неправильно. Нужен `log.Printf` + graceful shutdown.

### 3.4 Исправить `statusCode()` и `execute()`

Текущая логика некорректна:

```go
// строка 108: cmd.Wait() вызывается ПОСЛЕ cmd.Output(), который уже ждёт завершения
if code := statusCode(cmd); code != 0 { ... }
if err := cmd.Wait(); err != nil { ... }  // никогда не вернёт ошибку — процесс уже завершён
```

`cmd.Output()` внутри `statusCode` — запускает команду и ждёт. Затем `cmd.Wait()` вызывается на уже завершённом процессе.

Правильный подход:

```go
cmd.Stdout = os.Stdout
cmd.Stdin = os.Stdin
cmd.Stderr = os.Stderr

if err := cmd.Start(); err != nil {
    return fmt.Errorf("start: %w", err)
}

runner.setCmd(cmd)

if err := cmd.Wait(); err != nil {
    var exitErr *exec.ExitError
    if errors.As(err, &exitErr) {
        bus <- exitErr.ExitCode()
    }
    return
}
bus <- 0
```

### 3.5 Удалить мёртвый код

- `src/main.go:91` — закомментированный вызов `unpackCommand(rawCommand)`
- `test/main.go:13–18` — закомментированные строки в массиве

---

## Этап 4. Обработка ошибок и логирование

### 4.1 Заменить `log.Fatalf` на возврат ошибок

`Fatalf` вызывает `os.Exit(1)` без выполнения defer-цепочек. В критических местах нужно возвращать ошибку и завершаться через один централизованный выход.

### 4.2 Добавить структурированное логирование

Заменить стандартный `log` на `log/slog` (добавлен в Go 1.21, без внешних зависимостей):

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
logger.Info("starting process", "command", command)
logger.Error("signal forward failed", "signal", sig, "err", err)
```

Преимущества: machine-readable JSON, уровни, контекст.

### 4.3 Добавить валидацию входных данных

```go
if len(os.Args) < 2 {
    fmt.Fprintln(os.Stderr, "usage: taskmaster <command> [args...]")
    os.Exit(1)
}
```

---

## Этап 5. Тестирование

### 5.1 Unit-тесты для Runner

**Файл:** `internal/runner/runner_test.go`

Минимальный набор тестов:

| Тест | Что проверяет |
|------|---------------|
| `TestRunnerStartStop` | Запуск и остановка по контексту |
| `TestRunnerExitCode` | Корректное получение exit code |
| `TestRunnerRestartOnZero` | Рестарт при exit code 0 |
| `TestRunnerNoRestartOnSignal` | Нет рестарта после shutdown |
| `TestCmdConcurrentAccess` | Нет гонок при параллельном доступе |

### 5.2 Интеграционный тест

Использовать существующее `test/testapp` как тестовый бинарь:

```go
func TestIntegration(t *testing.T) {
    runner := runner.New(logger)
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    err := runner.Run(ctx, []string{"./testapp"})
    assert.NoError(t, err)
}
```

### 5.3 Запуск с race detector

```makefile
test:
    go test -race ./...
```

---

## Этап 6. Makefile и CI

### 6.1 Исправить Makefile

Текущий `test` target не работает:

```makefile
# было (сломано: compile и run — не make-targets, а shell-команды)
test:
    pwd
    compile
    run
    purge
```

```makefile
# стало
.PHONY: build test clean docker-build docker-push

build:
    go build -o dist/taskmaster ./cmd/taskmaster
    go build -o dist/testapp    ./test/testapp

test:
    go test -race ./...

lint:
    golangci-lint run ./...

clean:
    rm -rf dist/

run: build
    ./dist/taskmaster ./dist/testapp
```

### 6.2 Добавить GitHub Actions (опционально)

```yaml
# .github/workflows/ci.yml
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - run: go test -race ./...
      - run: go vet ./...
```

---

## Порядок выполнения

```
Этап 1  →  Этап 2  →  Этап 3  →  Этап 4  →  Этап 5  →  Этап 6
(основа)   (race)     (рефакторинг) (ошибки)  (тесты)   (CI)
```

Каждый этап — отдельный PR. Этапы 2 и 3 можно выполнять параллельно после завершения этапа 1.

---

## Сводная таблица изменений

| # | Файл | Изменение | Приоритет |
|---|------|-----------|-----------|
| 1 | `go.mod` | Переименовать модуль, обновить Go до 1.23 | Критический |
| 2 | `go.mod` | Удалить неиспользуемый `x/crypto` | Высокий |
| 3 | `.docker/Dockerfile` | Multi-stage build, Go 1.23, FROM scratch | Высокий |
| 4 | `src/main.go` | Убрать глобалы → struct Runner | Критический |
| 5 | `src/main.go` | Добавить mutex для `cmd` | Критический |
| 6 | `src/main.go` | Буферизованный `bus` канал | Высокий |
| 7 | `src/main.go` | context.Context вместо isShutdown | Высокий |
| 8 | `src/main.go` | Исправить `os.Args` слайс | Низкий |
| 9 | `src/main.go` | Убрать `os.Kill` из listen | Средний |
| 10 | `src/main.go` | Исправить `statusCode()` + `execute()` | Критический |
| 11 | `src/main.go` | Заменить Fatalf на возврат ошибок | Высокий |
| 12 | `src/main.go` | Удалить мёртвый код | Низкий |
| 13 | `*` | Структурированное логирование (slog) | Средний |
| 14 | `internal/runner/runner_test.go` | Unit-тесты + race detector | Высокий |
| 15 | `Makefile` | Исправить test target | Средний |
| 16 | `cmd/taskmaster/main.go` | Реструктурировать по стандартному layout | Средний |
