package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// radioDrivers are the USB-serial drivers a MeshCore radio realistically uses.
var radioDrivers = map[string]bool{
	"cp210x":  true,
	"ch341":   true,
	"cdc_acm": true,
}

// usbDevice is one row of the USB table (DST-18).
type usbDevice struct {
	ByID      string
	TTY       string
	Driver    string
	Candidate bool
}

// listUSB reads the by-id links and the driver behind each. glob, eval and
// driver are parameters so tests can use a fixture tree instead of /dev and
// /sys.
func listUSB(glob string, eval func(string) (string, error), driver func(string) string) ([]usbDevice, error) {
	paths, err := filepath.Glob(glob)
	if err != nil {
		return nil, err
	}
	var devices []usbDevice
	for _, path := range paths {
		target, err := eval(path)
		if err != nil {
			continue
		}
		tty := filepath.Base(target)
		name := driver(tty)
		devices = append(devices, usbDevice{
			ByID:      path,
			TTY:       tty,
			Driver:    name,
			Candidate: radioDrivers[name],
		})
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].ByID < devices[j].ByID })
	return devices, nil
}

// sysfsDriver reads the driver symlink for a tty device.
func sysfsDriver(sysfsRoot, tty string) string {
	target, err := os.Readlink(filepath.Join(sysfsRoot, "class", "tty", tty, "device", "driver"))
	if err != nil {
		return "unknown"
	}
	return filepath.Base(target)
}

func runUSB(a *app, args []string) error {
	flags := flag.NewFlagSet("usb", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}

	if _, err := os.Stat(a.kamConfigPath()); err != nil {
		return fmt.Errorf("%s not found; run `kamstack setup` first", a.kamConfigPath())
	}

	devices, err := listUSB(a.serialGlob, filepath.EvalSymlinks, func(tty string) string {
		return sysfsDriver(a.sysfsRoot, tty)
	})
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		return fmt.Errorf("no devices under %s; is the radio plugged in?", filepath.Dir(a.serialGlob))
	}

	fmt.Fprintln(a.stdout, "DEVICE\tTTY\tDRIVER\tRADIO")
	var candidates []usbDevice
	for _, device := range devices {
		mark := "-"
		if device.Candidate {
			mark = "yes"
			candidates = append(candidates, device)
		}
		fmt.Fprintf(a.stdout, "%s\t%s\t%s\t%s\n", device.ByID, device.TTY, device.Driver, mark)
	}
	switch len(candidates) {
	case 0:
		return fmt.Errorf("no radio candidate found (looked for cp210x, ch341 or cdc_acm)")
	case 1:
	default:
		choice, err := chooseUSB(a, candidates)
		if err != nil {
			return err
		}
		candidates = []usbDevice{choice}
	}

	selected := candidates[0]
	if err := updateKamPort(a.kamConfigPath(), selected.ByID); err != nil {
		return fmt.Errorf("write %s: %w", a.kamConfigPath(), err)
	}
	fmt.Fprintf(a.stdout, "\nUsing %s (%s) in %s\n", selected.ByID, selected.Driver, a.kamConfigPath())

	if !unitActive(a, kamService) {
		fmt.Fprintln(a.stdout, "kam is not running; start it with `kamstack start`")
		return nil
	}
	if !a.interactive {
		fmt.Fprintln(a.stdout, "Restart kam to apply: systemctl restart "+kamService)
		return nil
	}
	restart, err := a.confirm("Restart kam now?")
	if err != nil {
		return err
	}
	if restart {
		_, err := a.run("systemctl", "restart", kamService)
		return err
	}
	return nil
}

// chooseUSB asks which candidate to use. The by-id path is written, never the
// ttyUSB name: the kernel renumbers those.
func chooseUSB(a *app, candidates []usbDevice) (usbDevice, error) {
	if !a.interactive {
		var paths []string
		for _, candidate := range candidates {
			paths = append(paths, candidate.ByID)
		}
		return usbDevice{}, fmt.Errorf("more than one radio candidate; pass a single one by unplugging the others or choose one of: %s", strings.Join(paths, ", "))
	}
	fmt.Fprintf(a.stdout, "\nMore than one radio candidate; pick one:\n")
	for i, candidate := range candidates {
		fmt.Fprintf(a.stdout, "  %d) %s (%s)\n", i+1, candidate.ByID, candidate.Driver)
	}
	answer, err := a.ask("Radio", "1")
	if err != nil {
		return usbDevice{}, err
	}
	index, err := strconv.Atoi(answer)
	if err != nil || index < 1 || index > len(candidates) {
		return usbDevice{}, fmt.Errorf("expected a number from 1 to %d, got %q", len(candidates), answer)
	}
	return candidates[index-1], nil
}

func unitActive(a *app, unit string) bool {
	out, err := a.run("systemctl", "is-active", unit)
	return err == nil && strings.TrimSpace(string(out)) == "active"
}
