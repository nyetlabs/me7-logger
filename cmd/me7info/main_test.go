package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestECUOutputDefault(t *testing.T) {
	base := t.TempDir()
	got, err := ecuOutput(filepath.Join("dir", "foo.bin"), "", base)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, "ecus", "foo.ecu")
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	st, err := os.Stat(filepath.Join(base, "ecus"))
	if err != nil || !st.IsDir() {
		t.Fatalf("ecus dir: %v", err)
	}
	got, err = ecuOutput("foo.BIN", "", base)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("existing dir: got %s, want %s", got, want)
	}
}

func TestECUOutputDirIsFile(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "ecus"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ecuOutput("foo.bin", "", base)
	if err == nil || got != "" || !strings.HasPrefix(err.Error(), "ecus:") {
		t.Fatalf("got %q, err %v", got, err)
	}
}

func TestECUOutputExplicit(t *testing.T) {
	for _, flag := range []string{"other.ecu", "-"} {
		base := t.TempDir()
		got, err := ecuOutput("foo.bin", flag, base)
		if err != nil {
			t.Fatal(err)
		}
		if got != flag {
			t.Fatalf("got %q, want %q", got, flag)
		}
		if _, err := os.Stat(filepath.Join(base, "ecus")); !os.IsNotExist(err) {
			t.Fatalf("%q created ecus: %v", flag, err)
		}
	}
}
