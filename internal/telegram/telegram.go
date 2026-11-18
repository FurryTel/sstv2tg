// Package telegram posts images to one Telegram chat or forum topic via the
// Bot API.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBase = "https://api.telegram.org"
	maxAttempts = 3
)

// Client talks to the Telegram Bot API.
type Client struct {
	token    string
	chatID   int64
	threadID int
	base     string
	http     *http.Client
}

// New returns a client that posts to chatID using the given bot token. A
// nonzero threadID posts into that forum topic.
func New(token string, chatID int64, threadID int) *Client {
	return &Client{
		token:    token,
		chatID:   chatID,
		threadID: threadID,
		base:     defaultBase,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

// GetMe verifies the bot token. Returns an error if Telegram rejects it.
func (c *Client) GetMe(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url("getMe"), nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return c.redact(err)
	}
	defer resp.Body.Close()
	return parseResult(resp)
}

// SendPhoto posts png as a photo with caption. Retries on 5xx, 429 and
// network errors up to maxAttempts with exponential backoff, or as long as a
// 429 asks.
func (c *Client) SendPhoto(ctx context.Context, png []byte, caption string) error {
	var last error
	backoff := 500 * time.Millisecond
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := c.sendPhotoOnce(ctx, png, caption)
		if err == nil {
			return nil
		}
		last = err
		var tErr *terminalError
		if errors.As(err, &tErr) {
			return err
		}
		if attempt < maxAttempts {
			wait := backoff
			var rl *rateLimitError
			if errors.As(err, &rl) && rl.wait > 0 {
				wait = rl.wait
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
			backoff *= 2
		}
	}
	return last
}

func (c *Client) sendPhotoOnce(ctx context.Context, png []byte, caption string) error {
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fields := [][2]string{
		{"chat_id", strconv.FormatInt(c.chatID, 10)},
		{"caption", caption},
	}
	if c.threadID != 0 {
		fields = append(fields, [2]string{"message_thread_id", strconv.Itoa(c.threadID)})
	}
	for _, f := range fields {
		if err := mw.WriteField(f[0], f[1]); err != nil {
			return err
		}
	}
	part, err := mw.CreateFormFile("photo", "image.png")
	if err != nil {
		return err
	}
	if _, err := part.Write(png); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url("sendPhoto"), body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		return c.redact(err)
	}
	defer resp.Body.Close()
	return parseResult(resp)
}

// redact scrubs the bot token from net/http's wrapped *url.Error before it
// bubbles up into logs.
func (c *Client) redact(err error) error {
	var uErr *url.Error
	if errors.As(err, &uErr) {
		uErr.URL = strings.Replace(uErr.URL, c.token, "<redacted>", 1)
	}
	return err
}

func (c *Client) url(method string) string {
	return fmt.Sprintf("%s/bot%s/%s", c.base, c.token, method)
}

// terminalError marks a 4xx response that will not succeed on retry.
type terminalError struct{ msg string }

func (e *terminalError) Error() string { return e.msg }

// rateLimitError is a 429, carrying how long Telegram asked us to wait.
type rateLimitError struct {
	msg  string
	wait time.Duration
}

func (e *rateLimitError) Error() string { return e.msg }

func parseResult(resp *http.Response) error {
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var payload struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Parameters  struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("telegram: parse %d: %w", resp.StatusCode, err)
	}
	if payload.OK {
		return nil
	}
	msg := fmt.Sprintf("telegram: %d: %s", resp.StatusCode, payload.Description)
	if resp.StatusCode == http.StatusTooManyRequests {
		return &rateLimitError{msg: msg, wait: time.Duration(payload.Parameters.RetryAfter) * time.Second}
	}
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return &terminalError{msg: msg}
	}
	return errors.New(msg)
}
