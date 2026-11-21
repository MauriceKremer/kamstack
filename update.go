package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"kamstack.bytecoder.nl/internal/stack"
)

// releaseRepo is the kamstack repository whose latest manifest update reads.
// The release build overrides it with -X main.releaseRepo=<owner/repo>.
var releaseRepo = "MauriceKremer/kamstack"

const (
	updatePollInterval   = 500 * time.Millisecond
	defaultHealthTimeout = 10 * time.Second
)

func defaultManifestURL() string {
	return fmt.Sprintf("https://github.com/%s/releases/latest/download/manifest.json", releaseRepo)
}

// currentTarget is the manifest key for this build. The 32-bit ARM release is
// the armv7 archive.
func currentTarget() string {
	arch := runtime.GOARCH
	if arch == "arm" {
		arch = "armv7"
	}
	return runtime.GOOS + "/" + arch
}

func runUpdate(a *app, args []string) error {
	flags := flag.NewFlagSet("update", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	check := flags.Bool("check", false, "report installed and available versions without changing anything")
	manifestURL := flags.String("manifest-url", a.manifestURL, "manifest URL (default: latest kamstack release)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}

	manifest, err := fetchManifest(a, *manifestURL)
	if err != nil {
		return err
	}
	target := a.target()
	kamTarget, ok := manifest.Kam.Targets[target]
	if !ok {
		return fmt.Errorf("manifest %s has no kam build for %s", manifest.Version, target)
	}
	kammonTarget, ok := manifest.Kammon.Targets[target]
	if !ok {
		return fmt.Errorf("manifest %s has no kammon build for %s", manifest.Version, target)
	}

	installedKam := installedVersion(a, "kam", "version")
	installedKammon := installedVersion(a, "kammon", "-version")

	if *check {
		return reportUpdateCheck(a, manifest, installedKam, installedKammon)
	}
	if installedKam == manifest.Kam.Version && installedKammon == manifest.Kammon.Version {
		fmt.Fprintf(a.stdout, "Already current: kam %s, kammon %s\n", installedKam, installedKammon)
		return nil
	}
	return applyUpdate(a, manifest, kamTarget, kammonTarget)
}

func reportUpdateCheck(a *app, manifest stack.Manifest, installedKam, installedKammon string) error {
	outdated := false
	report := func(name, installed, available string) {
		if installed == "" {
			installed = "not installed"
		}
		if installed != available {
			outdated = true
		}
		fmt.Fprintf(a.stdout, "%-8s %-12s -> %s\n", name, installed, available)
	}
	report("kam", installedKam, manifest.Kam.Version)
	report("kammon", installedKammon, manifest.Kammon.Version)
	if outdated {
		return &exitStatus{code: 2, message: "update available"}
	}
	return nil
}

func applyUpdate(a *app, manifest stack.Manifest, kamTarget, kammonTarget stack.Target) error {
	stage, err := os.MkdirTemp("", "kamstack-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)

	kamArchive := filepath.Join(stage, "kam.tar.gz")
	if err := downloadArchive(a, "kam", kamTarget, kamArchive); err != nil {
		return err
	}
	kammonArchive := filepath.Join(stage, "kammon.tar.gz")
	if err := downloadArchive(a, "kammon", kammonTarget, kammonArchive); err != nil {
		return err
	}
	kamBinary := filepath.Join(stage, "kam")
	if err := extractBinary(kamArchive, "kam", kamBinary); err != nil {
		return err
	}
	kammonBinary := filepath.Join(stage, "kammon")
	if err := extractBinary(kammonArchive, "kammon", kammonBinary); err != nil {
		return err
	}

	// Nothing has changed on disk yet: the downloads are verified and the
	// binaries are unpacked before the services are touched.
	if _, err := a.run("systemctl", "stop", kamService, kammonService); err != nil {
		return fmt.Errorf("stop services: %w", err)
	}

	backup, err := createBackup(a, manifest.Version)
	if err != nil {
		_, _ = a.run("systemctl", "start", kamService, kammonService)
		return fmt.Errorf("backup failed; services restarted: %w", err)
	}

	for _, swap := range []struct {
		name string
		src  string
	}{
		{"kam", kamBinary},
		{"kammon", kammonBinary},
	} {
		if err := installBinary(swap.src, a.binary(swap.name)); err != nil {
			return rollbackUpdate(a, backup, fmt.Errorf("install %s: %w", swap.name, err))
		}
	}

	if _, err := a.run("systemctl", "start", kamService, kammonService); err != nil {
		return rollbackUpdate(a, backup, fmt.Errorf("start services: %w", err))
	}
	if err := waitHealthy(a); err != nil {
		return rollbackUpdate(a, backup, err)
	}

	fmt.Fprintf(a.stdout, "Updated to kam %s and kammon %s\n", manifest.Kam.Version, manifest.Kammon.Version)
	if backup != "" {
		fmt.Fprintf(a.stdout, "Backup: %s\n", backup)
	}
	fmt.Fprintf(a.stdout, "Previous binaries kept as %s and %s\n",
		a.binary("kam")+".prev", a.binary("kammon")+".prev")
	return nil
}

// rollbackUpdate restores the .prev binaries, starts both units again and
// leaves the backup in place (DST-29).
func rollbackUpdate(a *app, backup string, cause error) error {
	restoreBinary(a.binary("kam"))
	restoreBinary(a.binary("kammon"))
	if _, err := a.run("systemctl", "start", kamService, kammonService); err != nil {
		return fmt.Errorf("%v; rollback could not start the services: %v (backup %s)", cause, err, backup)
	}
	return fmt.Errorf("%v; rolled back to the previous binaries (backup %s)", cause, backup)
}

func fetchManifest(a *app, url string) (stack.Manifest, error) {
	raw, err := a.fetch(url)
	if err != nil {
		return stack.Manifest{}, fmt.Errorf("read manifest: %w", err)
	}
	manifest, err := stack.ParseManifest(raw)
	if err != nil {
		return stack.Manifest{}, fmt.Errorf("read manifest: %w", err)
	}
	return manifest, nil
}

// installedVersion parses `kam version` / `kammon -version`. An absent binary
// reports an empty version so `--check` can say "not installed".
func installedVersion(a *app, name, versionArg string) string {
	out, err := a.run(a.binary(name), versionArg)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ""
		}
		return ""
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return ""
	}
	return fields[1]
}

// downloadArchive fetches one component archive and verifies its checksum
// before anything is stopped.
func downloadArchive(a *app, component string, target stack.Target, dest string) error {
	raw, err := a.fetch(target.URL)
	if err != nil {
		return fmt.Errorf("download %s: %w", component, err)
	}
	sum := sha256.Sum256(raw)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, target.SHA256) {
		return fmt.Errorf("%s: checksum mismatch: got %s, want %s", component, got, target.SHA256)
	}
	if err := os.WriteFile(dest, raw, 0o600); err != nil {
		return err
	}
	return nil
}

