package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ensureLayout creates the directories and the service user a packaged install
// needs (DST-16). It is idempotent: setup may be re-run after a failed start.
func ensureLayout(a *app) error {
	for _, dir := range []string{a.binDir, a.configDir, a.dataDir, a.roomsDir(), a.unitDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	if _, err := a.run("id", "-u", serviceUser); err != nil {
		if _, err := a.run("useradd",
			"--system", "--no-create-home",
			"--home-dir", a.dataDir,
			"--shell", "/usr/sbin/nologin",
			serviceUser,
		); err != nil {
			return fmt.Errorf("create the %s user: %w", serviceUser, err)
		}
	}
	// Serial access needs the distro's dialout group; not every system has one.
	if _, err := a.run("getent", "group", "dialout"); err == nil {
		if _, err := a.run("usermod", "-aG", "dialout", serviceUser); err != nil {
			return fmt.Errorf("add %s to dialout: %w", serviceUser, err)
		}
	}

	if err := os.Chmod(a.binDir, 0o755); err != nil {
		return err
	}
	if err := os.Chmod(a.unitDir, 0o755); err != nil {
		return err
	}
	// KAM rewrites kam.yml at runtime, so the config directory belongs to the
	// service group and the files are 0660 (see the plan's ReadWritePaths note).
	if _, err := a.run("chgrp", serviceUser, a.configDir); err != nil {
		return fmt.Errorf("give %s ownership of %s: %w", serviceUser, a.configDir, err)
	}
	if err := os.Chmod(a.configDir, 0o750); err != nil {
		return err
	}
	if _, err := a.run("chown", serviceUser+":"+serviceUser, a.dataDir); err != nil {
		return fmt.Errorf("give %s ownership of %s: %w", serviceUser, a.dataDir, err)
	}
	if err := os.Chmod(a.dataDir, 0o750); err != nil {
		return err
	}
	if err := os.Chmod(a.roomsDir(), 0o750); err != nil {
		return err
	}
	return nil
}

// installLocalBinaries copies kam and kammon from beside the running kamstack
// binary into the install directory. It returns the names that are still
// missing, so setup can say plainly that the pair has not arrived yet.
func installLocalBinaries(a *app) ([]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	sourceDir := filepath.Dir(exe)
	var missing []string
	for _, name := range []string{"kam", "kammon"} {
		dest := a.binary(name)
		if _, err := os.Stat(dest); err == nil {
			continue
		}
		source := filepath.Join(sourceDir, name)
		if _, err := os.Stat(source); err != nil {
			missing = append(missing, name)
			continue
		}
		if err := copyFile(source, dest, 0o755); err != nil {
			return nil, fmt.Errorf("install %s: %w", name, err)
		}
	}
	return missing, nil
}

func copyFile(source, dest string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dest, mode)
}
