package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientSendMessageSuccess(t *testing.T) {
	var received sendMessageRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendMessage") {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(apiResponse{OK: true})
	}))
	defer server.Close()

	client := &Client{token: "TEST", baseURL: server.URL, httpClient: &http.Client{}}
	if err := client.SendMessage(context.Background(), 101, "привет"); err != nil {
		t.Fatalf("send message: %v", err)
	}

	if received.ChatID != 101 {
		t.Fatalf("chat_id = %d, want 101", received.ChatID)
	}
	if received.Text != "привет" {
		t.Fatalf("text = %q, want %q", received.Text, "привет")
	}
	if received.ParseMode != "HTML" {
		t.Fatalf("parse_mode = %q, want HTML", received.ParseMode)
	}
}

func TestClientRateLimitIsTemporary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(apiResponse{
			OK:          false,
			ErrorCode:   http.StatusTooManyRequests,
			Description: "Too Many Requests",
			Parameters: &struct {
				RetryAfter int `json:"retry_after"`
			}{RetryAfter: 7},
		})
	}))
	defer server.Close()

	client := &Client{token: "TEST", baseURL: server.URL, httpClient: &http.Client{}}
	err := client.SendMessage(context.Background(), 101, "привет")
	if err == nil {
		t.Fatal("expected an error")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.Code != http.StatusTooManyRequests {
		t.Fatalf("code = %d, want 429", apiErr.Code)
	}
	if apiErr.RetryAfter != 7*time.Second {
		t.Fatalf("retry_after = %s, want 7s", apiErr.RetryAfter)
	}
	if !apiErr.Temporary() || !isTemporary(err) {
		t.Fatal("429 must be treated as temporary")
	}
}

func TestClientBadRequestIsPermanent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(apiResponse{
			OK:          false,
			ErrorCode:   http.StatusBadRequest,
			Description: "Bad Request: can't parse entities",
		})
	}))
	defer server.Close()

	client := &Client{token: "TEST", baseURL: server.URL, httpClient: &http.Client{}}
	err := client.SendMessage(context.Background(), 101, "привет")
	if err == nil {
		t.Fatal("expected an error")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.Temporary() || isTemporary(err) {
		t.Fatal("400 must not be treated as temporary")
	}
}

func TestClientRedactsTokenFromNetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	serverURL := server.URL
	server.Close()

	const token = "SECRET_TOKEN"
	client := &Client{token: token, baseURL: serverURL, httpClient: &http.Client{}}

	err := client.SendMessage(context.Background(), 101, "привет")
	if err == nil {
		t.Fatal("expected a network error")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("error leaks the bot token: %v", err)
	}
	if !isTemporary(err) {
		t.Fatalf("network error must be temporary: %v", err)
	}
}

func TestClientGetUpdatesSendsOffset(t *testing.T) {
	var receivedOffset string
	var receivedAllowedUpdates string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/getUpdates") {
			http.NotFound(w, r)
			return
		}
		receivedOffset = r.URL.Query().Get("offset")
		receivedAllowedUpdates = r.URL.Query().Get("allowed_updates")
		_ = json.NewEncoder(w).Encode(apiResponse{OK: true, Result: json.RawMessage(`[]`)})
	}))
	defer server.Close()

	client := &Client{token: "TEST", baseURL: server.URL, httpClient: &http.Client{}}
	updates, err := client.GetUpdates(context.Background(), 42)
	if err != nil {
		t.Fatalf("get updates: %v", err)
	}
	if len(updates) != 0 {
		t.Fatalf("updates = %d, want 0", len(updates))
	}
	if receivedOffset != "42" {
		t.Fatalf("offset = %q, want 42", receivedOffset)
	}
	if receivedAllowedUpdates != `["message"]` {
		t.Fatalf("allowed_updates = %q", receivedAllowedUpdates)
	}
}

func TestBotStartAllowlistAndOffset(t *testing.T) {
	fake, server := newFakeServer(t)
	bot := newBot(
		&Client{token: "TEST", baseURL: server.URL, httpClient: &http.Client{}},
		[]int64{101},
		log.New(io.Discard, "", 0),
	)

	bot.handleUpdates(context.Background(), []telegramUpdate{
		{
			UpdateID: 5,
			Message: &telegramMessage{
				Text: "/start",
				From: &telegramUser{ID: 101},
				Chat: &telegramChat{ID: 101, Type: "private"},
			},
		},
		{
			UpdateID: 6,
			Message: &telegramMessage{
				Text: "/start",
				From: &telegramUser{ID: 999},
				Chat: &telegramChat{ID: 999, Type: "private"},
			},
		},
		{
			UpdateID: 7,
			Message: &telegramMessage{
				Text: "/start",
				From: &telegramUser{ID: 101},
				Chat: &telegramChat{ID: -100500, Type: "group"},
			},
		},
	})

	if bot.offset != 8 {
		t.Fatalf("offset = %d, want 8", bot.offset)
	}

	allowed := fake.messagesFor(101)
	if len(allowed) != 1 {
		t.Fatalf("allowed user messages = %d, want 1", len(allowed))
	}
	if !strings.Contains(allowed[0].Text, "подключены") {
		t.Fatalf("unexpected allowed reply: %q", allowed[0].Text)
	}

	denied := fake.messagesFor(999)
	if len(denied) != 1 {
		t.Fatalf("denied user messages = %d, want 1", len(denied))
	}
	if !strings.Contains(denied[0].Text, "Доступ запрещён") {
		t.Fatalf("unexpected denied reply: %q", denied[0].Text)
	}
}

func TestLoadConfigValidation(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	t.Setenv("TELEGRAM_ALLOWED_USER_IDS", "")
	t.Setenv("PUBLIC_BASE_URL", "")

	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected an error for an empty configuration")
	}

	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_ALLOWED_USER_IDS", "101, 202,101")
	t.Setenv("PUBLIC_BASE_URL", "https://example.test/")

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(config.AllowedUserIDs) != 2 {
		t.Fatalf("recipients = %v, want two unique IDs", config.AllowedUserIDs)
	}
	if config.PublicBaseURL != "https://example.test" {
		t.Fatalf("public base URL = %q", config.PublicBaseURL)
	}

	t.Setenv("TELEGRAM_ALLOWED_USER_IDS", "not-a-number")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected an error for a non-numeric user ID")
	}
}
