package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// doctorReport is everything the bug-report screen shows (DST-22). collectDoctor
// fills it from the machine; renderDoctor only formats it, so tests can feed a
// fixture without a Pi, a radio or network access.
type doctorReport struct {
	Version   string
	OS        string
	Arch      string
	ConfigDir string
	DataDir   string

	Configs   []fileStatus
	Serial    fileStatus
	Units     []unitStatus
	Endpoints []endpointStatus
	Disk      diskStatus
	Journal   []string
}

type fileStatus struct {
	Path  string
	State string
}

type unitStatus struct {
	Name  string
	State string
}

type endpointStatus struct {
	Label string
	URL   string
	Code  int
	Err   string
}

type diskStatus struct {
	Path  string
	Free  uint64
	Error string
}

func runDoctor(a *app, args []string) error {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}
	renderDoctor(a.stdout, collectDoctor(a))
	return nil
}

func collectDoctor(a *app) doctorReport {
	report := doctorReport{
		Version:   version,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		ConfigDir: a.configDir,
		DataDir:   a.dataDir,
	}

	for _, path := range []string{a.kamConfigPath(), a.kammonConfigPath()} {
		state := "missing"
		if info, err := os.Stat(path); err == nil {
			state = fmt.Sprintf("ok (%d bytes, %s)", info.Size(), info.Mode().Perm())
		}
		report.Configs = append(report.Configs, fileStatus{Path: path, State: state})
	}

	port := kamRadioPort(a.kamConfigPath())
	switch {
	case port == "":
		report.Serial = fileStatus{State: "not set in kam.yml"}
	default:
		state := "readable"
		if _, err := os.Stat(port); err != nil {
			state = "missing: " + err.Error()
		} else if file, err := os.Open(port); err != nil {
			state = "not readable: " + err.Error()
		} else {
			file.Close()
		}
		report.Serial = fileStatus{Path: port, State: state}
	}

	for _, unit := range []string{kamService, kammonService} {
		report.Units = append(report.Units, unitStatus{Name: unit, State: unitState(a, unit)})
	}

	api := kamAPIBase(a.kamConfigPath())
	endpoints := []struct {
		label string
		url   string
	}{
		{"KAM API", api + "/v1/status"},
		{"KAMmon", kammonWebBase(a.kammonConfigPath()) + "/"},
	}
	for _, endpoint := range endpoints {
		code, err := a.httpGet(endpoint.url, 3*time.Second)
		status := endpointStatus{Label: endpoint.label, URL: endpoint.url, Code: code}
		if err != nil {
			status.Err = err.Error()
		}
		report.Endpoints = append(report.Endpoints, status)
	}

	if free, err := diskFree(a.dataDir); err != nil {
		report.Disk = diskStatus{Path: a.dataDir, Error: err.Error()}
	} else {
		report.Disk = diskStatus{Path: a.dataDir, Free: free}
	}

	if out, err := a.run("journalctl", "--no-pager", "-u", kamService, "-u", kammonService, "-n", "20"); err != nil {
		report.Journal = []string{"journalctl: " + err.Error()}
	} else {
		report.Journal = splitLines(string(out))
	}
	return report
}

func renderDoctor(w io.Writer, report doctorReport) {
	fmt.Fprintf(w, "kamstack doctor\n")
	fmt.Fprintf(w, "version: kamstack %s %s/%s\n", report.Version, report.OS, report.Arch)
	fmt.Fprintf(w, "paths: %s, %s\n", report.ConfigDir, report.DataDir)

	fmt.Fprintf(w, "\nConfig\n")
	for _, config := range report.Configs {
		fmt.Fprintf(w, "  %s: %s\n", config.Path, config.State)
	}

	fmt.Fprintf(w, "\nSerial\n")
	if report.Serial.Path == "" {
		fmt.Fprintf(w, "  %s\n", report.Serial.State)
	} else {
		fmt.Fprintf(w, "  %s: %s\n", report.Serial.Path, report.Serial.State)
	}

	fmt.Fprintf(w, "\nUnits\n")
	for _, unit := range report.Units {
		fmt.Fprintf(w, "  %s: %s\n", unit.Name, unit.State)
	}

	fmt.Fprintf(w, "\nHTTP\n")
	for _, endpoint := range report.Endpoints {
		if endpoint.Err != "" {
			fmt.Fprintf(w, "  %s: unreachable (%s) %s\n", endpoint.Label, endpoint.Err, endpoint.URL)
			continue
		}
		fmt.Fprintf(w, "  %s: %d %s\n", endpoint.Label, endpoint.Code, endpoint.URL)
	}

	fmt.Fprintf(w, "\nDisk\n")
	if report.Disk.Error != "" {
		fmt.Fprintf(w, "  %s: %s\n", report.Disk.Path, report.Disk.Error)
	} else {
		fmt.Fprintf(w, "  %s: %s free\n", report.Disk.Path, humanBytes(report.Disk.Free))
	}

	fmt.Fprintf(w, "\nJournal (last 20 lines)\n")
	if len(report.Journal) == 0 {
		fmt.Fprintln(w, "  (empty)")
	}
	for _, line := range report.Journal {
		fmt.Fprintf(w, "  %s\n", line)
	}
}

func splitLines(text string) []string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func humanBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func diskFree(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return uint64(stat.Bavail) * uint64(stat.Bsize), nil
}
