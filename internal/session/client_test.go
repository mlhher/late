package session

import (
	"late/internal/client"
	"testing"
)

func TestSetClient(t *testing.T) {
	original := client.NewClient(client.Config{Model: "original"})
	replacement := client.NewClient(client.Config{Model: "replacement"})
	s := New(original, "", nil, "", false)

	s.SetClient(replacement)

	if got := s.Client(); got != replacement {
		t.Fatalf("Client() = %p, want replacement %p", got, replacement)
	}
}
