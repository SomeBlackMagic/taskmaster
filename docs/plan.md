# Taskmaster — План апгрейда

## Общий контекст

Taskmaster — минималистичный менеджер процессов на Go: запускает команду в бесконечном цикле и пробрасывает Linux-сигналы в дочерний процесс. Текущая реализация — MVP с рядом структурных, конкурентных и инфраструктурных проблем.

Все изменения должны соответствовать правилам **Go Clean Architecture & Concurrency Rules** (далее — номер правила в скобках).

---

## Текущие нарушения правил

| Правило | Нарушение | Место в коде |
|---------|-----------|--------------|
| #7 — глобальные переменные запрещены | `cmd`, `bus`, `signals`, логгеры — глобальные | `src/main.go:11–23` |
| #11 — каждая ошибка должна обрабатываться | `cmd.Output()` возвращает ошибку, которая игнорируется | `src/main.go:117` |
| #12 — ошибки оборачивать через `%w` | Нигде не используется `fmt.Errorf("%w", err)` | `src/main.go` |
| #14 — context первым аргументом | Ни одна функция не принимает `context.Context` | `src/main.go` |
| #17 — логгер инжектится как зависимость | Логгеры — глобальные переменные | `src/main.go:21–23` |
| #21 — горутина без контроля жизненного цикла | `go execute(command)` — fire-and-forget | `src/main.go:42, 52` |
| #22 — fire-and-forget горутины запрещены | `go execute(command)` не ожидается нигде | `src/main.go:42` |
| #23 — горутины принимают context | `execute()` не принимает `context.Context` | `src/main.go:89` |
| #24 — длинные циклы проверяют `ctx.Done()` | Бесконечный цикл в `execute()` без проверки | `src/main.go:92` |
| #27 — errgroup для оркестрации | Нет `errgroup`, горутины не отслеживаются | `src/main.go` |
| #31 — закрывает канал только отправитель | `main()` закрывает `bus`, хотя пишет `execute()` | `src/main.go:54, 59` |
| #35 — бесконечный select без условия выхода | `for { select {} }` — нет `ctx.Done()` | `src/main.go:44–86` |
| #38 — все каналы должны быть буферизованы | `bus = make(chan int)` — небуферизован | `src/main.go:27` |
| #41 — graceful shutdown | Нет корректного завершения с ожиданием горутин | `src/main.go` |
| #43 — завершение горутин через WaitGroup/errgroup | Нет ожидания горутины `execute` при shutdown | `src/main.go` |
| #47 — бесконечные retry без backoff запрещены | Рестарт без задержки в цикле `for {}` | `src/main.go:92` |
| #49 — panic в горутинах должен быть recovered | Нет `recover()` в `execute()` | `src/main.go:89` |
| #9 — конфигурация загружается при старте | Нет конфигурации — аргументы не валидируются | `src/main.go:38` |

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

Убрать неиспользуемую зависимость `golang.org/x/crypto`. Добавить `golang.org/x/sync` для `errgroup`.

### 1.3 Обновить Dockerfile (правила #9, #41)

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

### 1.4 Реструктурировать проект (правила #6, #7)

```
taskmaster/
├── cmd/
│   └── taskmaster/
│       └── main.go              # точка входа: парсинг конфига, DI, запуск
├── internal/
│   └── runner/
│       ├── config.go            # Config struct с defaults (#9, #57)
│       ├── runner.go            # Runner struct и логика (#6, #7)
│       ├── runner_test.go       # unit-тесты (#51, #52)
│       └── interfaces.go        # ProcessExecutor interface (#3, #18)
├── test/
│   └── testapp/
│       └── main.go              # тестовое приложение
├── docs/
│   └── plan.md
├── .docker/
│   └── Dockerfile
├── go.mod
├── go.sum
└── Makefile
```

---

## Этап 2. Конфигурация и зависимости (правила #6, #9, #57)

### 2.1 Добавить Config с дефолтами

**Файл:** `internal/runner/config.go`

```go
type Config struct {
    Command      []string
    RestartDelay time.Duration // default: 1s (#57)
    MaxRestarts  int           // default: 0 (unlimited, #57)
}

func DefaultConfig() Config {
    return Config{
        RestartDelay: time.Second,
        MaxRestarts:  0,
    }
}
```

