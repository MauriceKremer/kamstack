// Command manifest builds the kamstack manifest.json from the pinned pair in
// stack.yml and the assets of the two component releases. It runs in the
// kamstack release workflow and fails loudly when a pinned tag, archive or
// checksum is missing (PLAN.md DST-25).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"kamstack.bytecoder.nl/internal/stack"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "manifest:", err)
		os.Exit(1)
	}
}

func run() error {
	stackPath := flag.String("stack", "stack.yml", "path to stack.yml")
	outPath := flag.String("out", "manifest.json", "where to write the manifest")
	version := flag.String("version", "", "kamstack release version (no leading v)")
	kamRepo := flag.String("kam-repo", "", "owner/repo of the KAM releases")
	kammonRepo := flag.String("kammon-repo", "", "owner/repo of the KAMmon releases")
	flag.Parse()

	if *version == "" {
		*version = strings.TrimPrefix(os.Getenv("GITHUB_REF_NAME"), "v")
	}
	if *version == "" || *kamRepo == "" || *kammonRepo == "" {
		return fmt.Errorf("-version, -kam-repo and -kammon-repo are required")
	}

	pin, err := stack.Load(*stackPath)
	if err != nil {
		return err
	}
	kam, err := fetchRelease(*kamRepo, pin.Kam)
	if err != nil {
		return fmt.Errorf("kam %s: %w", pin.Kam, err)
	}
	kammon, err := fetchRelease(*kammonRepo, pin.Kammon)
	if err != nil {
		return fmt.Errorf("kammon %s: %w", pin.Kammon, err)
	}

	manifest, err := stack.BuildManifest(*version, pin, kam, kammon)
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(*outPath, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("manifest %s: kam %s, kammon %s, %d targets each\n",
		*version, manifest.Kam.Version, manifest.Kammon.Version, len(stack.Targets))
	return nil
}

type releaseResponse struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		URL                string `json:"url"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// fetchRelease reads one component release and its SHA256SUMS.
func fetchRelease(repo, tag string) (stack.Release, error) {
	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", repo, url.PathEscape(tag))
	raw, err := apiGet(endpoint, "application/vnd.github+json")
	if err != nil {
		return stack.Release{}, err
	}
	var release releaseResponse
	if err := json.Unmarshal(raw, &release); err != nil {
		return stack.Release{}, fmt.Errorf("parse release %s: %w", tag, err)
	}

	urls := map[string]string{}
	checksumURL := ""
	for _, asset := range release.Assets {
		urls[asset.Name] = asset.BrowserDownloadURL
		if asset.Name == "SHA256SUMS" {
			checksumURL = asset.URL
		}
	}
	if checksumURL == "" {
		return stack.Release{}, fmt.Errorf("release %s has no SHA256SUMS asset", tag)
	}
	checksumRaw, err := apiGet(checksumURL, "application/octet-stream")
	if err != nil {
		return stack.Release{}, fmt.Errorf("download SHA256SUMS for %s: %w", tag, err)
	}
	return stack.Release{
		Tag:       release.TagName,
		AssetURLs: urls,
		Checksums: stack.ParseSHA256SUMS(checksumRaw),
	}, nil
}

func apiGet(endpoint, accept string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "kamstack-manifest")
	if token := token(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s: %s", endpoint, resp.Status, snippet(body))
	}
	return body, nil
}

func token() string {
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token
	}
	return os.Getenv("GH_TOKEN")
}

func snippet(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > 200 {
		return text[:200] + "..."
	}
	return text
}
