package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveLocalPathUnderRoot(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "files")
	if err := mkdirAll(sub); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(sub, "a.txt")
	if err := writeFile(f, "x"); err != nil {
		t.Fatal(err)
	}

	cfg := MailConfig{
		ConfigBaseDir:  root,
		AttachmentRoot: "files",
	}
	got, err := cfg.resolveLocalPath("a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got != f {
		t.Fatalf("resolveLocalPath: got %q want %q", got, f)
	}
}

func TestResolveLocalPathRejectsEscape(t *testing.T) {
	root := t.TempDir()
	cfg := MailConfig{
		ConfigBaseDir:  root,
		AttachmentRoot: "safe",
	}
	if err := mkdirAll(filepath.Join(root, "safe")); err != nil {
		t.Fatal(err)
	}
	_, err := cfg.resolveLocalPath("../outside")
	if err == nil {
		t.Fatal("expected error for path escape")
	}
}

func mkdirAll(p string) error {
	return os.MkdirAll(p, 0o755)
}

func writeFile(p, content string) error {
	return os.WriteFile(p, []byte(content), 0o644)
}
