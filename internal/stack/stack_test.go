package stack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommittedStackIsValid(t *testing.T) {
	pin, err := Load(filepath.Join("..", "..", "stack.yml"))
	if err != nil {
		t.Fatalf("stack.yml: %v", err)
	}
	if pin.Version() != "0.7.0" {
		t.Errorf("version = %q", pin.Version())
	}
	if pin.ContractCommit == "" {
		t.Error("contract_commit is empty")
	}
}

func TestValidateRejectsMismatchedPair(t *testing.T) {
	err := (Stack{Kam: "v0.7.0", Kammon: "v0.7.1"}).Validate()
	if err == nil || !strings.Contains(err.Error(), "not the same version") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildManifestNeedsEveryTarget(t *testing.T) {
	pin := Stack{Kam: "v0.7.0", Kammon: "v0.7.0"}
	kam := fullRelease("kam", "v0.7.0")
	kammon := fullRelease("kammon", "v0.7.0")

	manifest, err := BuildManifest("0.7.0", pin, kam, kammon)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if manifest.Version != "0.7.0" || manifest.Kam.Version != "0.7.0" {
		t.Fatalf("manifest = %+v", manifest)
	}
	for _, target := range Targets {
		asset := AssetName("kam", "0.7.0", target)
		if manifest.Kam.Targets[target].URL == "" || manifest.Kam.Targets[target].SHA256 == "" {
			t.Errorf("%s missing from manifest: %+v", target, manifest.Kam.Targets)
		}
		if !strings.HasSuffix(manifest.Kam.Targets[target].URL, asset) {
			t.Errorf("%s url = %q", target, manifest.Kam.Targets[target].URL)
		}
	}
}

func TestBuildManifestFailsLoudlyOnMissingArchive(t *testing.T) {
	pin := Stack{Kam: "v0.7.0", Kammon: "v0.7.0"}
	kam := fullRelease("kam", "v0.7.0")
	delete(kam.AssetURLs, AssetName("kam", "0.7.0", "linux/armv7"))

	_, err := BuildManifest("0.7.0", pin, kam, fullRelease("kammon", "v0.7.0"))
	if err == nil || !strings.Contains(err.Error(), "kam_0.7.0_linux_armv7.tar.gz (asset)") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildManifestFailsLoudlyOnMissingChecksum(t *testing.T) {
	pin := Stack{Kam: "v0.7.0", Kammon: "v0.7.0"}
	kammon := fullRelease("kammon", "v0.7.0")
	delete(kammon.Checksums, AssetName("kammon", "0.7.0", "linux/arm64"))

	_, err := BuildManifest("0.7.0", pin, fullRelease("kam", "v0.7.0"), kammon)
	if err == nil || !strings.Contains(err.Error(), "SHA256SUMS") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildManifestRejectsWrongReleaseTag(t *testing.T) {
	pin := Stack{Kam: "v0.7.0", Kammon: "v0.7.0"}
	kam := fullRelease("kam", "v0.7.0")
	kam.Tag = "v0.6.0"

	_, err := BuildManifest("0.7.0", pin, kam, fullRelease("kammon", "v0.7.0"))
	if err == nil || !strings.Contains(err.Error(), "stack.yml pins v0.7.0") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseSHA256SUMS(t *testing.T) {
	raw := "abc123  kam_0.7.0_linux_amd64.tar.gz\ndef456 *kammon_0.7.0_linux_arm64.tar.gz\n\n"
	got := ParseSHA256SUMS([]byte(raw))
	if got["kam_0.7.0_linux_amd64.tar.gz"] != "abc123" {
		t.Errorf("plain line = %q", got["kam_0.7.0_linux_amd64.tar.gz"])
	}
	if got["kammon_0.7.0_linux_arm64.tar.gz"] != "def456" {
		t.Errorf("binary line = %q", got["kammon_0.7.0_linux_arm64.tar.gz"])
	}
}

func TestAssetName(t *testing.T) {
	if got := AssetName("kam", "0.7.0", "linux/armv7"); got != "kam_0.7.0_linux_armv7.tar.gz" {
		t.Fatalf("asset = %q", got)
	}
}

func fullRelease(component, tag string) Release {
	version := strings.TrimPrefix(tag, "v")
	release := Release{Tag: tag, AssetURLs: map[string]string{}, Checksums: map[string]string{}}
	for _, target := range Targets {
		asset := AssetName(component, version, target)
		release.AssetURLs[asset] = "https://example.test/" + asset
		release.Checksums[asset] = "checksum-" + target
	}
	return release
}

func TestLoadRejectsEmptyPin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stack.yml")
	if err := os.WriteFile(path, []byte("kam: v0.7.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error for a half-empty stack.yml")
	}
}
