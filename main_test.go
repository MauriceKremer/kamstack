package main

import (
	"strings"
	"testing"
)

func TestVersionIsNonEmpty(t *testing.T) {
	if version == "" {
		t.Fatal("version must not be empty; a local build should report dev")
	}
}

func TestRunVersion(t *testing.T) {
	var out, errOut strings.Builder
	if code := run([]string{"version"}, &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "kamstack "+version) {
		t.Fatalf("version output = %q, want it to name %q", out.String(), version)
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	var out, errOut strings.Builder
	if code := run([]string{"frobnicate"}, &out, &errOut); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "unknown command") {
		t.Fatalf("stderr = %q, want an unknown-command message", errOut.String())
	}
}

func TestHelpListsEveryCommand(t *testing.T) {
	var out, errOut strings.Builder
	if code := run([]string{"help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	for _, name := range commandOrder {
		if !strings.Contains(out.String(), "kamstack "+name) {
			t.Fatalf("help output missing %q:\n%s", name, out.String())
		}
	}
}
