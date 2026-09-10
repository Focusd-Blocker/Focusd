package extension

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	releaseURL    = "https://api.github.com/repos/Focusd-Blocker/Focusd/releases?per_page=100"
	checkInterval = 6 * time.Hour
)

var httpClient = &http.Client{Timeout: 60 * time.Second}

type releaseInfo struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

type localState struct {
	ETag    string `json:"etag"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

func extensionDir() string {
	cacheDir, _ := os.UserCacheDir()
	return filepath.Join(cacheDir, "focusd", "extension")
}

func XPPIfPath() string {
	return filepath.Join(extensionDir(), "focusd.xpi")
}

func XPIFileURL() string {
	p := XPPIfPath()
	if runtime.GOOS == "windows" {
		return "file:///" + filepath.ToSlash(p)
	}
	return "file://" + p
}

func statePath() string {
	return filepath.Join(extensionDir(), "state.json")
}

func loadState() (*localState, error) {
	data, err := os.ReadFile(statePath())
	if err != nil {
		if os.IsNotExist(err) {
			return &localState{}, nil
		}
		return nil, err
	}
	var s localState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func saveState(s *localState) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(), data, 0o644)
}

func fetchLatestRelease() (*releaseInfo, error) {
	req, err := http.NewRequest("GET", releaseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "focusd-extension-updater")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching release info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var releases []releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decoding release info: %w", err)
	}

	for i := range releases {
		tag := releases[i].TagName
		if strings.HasPrefix(tag, "extension-v") {
			return &releases[i], nil
		}
		if strings.HasPrefix(tag, "v") {
			for _, asset := range releases[i].Assets {
				if asset.Name == "focusd.xpi" {
					return &releases[i], nil
				}
			}
		}
	}
	return nil, fmt.Errorf("no extension release found")
}

func downloadXPI(downloadURL string) (string, error) {
	dir := extensionDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating extension dir: %w", err)
	}

	tmpPath := filepath.Join(dir, "focusd.xpi.tmp")
	finalPath := filepath.Join(dir, "focusd.xpi")

	resp, err := httpClient.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("downloading xpi: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned %d", resp.StatusCode)
	}

	f, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}

	hasher := sha256.New()
	writer := io.MultiWriter(f, hasher)

	if _, err := io.Copy(writer, resp.Body); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("writing xpi: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("closing temp file: %w", err)
	}

	sum := hex.EncodeToString(hasher.Sum(nil))

	if err := os.Rename(tmpPath, finalPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("renaming temp file: %w", err)
	}

	if err := os.Chmod(finalPath, 0o644); err != nil {
		return "", fmt.Errorf("chmod xpi: %w", err)
	}

	return sum, nil
}

func updateOnce() error {
	state, err := loadState()
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	release, err := fetchLatestRelease()
	if err != nil {
		return fmt.Errorf("checking release: %w", err)
	}

	version := strings.TrimPrefix(release.TagName, "extension-v")
	if version == release.TagName {
		version = strings.TrimPrefix(release.TagName, "v")
	}
	if state.Version == version {
		log.Println("extension: already up to date")
		return nil
	}

	log.Println("extension: new version detected, downloading...")
	downloadURL := ""
	for _, asset := range release.Assets {
		if asset.Name == "focusd.xpi" {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("extension release has no focusd.xpi asset")
	}
	sum, err := downloadXPI(downloadURL)
	if err != nil {
		return fmt.Errorf("downloading extension: %w", err)
	}

	state.ETag = ""
	state.Version = version
	state.SHA256 = sum
	if err := saveState(state); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	log.Printf("extension: updated successfully (sha256=%s)", sum[:16])
	return nil
}

// StartUpdateLoop performs an initial download then checks for updates
// on a schedule. It blocks until ctx is cancelled.
func StartUpdateLoop(ctx context.Context) {
	if err := updateOnce(); err != nil {
		log.Println("extension: initial update failed:", err)
	}

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := updateOnce(); err != nil {
				log.Println("extension: scheduled update failed:", err)
			}
		}
	}
}

// EnsureDownloaded does a one-shot download if the xpi doesn't exist yet.
// Called during install before policies are written.
func EnsureDownloaded() error {
	p := XPPIfPath()
	if _, err := os.Stat(p); err == nil {
		return nil
	}
	_, err := os.Stat(statePath())
	if err == nil {
		return nil
	}
	log.Println("extension: downloading xpi for the first time...")
	err = updateOnce()
	return err
}
