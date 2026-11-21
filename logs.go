package main

import (
	"flag"
	"fmt"
	"strconv"
)

// runLogs reads both units' journal. kamstack stores nothing and rotates
// nothing: journalctl owns the logs (DST-20).
func runLogs(a *app, args []string) error {
	flags := flag.NewFlagSet("logs", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	follow := flags.Bool("f", false, "follow new lines")
	lines := flags.Int("n", 100, "number of lines to show")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}
	if *lines < 0 {
		return fmt.Errorf("-n must not be negative")
	}

	journalArgs := []string{"--no-pager", "-u", kamService, "-u", kammonService, "-n", strconv.Itoa(*lines)}
	if *follow {
		journalArgs = append(journalArgs, "-f")
	}
	return a.runInteractive("journalctl", journalArgs...)
}
