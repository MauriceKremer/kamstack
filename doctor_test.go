package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRenderDoctorShowsEverySection(t *testing.T) {
	report := doctorReport{
		Version:   "0.7.0",
		OS:        "linux",
		Arch:      "arm64",
		ConfigDir: "/etc/kamstack",
		DataDir:   "/var/lib/kamstack",
		Configs: []fileStatus{
			{Path: "/etc/kamstack/kam.yml", State: "ok (10 bytes, -rw-rw----)"},
			{Path: "/etc/kamstack/kammon.yml", State: "ok (10 bytes, -rw-rw----)"},
		},
		Serial: fileStatus{Path: "/dev/serial/by-id/usb-radio", State: "readable"},
		Units: []unitStatus{
			{Name: "kamstack-kam", State: "active"},
			{Name: "kamstack-kammon", State: "active"},
		},
		Endpoints: []endpointStatus{
			{Label: "KAM API", URL: "http://127.0.0.1:8766/v1/status", Code: 200},
			{Label: "KAMmon", URL: "http://127.0.0.1:8080/", Code: 303},
		},
		Disk:    diskStatus{Path: "/var/lib/kamstack", Free: 2 * 1024 * 1024 * 1024},
		Journal: []string{"line one", "line two"},
		Warnings: []string{
			"database from an older version; restore the backup or reset the data",
		},
	}
	var out bytes.Buffer
	renderDoctor(&out, report)
	for _, want := range []string{
		"kamstack doctor",
		"kamstack 0.7.0 linux/arm64",
		"/etc/kamstack/kam.yml: ok",
		"/dev/serial/by-id/usb-radio: readable",
		"kamstack-kam: active",
		"KAM API: 200 http://127.0.0.1:8766/v1/status",
		"KAMmon: 303 http://127.0.0.1:8080/",
		"2.0 GiB free",
		"line two",
		"database from an older version",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("doctor output missing %q:\n%s", want, out.String())
		}
	}
}

func TestUnitConfigWriteAccess(t *testing.T) {
	a := &app{run: func(name string, args ...string) ([]byte, error) {
		return []byte("ProtectSystem=strict\nReadWritePaths=/var/lib/kamstack /etc/kamstack\n"), nil
	}}
	if w := unitConfigWriteAccess(a, "kamstack-kammon", "/etc/kamstack"); w != "" {
		t.Errorf("writable config dir should not warn, got %q", w)
	}

	a = &app{run: func(name string, args ...string) ([]byte, error) {
		return []byte("ProtectSystem=strict\nReadWritePaths=\n"), nil
	}}
	if w := unitConfigWriteAccess(a, "kamstack-kammon", "/etc/kamstack"); !strings.Contains(w, "is not in ReadWritePaths") {
		t.Errorf("read-only config dir should warn, got %q", w)
	}

	a = &app{run: func(name string, args ...string) ([]byte, error) {
		return []byte("ProtectSystem=no\nReadWritePaths=\n"), nil
	}}
	if w := unitConfigWriteAccess(a, "kamstack-kammon", "/etc/kamstack"); w != "" {
		t.Errorf("non-strict unit should not warn, got %q", w)
	}
}

func TestCollectDoctorReadsConfigSerialAndProbes(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "etc")
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tty := filepath.Join(root, "ttyUSB0")
	if err := os.WriteFile(tty, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	kamConfig, err := renderKamConfig("pi", filepath.Join(dataDir, "kam.db"), tty)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeConfig(filepath.Join(configDir, "kam.yml"), kamConfig); err != nil {
		t.Fatal(err)
	}
	kammonConfig, err := renderKammonConfig("admin", "viewer", kamAPIURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeConfig(filepath.Join(configDir, "kammon.yml"), kammonConfig); err != nil {
		t.Fatal(err)
	}

	a := &app{
		configDir: configDir,
		dataDir:   dataDir,
		run: func(name string, args ...string) ([]byte, error) {
			if name == "journalctl" {
				return []byte("line one\nno such column: new_field\n"), nil
			}
			if name == "systemctl" && len(args) > 0 && args[0] == "is-active" {
				if args[len(args)-1] == kamService {
					return []byte("active\n"), nil
				}
				return []byte("inactive\n"), nil
			}
			if name == "systemctl" && len(args) > 1 && args[0] == "show" {
				if args[1] == kamService {
					return []byte("ProtectSystem=strict\nReadWritePaths=" + configDir + "\n"), nil
				}
				return []byte("ProtectSystem=strict\nReadWritePaths=\n"), nil
			}
			return nil, nil
		},
		httpGet: func(url string, _ time.Duration) (int, error) {
			if strings.Contains(url, "/v1/status") {
				return 200, nil
			}
			return 303, nil
		},
	}

	report := collectDoctor(a)
	if report.Serial.State != "readable" {
		t.Errorf("serial = %+v", report.Serial)
	}
	if len(report.Configs) != 2 || report.Configs[0].State == "missing" {
		t.Errorf("configs = %+v", report.Configs)
	}
	if report.Units[0].State != "active" || report.Units[1].State != "inactive" {
		t.Errorf("units = %+v", report.Units)
	}
	if report.Endpoints[0].Code != 200 || report.Endpoints[1].Code != 303 {
		t.Errorf("endpoints = %+v", report.Endpoints)
	}
	if report.Disk.Error != "" {
		t.Errorf("disk = %+v", report.Disk)
	}
	if len(report.Journal) != 2 {
		t.Errorf("journal = %+v", report.Journal)
	}
	joined := strings.Join(report.Warnings, "\n")
	if !strings.Contains(joined, "older version") {
		t.Errorf("warnings missing schema note: %+v", report.Warnings)
	}
	if !strings.Contains(joined, kammonService+": ProtectSystem=strict") || !strings.Contains(joined, configDir+" is not in ReadWritePaths") {
		t.Errorf("warnings missing write-access note: %+v", report.Warnings)
	}
}
