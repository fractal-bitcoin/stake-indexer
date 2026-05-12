package stateapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetHeightStateQueryFIP101(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/brc20/statehash" {
			t.Fatalf("path = %q, want /brc20/statehash", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"code":0,"msg":"ok","data":{"detail":[{"blockHash":"","stateHash":"abcd"}]}}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		http:    &http.Client{Timeout: time.Second},
	}

	state, err := client.GetHeightState(context.Background(), 100)
	if err != nil {
		t.Fatalf("GetHeightState() error = %v", err)
	}
	if state.StateHash != "abcd" {
		t.Fatalf("state hash = %q, want abcd", state.StateHash)
	}
}