Валидация при старте (#9):

```go
func (c Config) Validate() error {
    if len(c.Command) == 0 {
        return errors.New("command must not be empty")
    }
    return nil
}
```

### 2.2 Инжекция зависимостей через конструктор (правило #6)

Все зависимости — через `New()`, никаких глобалов (#7):

```go
type Runner struct {
    cfg    Config
    logger *slog.Logger   // инжектируется (#17)
}

func New(cfg Config, logger *slog.Logger) *Runner {
    return &Runner{cfg: cfg, logger: logger}
}
```

`cmd/taskmaster/main.go` — точка входа:

```go
func main() {
    cfg := runner.DefaultConfig()
    cfg.Command = os.Args[1:]
    if err := cfg.Validate(); err != nil {
        fmt.Fprintln(os.Stderr, "usage: taskmaster <command> [args...]")
        os.Exit(1)
    }

    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
    r := runner.New(cfg, logger)

    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGTERM, syscall.SIGINT)
    defer stop()

    if err := r.Run(ctx); err != nil {
        logger.Error("runner failed", "err", err)
        os.Exit(1)
    }
}
```

---

## Этап 3. Конкурентность: устранение race conditions и нарушений

### 3.1 Защита `cmd` мьютексом (правила #34, #40)

Текущая проблема:

```go
// НАРУШЕНИЕ #34: cmd пишется в горутине execute(), читается в main() без sync
var cmd *exec.Cmd
```

Решение — инкапсулировать в struct с `sync.Mutex`:

```go
type Runner struct {
    mu  sync.Mutex
    cmd *exec.Cmd
    // ...
}

func (r *Runner) setCmd(c *exec.Cmd) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.cmd = c
}

func (r *Runner) sendSignal(sig os.Signal) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    if r.cmd == nil || r.cmd.Process == nil {
        return nil
    }
    return r.cmd.Process.Signal(sig)
}
```

### 3.2 Заменить bus-канал на errgroup (правила #27, #31, #43)

**Текущие нарушения:**
- `main()` закрывает `bus`, который пишет `execute()` — нарушение #31 (закрывает только отправитель)
- Горутина не ожидается при завершении — нарушение #43

**Решение** — использовать `errgroup`:

```go
import "golang.org/x/sync/errgroup"

func (r *Runner) Run(ctx context.Context) error {   // ctx первым (#14)
    g, ctx := errgroup.WithContext(ctx)

    g.Go(func() error {
        return r.execute(ctx)   // горутина принимает ctx (#23)
    })

    return g.Wait()   // ожидание завершения (#43)
}
```

Канал `bus` полностью убирается. Возврат ошибки из `execute()` — механизм передачи exit code.

### 3.3 Убрать бесконечный select без условия выхода (правило #35)

**Текущее нарушение:**
```go
// НАРУШЕНИЕ #35: нет ctx.Done()
for {
    select {
    case code := <-bus:  ...
    case sig := <-signals: ...
    }
}
```

**Решение** — обработка сигналов переезжает в `cmd/taskmaster/main.go` через `signal.NotifyContext`. Отдельного select-loop для сигналов больше нет. `runner.Run(ctx)` завершается когда `ctx` отменён.

### 3.4 Добавить backoff при рестарте (правило #47)

**Текущее нарушение:**
```go
// НАРУШЕНИЕ #47: бесконечный restart без задержки
for {
    cmd = exec.Command(...)
    // сразу снова
}
```

**Решение:**

```go
func (r *Runner) execute(ctx context.Context) error {
    for {
        select {
        case <-ctx.Done():
            return nil
        default:
        }

        if err := r.runOnce(ctx); err != nil {
            // ... логируем
        }

        select {
        case <-ctx.Done():
            return nil
        case <-time.After(r.cfg.RestartDelay):   // backoff (#47)
        }
    }
}
```

### 3.5 Добавить recover в горутину (правило #49)

```go
func (r *Runner) execute(ctx context.Context) (retErr error) {
    defer func() {
        if p := recover(); p != nil {
            r.logger.Error("panic in execute goroutine", "panic", p)
            retErr = fmt.Errorf("panic: %v", p)
        }
    }()
    // ...
}
```

---

## Этап 4. Исправление логики `execute()` и `statusCode()`

### 4.1 Исправить `statusCode()` и `execute()` (правила #11, #12, #14)

Текущая логика сломана:

```go
// НАРУШЕНИЕ #11: cmd.Output() запускает + ждёт процесс
// затем cmd.Wait() вызывается на уже завершённом — всегда ошибка
if code := statusCode(cmd); code != 0 { ... }
if err := cmd.Wait(); err != nil { ... }
```

Правильный подход — `cmd.Start()` + `cmd.Wait()`:

```go
func (r *Runner) runOnce(ctx context.Context) error {   // ctx первым (#14)
    var cmd *exec.Cmd
    if len(r.cfg.Command) > 1 {
        cmd = exec.CommandContext(ctx, r.cfg.Command[0], r.cfg.Command[1:]...)
    } else {
        cmd = exec.CommandContext(ctx, r.cfg.Command[0])
    }
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr

    if err := cmd.Start(); err != nil {
        return fmt.Errorf("start process: %w", err)   // оборачиваем #12
    }
    r.setCmd(cmd)

    if err := cmd.Wait(); err != nil {
        var exitErr *exec.ExitError
        if errors.As(err, &exitErr) {   // не сравниваем по строке #13
            return fmt.Errorf("process exited: %w", exitErr)
        }
        return fmt.Errorf("wait: %w", err)
    }
    return nil   // exit code 0
}
```

### 4.2 Использовать `exec.CommandContext` (правило #23, #24)

`exec.CommandContext(ctx, ...)` автоматически посылает SIGKILL процессу при отмене контекста. Это убирает ручную обработку `syscall.SIGKILL`.

---

## Этап 5. Логирование и обработка ошибок

### 5.1 Структурированное логирование через slog (правила #17, #58)

Заменить стандартный `log` на `log/slog` (стандартная библиотека с Go 1.21):

```go
// инжектируется через конструктор (#17), не глобал (#7)
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

logger.InfoContext(ctx, "starting process", "command", r.cfg.Command)
logger.ErrorContext(ctx, "process exited with error", "err", err)
```

Не логируем sensitive данные (#58): команда логируется, аргументы — только в debug-режиме.

### 5.2 Заменить `Fatalf` на возврат ошибок (правила #11, #12)

`Fatalf` вызывает `os.Exit(1)` без выполнения defer-цепочек. Все ошибки возвращаются через `error` и обрабатываются в одном месте — `cmd/taskmaster/main.go`.

### 5.3 Исправить обработку сигналов (правила #9, #41)

Убрать `os.Kill` и `syscall.SIGKILL` из подписки — их нельзя перехватить:

```go
// cmd/taskmaster/main.go
ctx, stop := signal.NotifyContext(context.Background(),
    syscall.SIGTERM,
    syscall.SIGINT,
)
defer stop()
```

`signal.NotifyContext` — идиоматичный способ graceful shutdown (#41).

---

## Этап 6. Интерфейсы (правила #2, #3, #18, #19)

Taskmaster — небольшое приложение, поэтому следуем правилу #10 (не вводить абстракции без необходимости) и #19 (не создавать интерфейс для каждого struct).

Единственный интерфейс, который оправдан — для тестирования запуска процессов (#53, #54):

```go
// internal/runner/interfaces.go
// Объявлен в потребляющем слое (#3), минимален (#18)
type ProcessStarter interface {
    Start(ctx context.Context, command []string) error
}
```

Используется в тестах для подмены реального запуска процессов (#53, #54).

---

## Этап 7. Тестирование (правила #40, #51–#55)

### 7.1 Unit-тесты с табличным подходом (правила #51, #52)

**Файл:** `internal/runner/runner_test.go`

```go
func TestRunnerExecute(t *testing.T) {
    tests := []struct {
        name        string
        command     []string
        wantErr     bool
        wantRestart bool
    }{
        {"exit 0 triggers restart", []string{"true"}, false, true},
        {"exit 1 stops runner",    []string{"false"}, true, false},
        {"invalid command",        []string{"no-such-bin"}, true, false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // ...
        })
    }
}
```

### 7.2 Тесты не зависят от инфраструктуры (правило #54)

Использовать `ProcessStarter` interface для мокирования (#53). Не запускать реальные внешние процессы в unit-тестах.

### 7.3 Минимальный набор тестов

| Тест | Правило |
|------|---------|
| `TestRunnerExitZeroRestart` | #21, #22 |
| `TestRunnerExitNonZeroStop` | #11, #12 |
| `TestRunnerContextCancellation` | #23, #24, #42 |
| `TestRunnerGracefulShutdown` | #41, #43 |
| `TestRunnerConcurrentSignal` | #34, #40 |
| `TestRunnerPanicRecovery` | #49 |
| `TestRunnerBackoff` | #47 |

### 7.4 Race detector обязателен (правила #40, #55)

```makefile
test:
    go test -race ./...
```

---

## Этап 8. Makefile и CI (правила #40, #51)

### 8.1 Исправить Makefile

Текущий `test` target не работает (`compile` и `run` — не make-targets):

```makefile
.PHONY: build test lint clean run

build:
    go build -o dist/taskmaster ./cmd/taskmaster
    go build -o dist/testapp    ./test/testapp

test:
    go test -race ./...

lint:
    go vet ./...

clean:
    rm -rf dist/

run: build
    ./dist/taskmaster ./dist/testapp
```

### 8.2 GitHub Actions (опционально)

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

## Что НЕ делаем (правила #10, #19, #60)

- **Не создаём интерфейс** для каждого struct (нарушение #19)
- **Не добавляем слои Domain/UseCase** — приложение слишком маленькое, это нарушит #10 и #60
- **Не добавляем worker pool** — один воркер по дизайну, ограничение уже есть
- **Не добавляем флаги командной строки** сверх необходимого — `flag` пакет только если потребуется

---

## Порядок выполнения

```
Этап 1  →  Этап 2  →  Этап 3  ──┐
(основа)   (конфиг)   (concurrency)│
                                   ├──>  Этап 5  →  Этап 7  →  Этап 8
Этап 4  ───────────────────────────┘     (logging)  (тесты)   (CI)
(логика)
                                   Этап 6 — по мере надобности
```

Каждый этап — отдельный PR. Этапы 3 и 4 можно выполнять параллельно после Этапа 2.

---

## Сводная таблица изменений

| # | Файл | Изменение | Правила | Приоритет |
|---|------|-----------|---------|-----------|
| 1 | `go.mod` | Переименовать модуль, обновить Go 1.23 | — | Критический |
| 2 | `go.mod` | Убрать `x/crypto`, добавить `x/sync` | #27 | Высокий |
| 3 | `.docker/Dockerfile` | Multi-stage build, Go 1.23, FROM scratch | — | Высокий |
| 4 | `cmd/taskmaster/main.go` | Точка входа: DI, конфиг, signal.NotifyContext | #6, #9, #41 | Критический |
| 5 | `internal/runner/config.go` | Config struct с валидацией и дефолтами | #9, #57 | Критический |
| 6 | `internal/runner/runner.go` | Runner struct: убрать все глобалы | #7, #8 | Критический |
| 7 | `internal/runner/runner.go` | Mutex для `cmd` | #34, #40 | Критический |
| 8 | `internal/runner/runner.go` | errgroup вместо bus-канала | #27, #31, #43 | Критический |
| 9 | `internal/runner/runner.go` | ctx в каждой функции первым | #14, #15, #23 | Критический |
| 10 | `internal/runner/runner.go` | ctx.Done() в execute-цикле | #24, #35, #42 | Критический |
| 11 | `internal/runner/runner.go` | cmd.Start()+cmd.Wait() вместо Output() | #11, #12 | Критический |
| 12 | `internal/runner/runner.go` | Backoff при рестарте | #47 | Высокий |
| 13 | `internal/runner/runner.go` | recover() в горутине | #49 | Высокий |
| 14 | `internal/runner/runner.go` | exec.CommandContext | #23 | Высокий |
| 15 | `internal/runner/runner.go` | Убрать os.Kill/SIGKILL из подписки | — | Средний |
| 16 | `internal/runner/runner.go` | Инжекция логгера, slog | #17, #58 | Высокий |
| 17 | `internal/runner/interfaces.go` | ProcessStarter interface | #3, #18, #53 | Средний |
| 18 | `internal/runner/runner_test.go` | Табличные unit-тесты + -race | #40, #51, #52 | Высокий |
| 19 | `Makefile` | Исправить test target | — | Средний |
| 20 | `src/main.go` | Удалить мёртвый код | — | Низкий |
