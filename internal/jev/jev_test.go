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
	if _, err := Evaluate(context.Background(), server.URL, "test-key", Build("test", "jev-1.13.0")); err == nil {
		t.Fatal("accepted invalid response")
	}
}
