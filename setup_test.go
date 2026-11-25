package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSetupWritesLayoutConfigsUnitsAndStarts(t *testing.T) {
	root := t.TempDir()
	a := &app{
		binDir:      filepath.Join(root, "opt", "kamstack", "bin"),
		configDir:   filepath.Join(root, "etc", "kamstack"),
		dataDir:     filepath.Join(root, "var", "lib", "kamstack"),
		unitDir:     filepath.Join(root, "etc", "systemd", "system"),
		stdout:      &bytes.Buffer{},
		stderr:      &bytes.Buffer{},
		interactive: false,
		hostname:    func() (string, error) { return "raspi", nil },
	}
	a.run = func(name string, args ...string) ([]byte, error) {
		return nil, nil
	}
	if err := os.MkdirAll(a.binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, binary := range []string{"kam", "kammon"} {
		if err := os.WriteFile(a.binary(binary), []byte("binary"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if err := runSetup(a, []string{"--yes"}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// KAM config points into the install and keeps the Pi serial default.
	var kam map[string]any
	raw, err := os.ReadFile(a.kamConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(raw, &kam); err != nil {
		t.Fatal(err)
	}
	if kam["node_name"] != "raspi" {
		t.Errorf("node_name = %v", kam["node_name"])
	}
	if kam["db_path"] != a.dbPath() {
		t.Errorf("db_path = %v, want %s", kam["db_path"], a.dbPath())
	}
	radio, _ := kam["radio"].(map[string]any)
	if radio["port"] != "/dev/ttyUSB0" {
		t.Errorf("radio.port = %v", radio["port"])
	}

	// KAMmon config has real passwords and the loopback KAM URL.
	raw, err = os.ReadFile(a.kammonConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "change-me") {
		t.Fatal("placeholder passwords survived setup")
	}
	var kammon map[string]any
	if err := yaml.Unmarshal(raw, &kammon); err != nil {
		t.Fatal(err)
	}
	if kammon["kam_monitor_url"] != kamAPIURL {
		t.Errorf("kam_monitor_url = %v", kammon["kam_monitor_url"])
	}
	users, _ := kammon["users"].([]any)
	passwords := map[string]string{}
	for _, entry := range users {
		user, _ := entry.(map[string]any)
		passwords[user["name"].(string)] = user["password"].(string)
	}
	for _, name := range []string{"admin", "viewer"} {
		if passwords[name] == "" || !strings.Contains(a.stdout.(*bytes.Buffer).String(), passwords[name]) {
			t.Errorf("%s password not printed once: %q", name, passwords[name])
		}
	}

	for _, unit := range []string{kamService + ".service", kammonService + ".service"} {
		path := filepath.Join(a.unitDir, unit)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("unit %s: %v", unit, err)
		}
		if !strings.Contains(string(content), a.binDir) || !strings.Contains(string(content), a.configDir) {
			t.Errorf("unit %s does not name the install paths:\n%s", unit, content)
		}
		if !strings.Contains(string(content), "WantedBy=multi-user.target") {
			t.Errorf("unit %s is not enabled at boot:\n%s", unit, content)
		}
		// Both services rewrite their YAML config; ProtectSystem=strict would block that.
		if !strings.Contains(string(content), "ReadWritePaths="+a.configDir) {
			t.Errorf("unit %s cannot write %s:\n%s", unit, a.configDir, content)
		}
	}
	if _, err := os.Stat(a.roomsDir()); err != nil {
		t.Errorf("rooms directory: %v", err)
	}

	// A second setup refuses to overwrite the configs.
	if err := runSetup(a, []string{"--yes"}); err == nil || !strings.Contains(err.Error(), "already configured") {
		t.Fatalf("second setup error = %v", err)
	}
}
