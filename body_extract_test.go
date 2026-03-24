package main

import (
	"strings"
	"testing"
)

func TestExtractBodiesFromRFC822(t *testing.T) {
	raw := "Subject: hi\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nhello world"
	plain, html, att, err := extractBodiesFromRFC822(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if plain != "hello world" {
		t.Fatalf("plain=%q", plain)
	}
	if html != "" {
		t.Fatalf("html=%q", html)
	}
	if len(att) != 0 {
		t.Fatalf("attachments=%v", att)
	}
}
