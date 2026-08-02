package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewMemePluginUsesDefaultAPIURL(t *testing.T) {
	m := newMemePlugin()
	if got := m.apiURL("/meme/infos"); got != defaultMemeAPIURL+"/meme/infos" {
		t.Fatalf("apiURL() = %q, want %q", got, defaultMemeAPIURL+"/meme/infos")
	}
}

func TestAPIURLFallsBackWhenInjectedURLIsEmpty(t *testing.T) {
	m := newMemePlugin()
	m.Config.Url = ""
	if got := m.apiURL("/meme/infos"); got != defaultMemeAPIURL+"/meme/infos" {
		t.Fatalf("apiURL() = %q, want %q", got, defaultMemeAPIURL+"/meme/infos")
	}
}

func TestLoadCacheUsesConfiguredAPIURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/meme/infos" {
			t.Fatalf("path = %q, want /meme/infos", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"key":"hit","keywords":["揍"]}]`))
	}))
	defer server.Close()

	m := newMemePlugin()
	m.Config.Url = server.URL + "/"
	if err := m.loadCache(); err != nil {
		t.Fatalf("loadCache() error = %v", err)
	}
	if got := m.lookupByKeyword("揍"); got == nil || got.Key != "hit" {
		t.Fatalf("lookupByKeyword() = %#v", got)
	}
}

func TestOnLoadDoesNotFailHostWhenMemeAPIIsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	server.Close()

	m := newMemePlugin()
	m.Config.Url = server.URL
	if err := m.OnLoad(); err != nil {
		t.Fatalf("OnLoad() error = %v, want nil", err)
	}
}
