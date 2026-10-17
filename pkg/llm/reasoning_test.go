package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStreamChatSendsSupportedReasoningEffort(t *testing.T) {
	var received []ReasoningEffort
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		var effort ReasoningEffort
		if err := json.Unmarshal(body["reasoning_effort"], &effort); err != nil {
			t.Errorf("decode reasoning_effort: %v", err)
		}
		received = append(received, effort)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"stop after inspecting request"}}`))
	}))
	defer server.Close()

	client := &Client{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()}
	for _, effort := range []ReasoningEffort{ReasoningEffortNone, ReasoningEffortLow, ReasoningEffortHigh, ReasoningEffortMax} {
		_, _, err := client.StreamChat(context.Background(), ChatRequest{
			Model: "deepseek-v4-flash", ReasoningEffort: effort,
			Messages: []Message{{Role: "user", Content: "test"}},
		}, nil)
		if err == nil {
			t.Fatalf("effort %q: expected the test endpoint to reject the request", effort)
		}
	}
	if len(received) != 4 {
		t.Fatalf("received %d requests, want 4", len(received))
	}
	for i, want := range []ReasoningEffort{ReasoningEffortNone, ReasoningEffortLow, ReasoningEffortHigh, ReasoningEffortMax} {
		if received[i] != want {
			t.Errorf("request %d reasoning effort = %q, want %q", i, received[i], want)
		}
	}
}

func TestReasoningEffortValidation(t *testing.T) {
	for _, effort := range []ReasoningEffort{ReasoningEffortNone, ReasoningEffortLow, ReasoningEffortHigh, ReasoningEffortMax} {
		if !effort.Valid() {
			t.Errorf("%q should be valid", effort)
		}
	}
	if ReasoningEffort("medium").Valid() {
		t.Fatal("unsupported effort should be invalid")
	}
}
