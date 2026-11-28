package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	telegramAPIBaseURL = "https://api.telegram.org"
	sendTimeout        = 10 * time.Second
	getMeTimeout       = 10 * time.Second
	updatesTimeout     = 30 * time.Second
	maxResponseBytes   = 1 << 20
)

// Client is a minimal Telegram Bot API client built on the standard library.
// The bot token is part of the request URL, so every error that may contain
// the URL is passed through redact before it reaches logs.
type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		token:      token,
		baseURL:    telegramAPIBaseURL,
		httpClient: &http.Client{},
	}
}

type sendMessageRequest struct {
	ChatID    int64  `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
	Parameters  *struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// APIError is returned when Telegram responds with a non-success envelope.
type APIError struct {
	Code        int
	Description string
	RetryAfter  time.Duration
}

func (e *APIError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("telegram api error %d: %s (retry after %s)", e.Code, e.Description, e.RetryAfter)
	}
	return fmt.Sprintf("telegram api error %d: %s", e.Code, e.Description)
}

// Temporary reports whether retrying the same request can reasonably succeed.
func (e *APIError) Temporary() bool {
	return e.Code == http.StatusTooManyRequests || e.Code >= http.StatusInternalServerError
}

type temporaryError struct {
	err error
}

func (e *temporaryError) Error() string { return e.err.Error() }
func (e *temporaryError) Unwrap() error { return e.err }

func isTemporary(err error) bool {
	var temp *temporaryError
	if errors.As(err, &temp) {
		return true
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Temporary()
	}

	return false
}

func (c *Client) redact(err error) error {
	if err == nil {
		return nil
	}

	message := err.Error()
	if c.token != "" {
		message = strings.ReplaceAll(message, c.token, "<redacted>")
	}

	return errors.New(message)
}

func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) error {
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	_, err := c.call(ctx, http.MethodPost, "sendMessage", nil, sendMessageRequest{
		ChatID:    chatID,
		Text:      text,
		ParseMode: "HTML",
	})

	return err
}

func (c *Client) GetMe(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, getMeTimeout)
	defer cancel()

	response, err := c.call(ctx, http.MethodGet, "getMe", nil, nil)
	if err != nil {
		return "", err
	}

	var result struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return "", fmt.Errorf("decode getMe response: %w", err)
	}

	return result.Username, nil
}

func (c *Client) GetUpdates(ctx context.Context, offset int64) ([]telegramUpdate, error) {
	ctx, cancel := context.WithTimeout(ctx, updatesTimeout+15*time.Second)
	defer cancel()

	query := url.Values{}
	query.Set("offset", strconv.FormatInt(offset, 10))
	query.Set("timeout", strconv.Itoa(int(updatesTimeout.Seconds())))
	query.Set("allowed_updates", `["message"]`)

	response, err := c.call(ctx, http.MethodGet, "getUpdates", query, nil)
	if err != nil {
		return nil, err
	}

	if len(response.Result) == 0 {
		return nil, nil
	}

	var updates []telegramUpdate
	if err := json.Unmarshal(response.Result, &updates); err != nil {
		return nil, fmt.Errorf("decode getUpdates response: %w", err)
	}

	return updates, nil
}

func (c *Client) call(
	ctx context.Context,
	httpMethod string,
	method string,
	query url.Values,
	payload any,
) (*apiResponse, error) {
	endpoint := c.baseURL + "/bot" + c.token + "/" + method
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encode telegram request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, httpMethod, endpoint, body)
	if err != nil {
		return nil, &temporaryError{err: c.redact(err)}
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, &temporaryError{err: c.redact(err)}
	}
	defer response.Body.Close()

	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return nil, &temporaryError{err: fmt.Errorf("read telegram response: %w", err)}
	}

	var result apiResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, &APIError{
			Code:        response.StatusCode,
			Description: truncate(strings.TrimSpace(string(data)), 300),
		}
	}

	if response.StatusCode == http.StatusOK && result.OK {
		return &result, nil
	}

	code := result.ErrorCode
	if code == 0 {
		code = response.StatusCode
	}

	apiErr := &APIError{
		Code:        code,
		Description: result.Description,
	}
	if result.Parameters != nil && result.Parameters.RetryAfter > 0 {
		apiErr.RetryAfter = time.Duration(result.Parameters.RetryAfter) * time.Second
	}

	return nil, apiErr
}

type telegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *telegramMessage `json:"message"`
}

type telegramMessage struct {
	Text string        `json:"text"`
	From *telegramUser `json:"from"`
	Chat *telegramChat `json:"chat"`
}

type telegramUser struct {
	ID int64 `json:"id"`
}

type telegramChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}
