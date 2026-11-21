package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A database or room key copied in from another user must be handed to the
// service user, not just the directory: otherwise KAM opens SQLite read-only
// and silently stops storing group texts.
func TestEnsureLayoutChownsExistingDataFiles(t *testing.T) {
	root := t.TempDir()
	a := &app{
		binDir:    filepath.Join(root, "bin"),
		configDir: filepath.Join(root, "etc"),
		dataDir:   filepath.Join(root, "data"),
		unitDir:   filepath.Join(root, "units"),
	}
	if err := os.MkdirAll(a.dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"kam.db", "kam.db-wal", "kam.db-shm"} {
		if err := os.WriteFile(filepath.Join(a.dataDir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(a.roomsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.roomsDir(), "lobby.key"), []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}

	var commands []string
	a.run = func(name string, args ...string) ([]byte, error) {
		commands = append(commands, strings.Join(append([]string{name}, args...), " "))
		return nil, nil
	}

	if err := ensureLayout(a); err != nil {
		t.Fatalf("layout: %v", err)
	}
	joined := strings.Join(commands, "\n")
	for _, want := range []string{
		"chown kamstack:kamstack " + filepath.Join(a.dataDir, "kam.db"),
		"chown kamstack:kamstack " + filepath.Join(a.dataDir, "kam.db-wal"),
		"chown kamstack:kamstack " + filepath.Join(a.dataDir, "kam.db-shm"),
		"chown -R kamstack:kamstack " + a.roomsDir(),
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in:\n%s", want, joined)
		}
	}
}

func TestEnsureLayoutSkipsMissingDatabaseFiles(t *testing.T) {
	root := t.TempDir()
	a := &app{
		binDir:    filepath.Join(root, "bin"),
		configDir: filepath.Join(root, "etc"),
		dataDir:   filepath.Join(root, "data"),
		unitDir:   filepath.Join(root, "units"),
	}
	var commands []string
	a.run = func(name string, args ...string) ([]byte, error) {
		commands = append(commands, strings.Join(append([]string{name}, args...), " "))
		return nil, nil
	}

	if err := ensureLayout(a); err != nil {
		t.Fatalf("layout: %v", err)
	}
	if strings.Contains(strings.Join(commands, "\n"), "kam.db") {
		t.Fatalf("chowned a database that does not exist:\n%s", strings.Join(commands, "\n"))
	}
}
