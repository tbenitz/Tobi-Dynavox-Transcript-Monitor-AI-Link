package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestContext(t *testing.T) {
	if got := recent("First sentence. Next? Last one!", "sentences", 2); got != "Next? Last one!" {
		t.Fatal(got)
	}
	if got := recent("a\nb   c d", "words", 2); got != "c d" {
		t.Fatal(got)
	}
	if got := recent("First line\nSecond line\nThird line", "sentences", 2); got != "Second line Third line" {
		t.Fatal(got)
	}
}
func TestResponseAndDraft(t *testing.T) {
	got := offline("Would you like coffee or tea?", "I would like ", hybrid)
	if len(got) != 5 || got[0].Text != "Coffee, please." || got[1].Text != "Tea, please." || !got[2].Word {
		t.Fatalf("%+v", got)
	}
	if got := insertion(suggestion{"Water", true}, "I want wa", true, false); got != "ter " {
		t.Fatal(got)
	}
	if got := insertion(suggestion{"No thanks", false}, "", false, true); got != "No thanks." {
		t.Fatal(got)
	}
	for _, mode := range []string{quick, words, hybrid} {
		got := offline("", "", mode)
		if len(got) < 3 || len(got) > 5 {
			t.Fatal(got)
		}
	}
}
func TestPrivacyResumeAndRotation(t *testing.T) {
	p := filepath.Join(t.TempDir(), "live.txt")
	w := func(s string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(s), 0600); err != nil {
			t.Fatal(err)
		}
	}
	w("before\n")
	var f follower
	if got, _ := f.read(p); got != "before\n" {
		t.Fatal(got)
	}
	w("before\nprivate\n")
	f.resume(p)
	if got, _ := f.read(p); got != "" {
		t.Fatalf("leaked %q", got)
	}
	w("before\nprivate\npublic\n")
	if got, _ := f.read(p); got != "public\n" {
		t.Fatal(got)
	}
	w("replacement with unknown material\n")
	if got, _ := f.read(p); got != "" {
		t.Fatalf("rotation leaked %q", got)
	}
	w("replacement with unknown material\nnext\n")
	if got, _ := f.read(p); got != "next\n" {
		t.Fatal(got)
	}
}
func TestOutputAndPayload(t *testing.T) {
	got := parseModel(`{"replies":["Okay","okay","<b>foo</b>"],"words":["tea","two words","water","coffee"]}`, "")
	if len(got.Replies) != 1 || len(got.Words) != 3 {
		t.Fatalf("%+v", got)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" || r.Header.Get("Authorization") != "" {
			t.Error("bad endpoint or authorization")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["model"] != "qwen2.5:0.5b-instruct" || body["stream"] != false {
			t.Errorf("%v %v", body, err)
		}
		w.Write([]byte(`{"message":{"content":"{\"replies\":[\"Yes, please.\"],\"words\":[\"tea\"]}"}}`))
	}))
	defer srv.Close()
	out, err := predict(context.Background(), srv.Client(), srv.URL+"/api/chat", "ollama", "qwen2.5:0.5b-instruct", "", "Would you like tea?", "", hybrid)
	if err != nil || len(out.Words) != 1 || out.Words[0] != "tea" {
		t.Fatalf("%+v %v", out, err)
	}
}
func TestHotkeyAndDelay(t *testing.T) {
	mods, vk, err := parseHotkey("Ctrl + Space")
	if mods != 2 || vk != 32 || err != nil {
		t.Fatal(mods, vk, err)
	}
	if _, _, err := parseHotkey("A"); err == nil {
		t.Fatal("unmodified letters should not be captured")
	}
	now := time.Now()
	if mayOpen(now, now, 500) || !mayOpen(now.Add(time.Second), now, 500) {
		t.Fatal("debounce")
	}
}
func TestBoundedTail(t *testing.T) {
	p := filepath.Join(t.TempDir(), "live.txt")
	if err := os.WriteFile(p, []byte("hidden\n"+strings.Repeat("a", 20000)+"\nrecent"), 0600); err != nil {
		t.Fatal(err)
	}
	var f follower
	got, err := f.read(p)
	if err != nil || got != "recent" {
		t.Fatal(len(got), err)
	}
}
