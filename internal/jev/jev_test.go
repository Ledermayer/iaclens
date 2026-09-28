package jev

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRejectInvalidModelAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing auth")
		}
		fmt.Fprint(w, `{"model":"jev-1.13.0","answers":{"domain":{"type":"choice","choice":"invented","confidence":0.9}}}`)
	}))
	defer server.Close()
	if _, err := Evaluate(context.Background(), server.URL, "test-key", Build("test", "jev-1.13.0", map[string]Question{"domain": {Type: "choice", Criteria: map[string]string{"known": "Known"}}})); err == nil {
		t.Fatal("accepted invalid response")
	}
}

func TestTemporaryServiceFailureRetries(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(503)
			return
		}
		fmt.Fprint(w, `{"model":"jev","answers":{}}`)
	}))
	defer server.Close()
	if _, err := Evaluate(context.Background(), server.URL, "test", Build("test", "jev", map[string]Question{})); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestPermanentFailureDoesNotRetry(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(401) }))
	defer server.Close()
	if _, err := Evaluate(context.Background(), server.URL, "test", Build("test", "jev", nil)); err == nil {
		t.Fatal("accepted unauthorized response")
	}
	if calls != 1 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestServiceFailureRetryBound(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(503) }))
	defer server.Close()
	if _, err := Evaluate(context.Background(), server.URL, "test", Build("test", "jev", nil)); err == nil {
		t.Fatal("accepted unavailable response")
	}
	if calls != 3 {
		t.Fatalf("calls = %d", calls)
	}
}
