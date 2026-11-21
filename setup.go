package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// runSetup prepares a fresh install: layout and user, Pi-targeted configs with
// generated KAMmon passwords, unit files, then enable and start (DST-17).
func runSetup(a *app, args []string) error {
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	nodeName := flags.String("node-name", "", "node name (default: this host's name)")
	port := flags.String("port", "/dev/ttyUSB0", "serial port for the radio; change it later with kamstack usb")
	yes := flags.Bool("yes", false, "accept defaults without asking")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}

	for _, path := range []string{a.kamConfigPath(), a.kammonConfigPath()} {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("already configured: %s exists; edit it or move it away first", path)
		}
	}

	name := *nodeName
	if name == "" {
		host, err := a.hostname()
		if err != nil || strings.TrimSpace(host) == "" {
			host = "kam"
		}
		name = host
		if a.interactive && !*yes {
			if name, err = a.ask("Node name", host); err != nil {
				return err
			}
		}
	}

	adminPassword, err := randomPassword(16)
	if err != nil {
		return fmt.Errorf("generate the admin password: %w", err)
	}
	viewerPassword, err := randomPassword(16)
	if err != nil {
		return fmt.Errorf("generate the viewer password: %w", err)
	}

	if err := ensureLayout(a); err != nil {
		return err
	}

	kamConfig, err := renderKamConfig(name, a.dbPath(), *port)
	if err != nil {
		return err
	}
	if err := writeConfig(a.kamConfigPath(), kamConfig); err != nil {
		return fmt.Errorf("write %s: %w", a.kamConfigPath(), err)
	}
	kammonConfig, err := renderKammonConfig(adminPassword, viewerPassword, kamAPIURL)
	if err != nil {
		return err
	}
	if err := writeConfig(a.kammonConfigPath(), kammonConfig); err != nil {
		return fmt.Errorf("write %s: %w", a.kammonConfigPath(), err)
	}

	// Print the credentials before systemctl runs: if a unit fails to start,
	// they are still on screen and not lost with the config file.
	fmt.Fprintf(a.stdout, "Node name: %s\n", name)
	fmt.Fprintf(a.stdout, "KAM config: %s (serial %s)\n", a.kamConfigPath(), *port)
	fmt.Fprintf(a.stdout, "KAMmon config: %s (http://127.0.0.1:8080)\n", a.kammonConfigPath())
	fmt.Fprintf(a.stdout, "\nKAMmon accounts (write these down now; they are not stored anywhere else):\n")
	fmt.Fprintf(a.stdout, "  admin  %s\n", adminPassword)
	fmt.Fprintf(a.stdout, "  viewer %s\n", viewerPassword)

	if err := writeUnits(a); err != nil {
		return fmt.Errorf("write unit files: %w", err)
	}
	if _, err := a.run("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if _, err := a.run("systemctl", "enable", kamService, kammonService); err != nil {
		return err
	}

	missing, err := installLocalBinaries(a)
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		if _, err := a.run("systemctl", "start", kamService, kammonService); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(a.stderr,
			"warning: %s not found in %s or beside kamstack; the units are enabled but not started\n"+
				"         run `kamstack update` (or copy the binaries there) and then `kamstack start`\n",
			strings.Join(missing, " and "), a.binDir)
	}

	fmt.Fprintf(a.stdout, "\nNext: `kamstack usb` to pick the radio, then `kamstack doctor`.\n")
	return nil
}
