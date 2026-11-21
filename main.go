// Command kamstack sets up, controls and updates a KAM + KAMmon node on a
// Raspberry Pi. It is the only binary that knows both components as a pair.
//
// Usage:
//
//	kamstack setup [--node-name N] [--port P] [--yes]
//	kamstack usb
//	kamstack start | stop | restart | status
//	kamstack logs [-f] [-n N]
//	kamstack version
//	kamstack doctor
//	kamstack update [--check]
package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"runtime"
	"strings"
)

// version is the kamstack release, set by the build with
// -ldflags "-X main.version=<tag>". A plain local build reports "dev".
var version = "dev"

// commandOrder keeps the help text and the command list in a fixed order;
// map iteration order is random.
var commandOrder = []string{
	"setup", "usb", "start", "stop", "restart", "status", "logs", "version", "doctor", "update",
}

// command is one subcommand. A non-nil error is reported and turns into a
// non-zero exit code.
type command struct {
	usage string
	run   func(*app, []string) error
}

var commands = map[string]command{
	"setup":   {"setup [--node-name N] [--port P] [--yes]", runSetup},
	"usb":     {"usb", runUSB},
	"start":   {"start", serviceAction("start")},
	"stop":    {"stop", serviceAction("stop")},
	"restart": {"restart", serviceAction("restart")},
	"status":  {"status", runStatus},
	"logs":    {"logs [-f] [-n N]", runLogs},
	"version": {"version", runVersion},
	"doctor":  {"doctor", runDoctor},
	"update":  {"update [--check]", notImplemented("update")},
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run dispatches one subcommand and returns the process exit code: 0 on
// success, a command's own code when it exposes one, and 1 otherwise. Usage
// errors are 2.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	}
	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(stderr, "kamstack: unknown command %q\n\n", args[0])
		printUsage(stderr)
		return 2
	}
	if err := cmd.run(newApp(stdout, stderr), args[1:]); err != nil {
		fmt.Fprintln(stderr, "kamstack:", err)
		return exitCode(err)
	}
	return 0
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: kamstack <command> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "commands:")
	for _, name := range commandOrder {
		fmt.Fprintf(w, "  kamstack %s\n", commands[name].usage)
	}
}

// notImplemented is the placeholder for subcommands the plan still lists as
// open, so the scaffold builds and `kamstack help` already describes them.
func notImplemented(name string) func(*app, []string) error {
	return func(*app, []string) error {
		return fmt.Errorf("%s: not implemented yet", name)
	}
}

// runVersion prints kamstack's own version and the versions of the installed
// pair (DST-21).
func runVersion(a *app, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: kamstack version")
	}
	fmt.Fprintf(a.stdout, "kamstack %s %s/%s\n", version, runtime.GOOS, runtime.GOARCH)
	reportBinaryVersion(a, "kam", "version")
	reportBinaryVersion(a, "kammon", "-version")
	return nil
}

func reportBinaryVersion(a *app, name, versionArg string) {
	out, err := a.run(a.binary(name), versionArg)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(a.stdout, "%s: not installed in %s\n", name, a.binDir)
			return
		}
		fmt.Fprintf(a.stdout, "%s: %v\n", name, err)
		return
	}
	fmt.Fprintln(a.stdout, strings.TrimSpace(string(out)))
}
