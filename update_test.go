package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kamstack.bytecoder.nl/internal/stack"
)

func makeArchive(t *testing.T, binary, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Name:     "kamstack_pkg/" + binary,
		Mode:     0o755,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// updateFixture serves a manifest and both archives for version 0.8.0.
type updateFixture struct {
	app      *app
	commands *[]string
	server   *httptest.Server
	kam      []byte
	kammon   []byte
}

func newUpdateFixture(t *testing.T, installedKam, installedKammon string) *updateFixture {
	t.Helper()
	root := t.TempDir()
	a := &app{
		binDir:        filepath.Join(root, "bin"),
		configDir:     filepath.Join(root, "etc"),
		dataDir:       filepath.Join(root, "data"),
		unitDir:       filepath.Join(root, "units"),
		stdout:        &bytes.Buffer{},
		stderr:        &bytes.Buffer{},
		fetch:         downloadURL,
		healthTimeout: 0, // one attempt
		sleep:         func(time.Duration) {},
		target:        func() string { return "linux/amd64" },
	}
	if err := os.MkdirAll(a.binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(a.dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, version := range map[string]string{"kam": installedKam, "kammon": installedKammon} {
		if version == "" {
			continue
		}
		if err := os.WriteFile(a.binary(name), []byte("old-"+name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	commands := []string{}
	a.run = func(name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		switch name {
		case a.binary("kam"):
			return []byte("kam " + installedKam + " linux/amd64\n"), nil
		case a.binary("kammon"):
			return []byte("kammon " + installedKammon + " linux/amd64\n"), nil
		}
		return nil, nil
	}
	a.httpGet = func(string, time.Duration) (int, error) { return 200, nil }

	fixture := &updateFixture{app: a, commands: &commands}
	fixture.kam = makeArchive(t, "kam", "new-kam")
	fixture.kammon = makeArchive(t, "kammon", "new-kammon")

	mux := http.NewServeMux()
	fixture.server = httptest.NewServer(mux)
	t.Cleanup(fixture.server.Close)

	mux.HandleFunc("/kam.tar.gz", func(w http.ResponseWriter, _ *http.Request) { w.Write(fixture.kam) })
	mux.HandleFunc("/kammon.tar.gz", func(w http.ResponseWriter, _ *http.Request) { w.Write(fixture.kammon) })
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, _ *http.Request) {
		manifest := stack.Manifest{
			Version: "0.8.0",
			Kam: stack.Component{Version: "0.8.0", Targets: map[string]stack.Target{
				"linux/amd64": {URL: fixture.server.URL + "/kam.tar.gz", SHA256: sha256Hex(fixture.kam)},
			}},
			Kammon: stack.Component{Version: "0.8.0", Targets: map[string]stack.Target{
				"linux/amd64": {URL: fixture.server.URL + "/kammon.tar.gz", SHA256: sha256Hex(fixture.kammon)},
			}},
		}
		raw, _ := json.Marshal(manifest)
		w.Write(raw)
	})
	a.manifestURL = fixture.server.URL + "/manifest.json"
	return fixture
}

func TestUpdateCheckReportsOutdatedAndExitsTwo(t *testing.T) {
	fixture := newUpdateFixture(t, "0.7.0", "0.7.0")
	err := runUpdate(fixture.app, []string{"--check"})
	var status *exitStatus
	if !errors.As(err, &status) || status.ExitCode() != 2 {
		t.Fatalf("error = %v, want exit status 2", err)
	}
	out := fixture.app.stdout.(*bytes.Buffer).String()
	if !strings.Contains(out, "0.7.0") || !strings.Contains(out, "0.8.0") {
		t.Fatalf("check output = %q", out)
	}
}

func TestUpdateCheckReportsCurrent(t *testing.T) {
	fixture := newUpdateFixture(t, "0.8.0", "0.8.0")
	if err := runUpdate(fixture.app, []string{"--check"}); err != nil {
		t.Fatalf("check: %v", err)
	}
}

func TestUpdateVerifiesThenStopsBacksUpAndSwaps(t *testing.T) {
	fixture := newUpdateFixture(t, "0.7.0", "0.7.0")
	if err := os.WriteFile(filepath.Join(fixture.app.dataDir, "kam.db"), []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(fixture.app.roomsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture.app.roomsDir(), "lobby.key"), []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := runUpdate(fixture.app, nil); err != nil {
		t.Fatalf("update: %v", err)
	}

	if got := readFile(t, fixture.app.binary("kam")); got != "new-kam" {
		t.Errorf("kam binary = %q", got)
	}
	if got := readFile(t, fixture.app.binary("kammon")); got != "new-kammon" {
		t.Errorf("kammon binary = %q", got)
	}
	if got := readFile(t, fixture.app.binary("kam")+".prev"); got != "old-kam" {
		t.Errorf("kam .prev = %q", got)
	}
	joined := strings.Join(*fixture.commands, "\n")
	if !strings.Contains(joined, "systemctl stop") || !strings.Contains(joined, "systemctl start") {
		t.Fatalf("commands = %v", *fixture.commands)
	}

	backups, err := os.ReadDir(filepath.Join(fixture.app.dataDir, "backups"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups = %v, %v", backups, err)
	}
	entries := archiveEntries(t, filepath.Join(fixture.app.dataDir, "backups", backups[0].Name()))
	for _, want := range []string{"kam.db", "rooms/lobby.key"} {
		if !entries[want] {
			t.Errorf("backup missing %q: %v", want, entries)
		}
	}
}

func TestUpdateRefusesChecksumMismatchBeforeStopping(t *testing.T) {
	fixture := newUpdateFixture(t, "0.7.0", "0.7.0")
	// Replace the manifest with a wrong checksum.
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, _ *http.Request) {
		manifest := stack.Manifest{
			Version: "0.8.0",
			Kam: stack.Component{Version: "0.8.0", Targets: map[string]stack.Target{
				"linux/amd64": {URL: server.URL + "/kam.tar.gz", SHA256: "deadbeef"},
			}},
			Kammon: stack.Component{Version: "0.8.0", Targets: map[string]stack.Target{
				"linux/amd64": {URL: server.URL + "/kammon.tar.gz", SHA256: sha256Hex(fixture.kammon)},
			}},
		}
		raw, _ := json.Marshal(manifest)
		w.Write(raw)
	})
	mux.HandleFunc("/kam.tar.gz", func(w http.ResponseWriter, _ *http.Request) { w.Write(fixture.kam) })
	mux.HandleFunc("/kammon.tar.gz", func(w http.ResponseWriter, _ *http.Request) { w.Write(fixture.kammon) })

	err := runUpdate(fixture.app, []string{"--manifest-url", server.URL + "/manifest.json"})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error = %v", err)
	}
	for _, command := range *fixture.commands {
		if strings.Contains(command, "systemctl stop") {
			t.Fatalf("services were stopped despite a bad checksum: %v", *fixture.commands)
		}
	}
	if got := readFile(t, fixture.app.binary("kam")); got != "old-kam" {
		t.Fatalf("binary changed after a failed verification: %q", got)
	}
}

func TestUpdateRollsBackWhenHealthCheckFails(t *testing.T) {
	fixture := newUpdateFixture(t, "0.7.0", "0.7.0")
	fixture.app.httpGet = func(string, time.Duration) (int, error) { return 500, nil }

	err := runUpdate(fixture.app, nil)
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("error = %v", err)
	}
	if got := readFile(t, fixture.app.binary("kam")); got != "old-kam" {
		t.Fatalf("kam binary = %q, want the previous binary", got)
	}
	starts := 0
	for _, command := range *fixture.commands {
		if strings.Contains(command, "systemctl start") {
			starts++
		}
	}
	if starts != 2 {
		t.Fatalf("systemctl start ran %d times, want the update start and the rollback start: %v", starts, *fixture.commands)
	}
}

func TestCreateBackupWithoutStateDoesNothing(t *testing.T) {
	a := &app{dataDir: t.TempDir()}
	path, err := createBackup(a, "0.8.0")
	if err != nil || path != "" {
		t.Fatalf("backup = %q, %v", path, err)
	}
}

func TestPruneBackupsKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	names := []string{
		"2026-01-01-0000-0.7.0.tar.gz",
		"2026-01-02-0000-0.7.0.tar.gz",
		"2026-01-03-0000-0.7.0.tar.gz",
		"2026-01-04-0000-0.8.0.tar.gz",
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := pruneBackups(dir, 3); err != nil {
		t.Fatal(err)
	}
	remaining, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 3 {
		t.Fatalf("remaining = %v", remaining)
	}
	if _, err := os.Stat(filepath.Join(dir, names[0])); !os.IsNotExist(err) {
		t.Fatal("the oldest backup was not pruned")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

func archiveEntries(t *testing.T, path string) map[string]bool {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	entries := map[string]bool{}
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if err != nil {
			break
		}
		entries[header.Name] = true
	}
	return entries
}
