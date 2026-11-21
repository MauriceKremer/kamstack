package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The paths and service names a packaged install uses. They are fields on app
// rather than bare constants so tests can point them at a temporary tree
// (PLAN.md DST-16).
const (
	defaultBinDir    = "/opt/kamstack/bin"
	defaultConfigDir = "/etc/kamstack"
	defaultDataDir   = "/var/lib/kamstack"
	defaultUnitDir   = "/etc/systemd/system"

	serviceUser   = "kamstack"
	kamService    = "kamstack-kam"
	kammonService = "kamstack-kammon"

	// KAM's loopback API and KAMmon's web listener, as setup writes them.
	kamAPIURL    = "http://127.0.0.1:8766"
	kammonWebURL = "http://127.0.0.1:8080"
)

// app carries the paths and the side effects a command uses. Tests build one
// with temporary paths and fake run/httpGet functions; nothing then touches a
// real Pi, systemd or the network.
type app struct {
	binDir    string
	configDir string
	dataDir   string
	unitDir   string

	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	// interactive is true when stdin is a terminal, so setup may ask questions.
	interactive bool

	// run executes a command and returns its combined output. A non-nil error
	// keeps the original *exec.ExitError, so main can pass its exit code on.
	run func(name string, args ...string) ([]byte, error)
	// runInteractive wires the command straight to our stdin/stdout/stderr,
	// for journalctl -f and human-facing systemctl output.
	runInteractive func(name string, args ...string) error
	// httpGet returns the status code of GET url.
	httpGet func(url string, timeout time.Duration) (int, error)
	// httpBody is httpGet plus the body, for the modem link.
	httpBody func(url string, timeout time.Duration) (int, []byte, error)
	// hostname supplies the default node name.
	hostname func() (string, error)

	// serialGlob and sysfsRoot are injectable for the USB table (DST-18).
	serialGlob string
	sysfsRoot  string

	input *bufio.Reader
}

func newApp(stdout, stderr io.Writer) *app {
	a := &app{
		binDir:      defaultBinDir,
		configDir:   defaultConfigDir,
		dataDir:     defaultDataDir,
		unitDir:     defaultUnitDir,
		stdin:       os.Stdin,
		stdout:      stdout,
		stderr:      stderr,
		interactive: stdinIsTerminal(),
		run:         execOutput,
		httpGet:     httpStatus,
		httpBody:    httpFetch,
		hostname:    os.Hostname,
		serialGlob:  "/dev/serial/by-id/*",
		sysfsRoot:   "/sys",
	}
	a.runInteractive = a.execInteractive
	return a
}

func (a *app) kamConfigPath() string    { return filepath.Join(a.configDir, "kam.yml") }
func (a *app) kammonConfigPath() string { return filepath.Join(a.configDir, "kammon.yml") }
func (a *app) dbPath() string           { return filepath.Join(a.dataDir, "kam.db") }
func (a *app) roomsDir() string         { return filepath.Join(a.dataDir, "rooms") }
func (a *app) binary(name string) string {
	return filepath.Join(a.binDir, name)
}

func (a *app) reader() *bufio.Reader {
	if a.input == nil {
		a.input = bufio.NewReader(a.stdin)
	}
	return a.input
}

// ask reads one answer, using def when the line is empty.
func (a *app) ask(question, def string) (string, error) {
	fmt.Fprintf(a.stdout, "%s [%s]: ", question, def)
	line, err := a.reader().ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	answer := strings.TrimSpace(line)
	if answer == "" {
		return def, nil
	}
	return answer, nil
}

// confirm asks a yes/no question, defaulting to yes.
func (a *app) confirm(question string) (bool, error) {
	answer, err := a.ask(question+" [Y/n]", "y")
	if err != nil {
		return false, err
	}
	switch strings.ToLower(answer) {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return false, fmt.Errorf("expected y or n, got %q", answer)
	}
}

func execOutput(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return out, nil
}

func (a *app) execInteractive(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = a.stdin, a.stdout, a.stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func httpStatus(url string, timeout time.Duration) (int, error) {
	code, _, err := httpFetch(url, timeout)
	return code, err
}

func httpFetch(url string, timeout time.Duration) (int, []byte, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// exitCode turns a command error into the process exit code, keeping the
// code a wrapped *exec.ExitError carries (DST-19).
func exitCode(err error) int {
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() > 0 {
		return exit.ExitCode()
	}
	return 1
}
