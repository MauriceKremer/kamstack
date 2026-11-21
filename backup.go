package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// backupKeep is how many update backups stay on disk (DST-28).
const backupKeep = 3

// createBackup archives the database files and rooms/ while the services are
// stopped. It copies files, so it needs no SQLite dependency. Returns "" when
// there is no state yet.
func createBackup(a *app, version string) (string, error) {
	dir := filepath.Join(a.dataDir, "backups")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}

	var paths []string
	for _, name := range []string{"kam.db", "kam.db-wal", "kam.db-shm"} {
		path := filepath.Join(a.dataDir, name)
		if _, err := os.Stat(path); err == nil {
			paths = append(paths, path)
		}
	}
	if _, err := os.Stat(a.roomsDir()); err == nil {
		paths = append(paths, a.roomsDir())
	}
	if len(paths) == 0 {
		return "", nil
	}

	name := fmt.Sprintf("%s-%s.tar.gz", time.Now().Format("2006-01-02-1504"), version)
	dest := filepath.Join(dir, name)
	if err := writeTarGz(dest, a.dataDir, paths); err != nil {
		return "", err
	}
	if err := pruneBackups(dir, backupKeep); err != nil {
		return "", err
	}
	return dest, nil
}

func writeTarGz(dest, base string, paths []string) error {
	file, err := os.Create(dest)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)

	for _, path := range paths {
		err := filepath.Walk(path, func(current string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(base, current)
			if err != nil {
				return err
			}
			if info.IsDir() {
				header, err := tar.FileInfoHeader(info, "")
				if err != nil {
					return err
				}
				header.Name = filepath.ToSlash(rel) + "/"
				return tw.WriteHeader(header)
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			header.Name = filepath.ToSlash(rel)
			if err := tw.WriteHeader(header); err != nil {
				return err
			}
			in, err := os.Open(current)
			if err != nil {
				return err
			}
			defer in.Close()
			_, err = io.Copy(tw, in)
			return err
		})
		if err != nil {
			tw.Close()
			gz.Close()
			file.Close()
			return err
		}
	}
	if err := tw.Close(); err != nil {
		gz.Close()
		file.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

// pruneBackups keeps the newest keep archives. The timestamped names sort
// chronologically.
func pruneBackups(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".tar.gz") {
			names = append(names, entry.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for i := keep; i < len(names); i++ {
		if err := os.Remove(filepath.Join(dir, names[i])); err != nil {
			return err
		}
	}
	return nil
}