// extractBinary pulls the named binary out of a release archive.
func extractBinary(archivePath, name, dest string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != name {
			continue
		}
		tmp := dest + ".new"
		out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, reader); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		return os.Rename(tmp, dest)
	}
	return fmt.Errorf("%s: %s not found in the archive", archivePath, name)
}

// installBinary keeps the previous binary as .prev and swaps the new one in.
func installBinary(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(dest); err == nil {
		if err := copyFile(dest, dest+".prev", 0o755); err != nil {
			return err
		}
	}
	tmp := dest + ".new"
	if err := copyFile(src, tmp, 0o755); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

func restoreBinary(dest string) {
	previous := dest + ".prev"
	if _, err := os.Stat(previous); err != nil {
		return
	}
	_ = os.Rename(previous, dest)
}

// waitHealthy waits up to healthTimeout for KAM's /v1/status and KAMmon's /
// (DST-29).
func waitHealthy(a *app) error {
	kamURL := kamAPIBase(a.kamConfigPath()) + "/v1/status"
	kammonURL := kammonWebBase(a.kammonConfigPath()) + "/"
	attempts := 1
	if a.healthTimeout > 0 {
		attempts = int(a.healthTimeout / updatePollInterval)
		if attempts < 1 {
			attempts = 1
		}
	}
	var last error
	for attempt := 0; attempt < attempts; attempt++ {
		kamCode, kamErr := a.httpGet(kamURL, 2*time.Second)
		if kamErr != nil {
			last = fmt.Errorf("KAM API %s: %w", kamURL, kamErr)
		} else if kamCode != 200 {
			last = fmt.Errorf("KAM API %s: %d", kamURL, kamCode)
		} else {
			kammonCode, kammonErr := a.httpGet(kammonURL, 2*time.Second)
			switch {
			case kammonErr != nil:
				last = fmt.Errorf("KAMmon %s: %w", kammonURL, kammonErr)
			case kammonCode != 200 && kammonCode != 303:
				last = fmt.Errorf("KAMmon %s: %d", kammonURL, kammonCode)
			default:
				return nil
			}
		}
		if attempt < attempts-1 {
			a.sleep(updatePollInterval)
		}
	}
	return fmt.Errorf("health check failed: %w", last)
}
