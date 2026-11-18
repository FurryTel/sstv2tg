package telegram

import (
	"context"
	"strings"
	"testing"
)

func TestSendPhotoRedactsTokenOnTransportError(t *testing.T) {
	c := New("SECRETTOKEN", 1, 0)
	c.base = "http://127.0.0.1:1" // nothing listening
	err := c.SendPhoto(context.Background(), []byte("x"), "")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "SECRETTOKEN") {
		t.Fatalf("error leaks token: %v", err)
	}
}
