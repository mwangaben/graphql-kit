package handlers

import (
	"strings"
	"testing"
)

// Light tests — just verify the HTML is well-formed enough.
// The content itself is stable, so we don't test specific strings beyond
// the essentials.

func TestPlaygroundHTML_ContainsEssentials(t *testing.T) {
	essentials := []string{
		"<!DOCTYPE html>",
		"GraphQL Playground",
		"subscriptionEndpoint",
		"wsUrl",
		"cdn.jsdelivr.net",
	}

	for _, want := range essentials {
		if !strings.Contains(playgroundHTML, want) {
			t.Errorf("playground HTML missing %q", want)
		}
	}
}

func TestPlaygroundHTML_WebSocketEndpoint(t *testing.T) {
	// Playground should use /subscriptions by default
	if !strings.Contains(playgroundHTML, "/subscriptions") {
		t.Error("playground should point to /subscriptions WS endpoint")
	}
}

func TestPlaygroundHTML_UsesWSSProtocol(t *testing.T) {
	// Should choose wss for https, ws for http
	if !strings.Contains(playgroundHTML, "wss:") {
		t.Error("playground should use wss protocol")
	}
	if !strings.Contains(playgroundHTML, "ws:") {
		t.Error("playground should use ws protocol")
	}
}
