package main

import (
	"testing"
)

func TestParseMailUIDs(t *testing.T) {
	t.Parallel()
	got, err := parseMailUIDs(map[string]any{"uids": []any{float64(42), float64(1)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != 1 || got[1] != 42 {
		t.Fatalf("got %v want [1 42]", got)
	}

	_, err = parseMailUIDs(map[string]any{"uids": []any{}})
	if err == nil {
		t.Fatal("empty uids want error")
	}

	var tooMany []any
	for i := 0; i < maxMailModifyUIDs+1; i++ {
		tooMany = append(tooMany, float64(i+1))
	}
	_, err = parseMailUIDs(map[string]any{"uids": tooMany})
	if err == nil {
		t.Fatal("too many uids want error")
	}

	got, err = parseMailUIDs(map[string]any{"uids": []any{float64(7), float64(7)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != 7 {
		t.Fatalf("dedup: got %v", got)
	}
}
