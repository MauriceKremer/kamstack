// Package stack reads the pinned pair in stack.yml and turns verified release
// assets into the manifest kamstack installs from (PLAN.md DST-24, DST-25).
package stack

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Targets are the platforms every component release ships.
var Targets = []string{"linux/arm64", "linux/armv7", "linux/amd64"}

// Stack is the matched pair pinned by this kamstack release.
type Stack struct {
	Kam            string `yaml:"kam"`
	Kammon         string `yaml:"kammon"`
	ContractCommit string `yaml:"contract_commit"`
}

// Load reads stack.yml.
func Load(path string) (Stack, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Stack{}, err
	}
	var pin Stack
	if err := yaml.Unmarshal(raw, &pin); err != nil {
		return Stack{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := pin.Validate(); err != nil {
		return Stack{}, fmt.Errorf("%s: %w", path, err)
	}
	return pin, nil
}

// Validate enforces the one-number rule: both components carry the same tag.
func (s Stack) Validate() error {
	if s.Kam == "" || s.Kammon == "" {
		return fmt.Errorf("stack.yml must pin both kam and kammon")
	}
	if strings.TrimPrefix(s.Kam, "v") != strings.TrimPrefix(s.Kammon, "v") {
		return fmt.Errorf("kam %s and kammon %s are not the same version", s.Kam, s.Kammon)
	}
	return nil
}

// Version is the pinned version without the leading v, matching what
// `kam version` and `kammon -version` print.
func (s Stack) Version() string {
	return strings.TrimPrefix(s.Kam, "v")
}

// Release is what the release workflow verified about one component release.
type Release struct {
	Tag       string
	AssetURLs map[string]string
	Checksums map[string]string
}

// Manifest is the JSON document attached to a kamstack release and consumed by
// `kamstack update` (DST-26, DST-27).
type Manifest struct {
	Version string    `json:"version"`
	Kam     Component `json:"kam"`
	Kammon  Component `json:"kammon"`
}

type Component struct {
	Version string            `json:"version"`
	Targets map[string]Target `json:"targets"`
}

type Target struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

// BuildManifest checks that every expected archive and checksum is present,
// then assembles the manifest. version is the kamstack release version.
func BuildManifest(version string, pin Stack, kam, kammon Release) (Manifest, error) {
	if err := pin.Validate(); err != nil {
		return Manifest{}, err
	}
	kamComponent, err := component("kam", pin.Kam, kam)
	if err != nil {
		return Manifest{}, err
	}
	kammonComponent, err := component("kammon", pin.Kammon, kammon)
	if err != nil {
		return Manifest{}, err
	}
	return Manifest{
		Version: version,
		Kam:     kamComponent,
		Kammon:  kammonComponent,
	}, nil
}

func component(name, tag string, release Release) (Component, error) {
	if release.Tag != "" && release.Tag != tag {
		return Component{}, fmt.Errorf("%s release is %s, stack.yml pins %s", name, release.Tag, tag)
	}
	version := strings.TrimPrefix(tag, "v")
	result := Component{Version: version, Targets: map[string]Target{}}
	var missing []string
	for _, target := range Targets {
		asset := AssetName(name, version, target)
		url := release.AssetURLs[asset]
		sha := release.Checksums[asset]
		switch {
		case url == "":
			missing = append(missing, asset+" (asset)")
		case sha == "":
			missing = append(missing, asset+" (SHA256SUMS)")
		default:
			result.Targets[target] = Target{URL: url, SHA256: sha}
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return Component{}, fmt.Errorf("%s %s: missing %s", name, tag, strings.Join(missing, ", "))
	}
	return result, nil
}

// ParseManifest decodes the manifest a kamstack release publishes.
func ParseManifest(raw []byte) (Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, err
	}
	if manifest.Version == "" || manifest.Kam.Version == "" || manifest.Kammon.Version == "" {
		return Manifest{}, fmt.Errorf("manifest is missing a version")
	}
	return manifest, nil
}

// AssetName is the archive name the component release workflows publish.
func AssetName(component, version, target string) string {
	arch := strings.TrimPrefix(target, "linux/")
	return fmt.Sprintf("%s_%s_linux_%s.tar.gz", component, version, arch)
}

// ParseSHA256SUMS reads the `sha256sum` output attached to each release.
func ParseSHA256SUMS(raw []byte) map[string]string {
	checksums := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		checksums[strings.TrimPrefix(fields[1], "*")] = fields[0]
	}
	return checksums
}
