package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func configMap(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var values map[string]any
	if err := yaml.Unmarshal(raw, &values); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return values
}

func TestRenderKamConfigSetsInstallValues(t *testing.T) {
	raw, err := renderKamConfig("pi-kam", "/var/lib/kamstack/kam.db", "/dev/serial/by-id/usb-radio")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	cfg := configMap(t, raw)
	if cfg["node_name"] != "pi-kam" {
		t.Errorf("node_name = %v", cfg["node_name"])
	}
	if cfg["db_path"] != "/var/lib/kamstack/kam.db" {
		t.Errorf("db_path = %v", cfg["db_path"])
	}
	radio, _ := cfg["radio"].(map[string]any)
	if radio["port"] != "/dev/serial/by-id/usb-radio" {
		t.Errorf("radio.port = %v", radio["port"])
	}
	channels, _ := cfg["channels"].([]any)
	if len(channels) == 0 {
		t.Fatal("the bundled channels were dropped")
	}
}

func TestRenderKammonConfigSetsPasswordsAndURL(t *testing.T) {
	raw, err := renderKammonConfig("admin-secret", "viewer-secret", "http://127.0.0.1:8766")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(string(raw), "change-me") {
		t.Fatal("the placeholder passwords survived")
	}
	cfg := configMap(t, raw)
	if cfg["kam_monitor_url"] != "http://127.0.0.1:8766" {
		t.Errorf("kam_monitor_url = %v", cfg["kam_monitor_url"])
	}
	users, _ := cfg["users"].([]any)
	got := map[string]string{}
	for _, entry := range users {
		user, _ := entry.(map[string]any)
		got[user["name"].(string)] = user["password"].(string)
	}
	if got["admin"] != "admin-secret" || got["viewer"] != "viewer-secret" {
		t.Fatalf("users = %v", got)
	}
}

func TestWriteConfigIsGroupWritable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "etc", "kamstack", "kam.yml")
	if err := writeConfig(path, []byte("version: 1\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o660 {
		t.Fatalf("mode = %o, want 660 (KAM rewrites the file)", info.Mode().Perm())
	}
}

func TestUpdateKamPortKeepsOtherValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kam.yml")
	raw, err := renderKamConfig("keep-me", "/tmp/kam.db", "/dev/ttyUSB0")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeConfig(path, raw); err != nil {
		t.Fatal(err)
	}
	if err := updateKamPort(path, "/dev/serial/by-id/usb-new"); err != nil {
		t.Fatalf("update: %v", err)
	}
	doc, err := readConfigNode(path)
	if err != nil {
		t.Fatal(err)
	}
	radio, err := childMapping(documentRoot(doc), "radio")
	if err != nil {
		t.Fatal(err)
	}
	if got := getKey(radio, "port"); got != "/dev/serial/by-id/usb-new" {
		t.Fatalf("port = %q", got)
	}
	if got := getKey(documentRoot(doc), "node_name"); got != "keep-me" {
		t.Fatalf("node_name = %q", got)
	}
}
