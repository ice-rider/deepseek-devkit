# deepseek-devkit

*разумеется навайбкожен ридмик. да и не только он. а вы че думали, в сказку попали?*

---

Неофициальный Go-клиент для веб-API [chat.deepseek.com](https://chat.deepseek.com).

Модуль умеет:

* авторизовываться по токену;
* создавать, перечислять, переименовывать и удалять чат-сессии;
* получать историю сообщений;
* отправлять сообщения в **синхронном** и **стриминговом** режимах;
* останавливать генерацию;
* автоматически решать Proof-of-Work челлендж (`x-ds-pow-response`);
* вести подробный **сетевой лог** и делать retry — по желанию.

> ⚠️ Это не официальный SDK. Проект использует приватные эндпоинты веб-клиента DeepSeek,
> и они могут измениться в любой момент. Используйте на свой риск и уважайте ToS DeepSeek.

---

## Содержание

1. [Установка](#установка)
2. [Быстрый старт](#быстрый-старт)
3. [Как получить токен](#как-получить-токен)
4. [API](#api)
   - [Создание клиента](#создание-клиента)
   - [Авторизация](#авторизация)
   - [Чаты](#чаты)
   - [Отправка сообщений](#отправка-сообщений)
   - [Остановка генерации](#остановка-генерации)
5. [Сетевое логирование и retry](#сетевое-логирование-и-retry)
6. [Обработка ошибок](#обработка-ошибок)
7. [Ограничения и заметки](#ограничения-и-заметки)
8. [Лицензия](#лицензия)

---

## Установка

```bash
go get github.com/ice-rider/deepseek-devkit@v0.1.2
````

Требуется Go ≥ 1.22 (модуль собран под `go 1.27.2`, но использует только стандартную библиотеку и `ds2api/pow`).

---

## Быстрый старт

Минимальный пример: авторизация → создание чата → отправка сообщения.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	devkit "github.com/ice-rider/deepseek-devkit"
)

func main() {
	token := os.Getenv("DEEPSEEK_TOKEN")
	if token == "" {
		log.Fatal("DEEPSEEK_TOKEN is not set")
	}

	client := devkit.NewClient(token)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. Проверяем токен и получаем данные пользователя.
	user, err := client.Authorize(ctx)
	if err != nil {
		log.Fatalf("authorize: %v", err)
	}
	log.Printf("authorized as %s", user.Email)

	// 2. Создаём новую чат-сессию.
	session, err := client.NewChat(ctx)
	if err != nil {
		log.Fatalf("new chat: %v", err)
	}
	log.Printf("session: %s", session.ID)

	// 3. Отправляем сообщение в синхронном режиме.
	res, _, err := client.NewMessage(ctx, session.ID, "Что такое Go? Ответь кратко.", false)
	if err != nil {
		log.Fatalf("new message: %v", err)
	}
	fmt.Println("Ответ:", res.AssistantMessage.Content)
}
```

---

## Как получить токен

1. Откройте [chat.deepseek.com](https://chat.deepseek.com) и войдите в аккаунт.
2. Откройте DevTools → **Application** → **Local Storage** → `chat.deepseek.com`.
3. Найдите ключ `userToken` (или `__userToken__` в зависимости от версии) и скопируйте его значение — это и есть `DEEPSEEK_TOKEN`.

Токен передаётся в заголовке `Authorization: Bearer <token>`. В логах он **автоматически маскируется**.

---

## API

### Создание клиента

```go
// Самый простой способ — с настройками по умолчанию
// (сетевой лог выключен, baseURL = https://chat.deepseek.com).
client := devkit.NewClient(token)

// Расширенный способ — с собственной конфигурацией.
cfg := devkit.DefaultConfig()
cfg.BaseURL = "https://chat.deepseek.com" // можно переопределить
cfg.Network = devkit.DebugNetworkConfig() // включить подробный лог
client := devkit.NewClientWithConfig(token, cfg)
```

Интерфейс, которому удовлетворяет `*Client`:

```go
type DeepSeek interface {
	Authorize(ctx context.Context) (*User, error)
	GetCurrentUser(ctx context.Context) (*User, error)
	NewChat(ctx context.Context) (*Session, error)
	GetListOfChats(ctx context.Context) ([]*Session, error)
	GetChatById(ctx context.Context, chatID string) ([]*Message, error)
	RenameChat(ctx context.Context, chatID, newTitle string) error
	DeleteChat(ctx context.Context, chatID string) error
	NewMessage(ctx context.Context, chatID, prompt string, stream bool) (*NewMessageResult, <-chan string, error)
	StopGeneration(ctx context.Context, chatID string) error
}
```

Удобно использовать в юнит-тестах — достаточно замокать интерфейс.

---

### Авторизация

```go
user, err := client.Authorize(ctx)
if err != nil {
	log.Fatal(err)
}
fmt.Println(user.ID, user.Email)
```

`Authorize` валидирует токен запросом к `/api/v0/users/current`. `GetCurrentUser` — его псевдоним.

Если токен пустой или невалидный, вернётся ошибка. Успешный вызов возвращает `*devkit.User`.

---

### Чаты

```go
// Создать
sess, err := client.NewChat(ctx)

// Список (первая страница, 50 штук)
chats, err := client.GetListOfChats(ctx)

// История конкретной сессии
msgs, err := client.GetChatById(ctx, sess.ID)
for _, m := range msgs {
	fmt.Printf("[%s] %s: %s\n", m.ID, m.Role, m.Content)
}

// Переименовать
err = client.RenameChat(ctx, sess.ID, "Мой новый заголовок")

// Удалить
err = client.DeleteChat(ctx, sess.ID)
```

Структура `Session`:

```go
type Session struct {
	ID        string
	Title     string
	ModelType string
	UpdatedAt float64
	Pinned    bool
}
```

Структура `Message`:

```go
type Message struct {
	ID        string
	Role      string // "user" | "assistant"
	Content   string
	CreatedAt int64
}
```

---

### Отправка сообщений

#### Синхронный режим

```go
res, _, err := client.NewMessage(ctx, chatID, "Расскажи про горутины", false)
if err != nil {
	log.Fatal(err)
}
fmt.Println(res.AssistantMessage.Content)
```

Возвращает `*NewMessageResult`, в котором заполнены `UserMessage` и `AssistantMessage`. Второй элемент (`<-chan string`) — `nil`.

#### Стриминговый режим

```go
res, stream, err := client.NewMessage(ctx, chatID, "Напиши хайку про Go", true)
if err != nil {
	log.Fatal(err)
}
_ = res // в стриминговом режиме res == nil

var full strings.Builder
for chunk := range stream {
	full.WriteString(chunk)
	fmt.Print(chunk) // печатаем по мере поступления
}
fmt.Println("\n---\n", full.String())
```

Особенности стриминга:

* Канал закрывается сам, когда ответ закончился или случилась ошибка.
* `context.Context` управляет жизнью стрима — отмена контекста прерывает чтение.
* Ошибки стрима **не пробрасываются через канал** (ограничение текущей реализации). Если нужна надёжная обработка ошибок — используйте `stream=false`.

#### Многоходовой диалог

Клиент сам хранит `lastMsgID` для каждой сессии, поэтому второй вызов `NewMessage` автоматически продолжит тот же диалог:

```go
_, _, _ = client.NewMessage(ctx, chatID, "Меня зовут Глеб.", false)
res, _, _ := client.NewMessage(ctx, chatID, "Как меня зовут?", false)
fmt.Println(res.AssistantMessage.Content) // "Вас зовут Глеб."
```

Если `lastMsgID` потерян (например, после рестарта приложения), клиент автоматически восстанавливает его из истории через `GetChatById`.

---

### Остановка генерации

```go
// Обычно вызывается из отдельной горутины, пока идёт стрим:
go func() {
	time.Sleep(2 * time.Second)
	if err := client.StopGeneration(ctx, chatID); err != nil {
		log.Printf("stop: %v", err)
	}
}()

for chunk := range stream {
	fmt.Print(chunk)
}
```

`StopGeneration` использует `lastMsgID`, который обновляется на каждом событии SSE. Если в сессии ещё не было ответов ассистента — вернётся ошибка `no active message in chat`.

---

## Сетевое логирование и retry

Модуль умеет подробно логировать HTTP-обмен и ретраить временные ошибки. **По умолчанию слой выключен** и не создаёт накладных расходов.

### Что логируется

* метод, URL (секретные параметры маскируются);
* заголовки — `Authorization`, `x-ds-pow-response`, `Cookie` **всегда** маскируются;
* тело запроса и ответа (опционально, с ограничением размера);
* статус-код и длительность;
* отдельные SSE-чанки (`LogStreaming`).

### Что ретраится

* HTTP `429`, `502`, `503`, `504`;
* сетевые ошибки (`connection reset`, `timeout` и т.п.);
* **не** ретраятся ошибки контекста (`context.Canceled`, `context.DeadlineExceeded`);
* потоковые ответы **не** ретраятся после начала чтения тела.

Backoff экспоненциальный: `RetryBackoff * 2^(attempt-1)`, но не больше `MaxRetryBackoff`.

### Конфигурация

```go
type NetworkConfig struct {
	Enabled         bool           // мастер-выключатель
	LogRequests     bool
	LogResponses    bool
	LogBodies       bool
	LogStreaming    bool
	MaxBodyBytes    int            // по умолчанию 4096
	Logger          *log.Logger    // nil → логгер по умолчанию
	Timeout         time.Duration  // 0 = без таймаута клиента (безопасно для SSE)
	MaxRetries      int            // 0 = без ретраев
	RetryBackoff    time.Duration  // по умолчанию 500ms
	MaxRetryBackoff time.Duration  // по умолчанию 10s
}
```

### Способы включения

**1. Через конфиг при создании клиента** — самый предсказуемый вариант:

```go
cfg := devkit.DefaultConfig()
cfg.Network = devkit.DebugNetworkConfig() // всё включено, включая SSE-чанки
client := devkit.NewClientWithConfig(token, cfg)
```

**2. Точечно** — включить только нужное:

```go
cfg := devkit.DefaultConfig()
cfg.Network.Enabled      = true
cfg.Network.LogRequests  = true
cfg.Network.LogResponses = true
cfg.Network.LogBodies    = true
cfg.Network.LogStreaming = false   // SSE-чанки выключены
cfg.Network.MaxRetries   = 3
cfg.Network.Logger       = log.New(os.Stderr, "[net] ", log.LstdFlags)
client := devkit.NewClientWithConfig(token, cfg)
```

**3. В рантайме** — включить/выключить одной строкой:

```go
client := devkit.NewClient(token)

// ... какие-то операции ...

client.SetNetworkLogging(true)              // включить
client.SetNetworkLogging(false)             // выключить

client.SetNetworkConfig(myNetworkConfig)    // заменить целиком
nc := client.NetworkConfig()                // прочитать текущую
```

**4. Через переменную окружения** — удобно для CLI:

```go
cfg := devkit.DefaultConfig()
if os.Getenv("DEEPSEEK_DEBUG_NETWORK") != "" {
	cfg.Network = devkit.DebugNetworkConfig()
}
client := devkit.NewClientWithConfig(token, cfg)
```

### Пример вывода

```
[devkit/http] 2026/11/08 12:00:01.123456 → POST https://chat.deepseek.com/api/v0/chat/completion (attempt 1)
  headers: map[Authorization:[<redacted>] Content-Type:[application/json] User-Agent:[deepseek-devkit/0.1] X-Ds-Pow-Response:[<redacted>]]
  body: {"chat_session_id":"abc","parent_message_id":42,"model_type":"default","prompt":"..."}
[devkit/http] 2026/11/08 12:00:02.987654 ← 200 POST https://chat.deepseek.com/api/v0/chat/completion (attempt 1, 1.864s)
[devkit/http] 2026/11/08 12:00:02.987900   POST https://chat.deepseek.com/api/v0/chat/completion body (truncated): data: {"v":"4"}
[devkit/http] 2026/11/08 12:00:02.988100   POST https://chat.deepseek.com/api/v0/chat/completion body complete: 4096 bytes in 1.864s
```

### Важные нюансы

* `Timeout` по умолчанию `0` — таймаут задаётся через `context.WithTimeout` у вызывающего. Это сделано специально, чтобы длинные SSE-стримы не обрывались на 60 секундах.
* `Logger` захватывается при первом использовании `http.Client`. Чтобы сменить логгер, создайте новый `Client`.
* `LogBodies = true` буферизует тело запроса целиком — для этого API тела маленькие JSON, так что это безопасно. Если когда-нибудь появятся большие аплоады — выключите `LogBodies`.

---

## Обработка ошибок

Все ошибки оборачиваются через `fmt.Errorf("...: %w", err)` — работают `errors.Is` и `errors.As`.

Типичные ошибки:

| Сообщение | Причина |
|---|---|
| `devkit: token is empty` | Пустой токен |
| `authorize: http 401: ...` | Невалидный или истёкший токен |
| `new chat: http 5xx: ...` | Временная ошибка DeepSeek |
| `completion: ...` | Ошибка при запросе завершения |
| `devkit: no active message in chat "..."` | `StopGeneration` до первого ответа ассистента |
| `context deadline exceeded` | Истёк `context.WithTimeout` |

Пример обработки:

```go
_, _, err := client.NewMessage(ctx, chatID, prompt, false)
if err != nil {
	if errors.Is(err, context.DeadlineExceeded) {
		log.Println("DeepSeek не ответил вовремя")
		return
	}
	log.Printf("unexpected: %v", err)
}
```

---

## Ограничения и заметки

* **Ошибки стрима не пробрасываются через канал.** Если нужна гарантированная обработка ошибок — используйте `stream=false` или оберните чтение в таймаут по контексту.
* **`lastMsgID` хранится в памяти** и теряется при рестарте процесса. Клиент автоматически восстанавливает его из истории при следующем `NewMessage`, но это стоит один лишний GET.
* **`StopGeneration` не блокирует** вызывающего — если нужно прервать текущий стрим, вызывайте его из отдельной горутины.
* **PoW-челлендж** решается автоматически перед каждым `NewMessage`. Отдельного API для этого нет — это внутренняя деталь.
* **`GetListOfChats` возвращает только первую страницу** (50 чатов). Пагинация пока не реализована.
* **Ретраи применяются только к начальному ответу.** Если стрим упал в середине — байты уже отданы вызывающему, повторять небезопасно.

---

## Лицензия

MIT. См. файл [LICENSE](LICENSE).