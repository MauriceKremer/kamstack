package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListUSBReadsByIDPathsAndDrivers(t *testing.T) {
	root := t.TempDir()
	byID := filepath.Join(root, "dev", "serial", "by-id")
	if err := os.MkdirAll(byID, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tty := range []string{"ttyUSB0", "ttyUSB1"} {
		if err := os.WriteFile(filepath.Join(root, "dev", tty), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	links := map[string]string{
		"usb-a-radio-if00-port0":  "../../ttyUSB0",
		"usb-b-serial-if00-port0": "../../ttyUSB1",
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(byID, name)); err != nil {
			t.Fatal(err)
		}
	}
	sysfs := filepath.Join(root, "sys")
	for tty, driver := range map[string]string{"ttyUSB0": "cp210x", "ttyUSB1": "ftdi_sio"} {
		driverPath := filepath.Join(sysfs, "class", "tty", tty, "device", "driver")
		if err := os.MkdirAll(filepath.Dir(driverPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("/drivers/"+driver, driverPath); err != nil {
			t.Fatal(err)
		}
	}

	devices, err := listUSB(
		filepath.Join(byID, "*"),
		filepath.EvalSymlinks,
		func(tty string) string { return sysfsDriver(sysfs, tty) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 2 {
		t.Fatalf("devices = %+v", devices)
	}
	if devices[0].ByID != filepath.Join(byID, "usb-a-radio-if00-port0") ||
		devices[0].TTY != "ttyUSB0" ||
		devices[0].Driver != "cp210x" ||
		!devices[0].Candidate {
		t.Fatalf("first device = %+v", devices[0])
	}
	if devices[1].Driver != "ftdi_sio" || devices[1].Candidate {
		t.Fatalf("second device = %+v", devices[1])
	}
}

func TestSysfsDriverWithoutDriverIsUnknown(t *testing.T) {
	if got := sysfsDriver(t.TempDir(), "ttyUSB9"); got != "unknown" {
		t.Fatalf("driver = %q", got)
	}
}

func TestChooseUSBRefusesSilentlyInNonInteractiveMode(t *testing.T) {
	a := &app{stdout: &discard{}, stderr: &discard{}}
	_, err := chooseUSB(a, []usbDevice{{ByID: "/dev/a"}, {ByID: "/dev/b"}})
	if err == nil {
		t.Fatal("expected an error when two candidates need a choice")
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
