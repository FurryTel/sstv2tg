package telegram

import (
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSendPhotoFields(t *testing.T) {
	for _, tc := range []struct {
		name     string
		threadID int
		want     map[string]string
	}{
		{"chat", 0, map[string]string{"chat_id": "-42", "caption": "hi", "photo": "PNGDATA"}},
		{"topic", 7, map[string]string{"chat_id": "-42", "caption": "hi", "photo": "PNGDATA", "message_thread_id": "7"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got map[string]string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/bottoken/sendPhoto" {
					t.Errorf("unexpected path %q", r.URL.Path)
				}
				got = readForm(t, r)
				io.WriteString(w, `{"ok":true}`)
			}))
			defer srv.Close()

			c := New("token", -42, tc.threadID)
			c.base = srv.URL
			if err := c.SendPhoto(context.Background(), []byte("PNGDATA"), "hi"); err != nil {
				t.Fatalf("SendPhoto: %v", err)
			}
			if !maps.Equal(got, tc.want) {
				t.Errorf("form %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSendPhotoRetriesOn5xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, `{"ok":false,"description":"boom"}`)
			return
		}
		io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	c := New("token", 1, 0)
	c.base = srv.URL
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.SendPhoto(ctx, []byte("x"), ""); err != nil {
		t.Fatalf("SendPhoto: %v", err)
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("calls = %d, want 3", got)
	}
}

func TestSendPhotoWaitsOutRateLimit(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"ok":false,"description":"Too Many Requests: retry after 1","parameters":{"retry_after":1}}`)
			return
		}
		io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	c := New("token", 1, 0)
	c.base = srv.URL
	start := time.Now()
	if err := c.SendPhoto(context.Background(), []byte("x"), ""); err != nil {
		t.Fatalf("SendPhoto: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("calls = %d, want 2", got)
	}
	if waited := time.Since(start); waited < time.Second {
		t.Errorf("retried after %v, want at least the 1s retry_after", waited)
	}
}

func TestSendPhotoGivesUpOn4xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"ok":false,"description":"chat not found"}`)
	}))
	defer srv.Close()

	c := New("token", 1, 0)
	c.base = srv.URL
	err := c.SendPhoto(context.Background(), []byte("x"), "")
	if err == nil {
		t.Fatal("expected error")
	}
	var tErr *terminalError
	if !errors.As(err, &tErr) {
		t.Errorf("err = %v, want terminalError", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("calls = %d, want 1", got)
	}
}

func readForm(t *testing.T, r *http.Request) map[string]string {
	t.Helper()
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("content-type: %v", err)
	}
	form := map[string]string{}
	mr := multipart.NewReader(r.Body, params["boundary"])
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			return form
		}
		if err != nil {
			t.Fatalf("next part: %v", err)
		}
		buf := &bytes.Buffer{}
		if _, err := io.Copy(buf, part); err != nil {
			t.Fatalf("copy: %v", err)
		}
		form[part.FormName()] = buf.String()
	}
}

func TestParseResultDescribesFailure(t *testing.T) {
	resp := &http.Response{
		StatusCode: 403,
		Body:       io.NopCloser(strings.NewReader(`{"ok":false,"description":"forbidden"}`)),
	}
	err := parseResult(resp)
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("err = %v", err)
	}
}
