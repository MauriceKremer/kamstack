package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	"time"
)

// serviceAction wraps systemctl for start, stop and restart. The exit code of
// systemctl becomes ours (DST-19).
func serviceAction(action string) func(*app, []string) error {
	return func(a *app, args []string) error {
		flags := flag.NewFlagSet(action, flag.ContinueOnError)
		flags.SetOutput(a.stderr)
		if err := flags.Parse(args); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("unexpected positional arguments: %v", flags.Args())
		}
		return a.runInteractive("systemctl", action, kamService, kammonService)
	}
}

// runStatus prints the two unit states, both HTTP endpoints and the modem link
// on one screen (DST-19).
func runStatus(a *app, args []string) error {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}

	fmt.Fprintf(a.stdout, "%-8s %s\n", "kam:", unitState(a, kamService))
	fmt.Fprintf(a.stdout, "%-8s %s\n", "kammon:", unitState(a, kammonService))

	api := kamAPIBase(a.kamConfigPath())
	checkHTTP(a, "KAM API", api+"/v1/status")
	checkHTTP(a, "KAMmon", kammonWebBase(a.kammonConfigPath())+"/")
	fmt.Fprintf(a.stdout, "%-8s %s\n", "modem:", modemLink(a, api))
	return nil
}

func unitState(a *app, unit string) string {
	out, _ := a.run("systemctl", "is-active", unit)
	state := strings.TrimSpace(string(out))
	if state == "" {
		return "unknown"
	}
	return state
}

func checkHTTP(a *app, label, url string) {
	code, err := a.httpGet(url, 3*time.Second)
	if err != nil {
		fmt.Fprintf(a.stdout, "%-8s %s: unreachable (%v)\n", label+":", url, err)
		return
	}
	suffix := ""
	if code != 200 && code != 303 {
		suffix = " (unexpected)"
	}
	fmt.Fprintf(a.stdout, "%-8s %s: %d%s\n", label+":", url, code, suffix)
}

func modemLink(a *app, api string) string {
	_, body, err := a.httpBody(api+"/v1/modem", 3*time.Second)
	if err != nil {
		return "unknown (" + err.Error() + ")"
	}
	var payload struct {
		Link string `json:"link"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Link == "" {
		return "unknown"
	}
	return payload.Link
}
