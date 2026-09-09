package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	releaseURL    = "https://api.github.com/repos/Focusd-Blocker/Focusd/releases/latest"
	manifestURL   = "https://github.com/Focusd-Blocker/Focusd/releases/latest/download/release-manifest.json"
	checkInterval = 6 * time.Hour
)

var httpClient = &http.Client{Timeout: 90 * time.Second}

type releaseInfo struct {
	TagName string `json:"tag_name"`
}

type releaseManifest struct {
	DaemonVersion    string `json:"daemon_version"`
	ExtensionVersion string `json:"extension_version"`
	Artifacts        map[string]struct {
		SHA256 string `json:"sha256"`
	} `json:"artifacts"`
}

func StartUpdateLoop(ctx context.Context, currentVersion string) {
	if err := updateOnce(currentVersion); err != nil {
		log.Println("daemon update:", err)
	}

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := updateOnce(currentVersion); err != nil {
				log.Println("daemon update:", err)
			}
		}
	}
}

func updateOnce(currentVersion string) error {
	release, err := fetchRelease()
	if err != nil {
		return err
	}
	latest := strings.TrimPrefix(release.TagName, "v")

	manifest, err := fetchManifest()
	if err != nil {
		return fmt.Errorf("fetching release manifest: %w", err)
	}
	if manifest.DaemonVersion == "" {
		return fmt.Errorf("release manifest has no daemon version")
	}
	if manifest.DaemonVersion == currentVersion {
		return nil
	}

	assetName := fmt.Sprintf("focusd-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}
	expected, ok := manifest.Artifacts[assetName]
	if !ok || !isSHA256(expected.SHA256) {
		return fmt.Errorf("release manifest has no valid checksum for %s", assetName)
	}

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating current executable: %w", err)
	}
	tempPath := executable + ".update"
	if err := downloadAndVerify(assetName, expected.SHA256, tempPath); err != nil {
		os.Remove(tempPath)
		return err
	}

	helperPath := executable + ".updater"
	if err := copyFile(executable, helperPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("creating update helper: %w", err)
	}

	cmd := exec.Command(helperPath, "--apply-update", tempPath, executable, helperPath)
	cmd.Dir = filepath.Dir(executable)
	if err := cmd.Start(); err != nil {
		os.Remove(tempPath)
		os.Remove(helperPath)
		return fmt.Errorf("starting update helper: %w", err)
	}

	log.Printf("daemon update: %s is ready; restarting", latest)
	os.Exit(0)
	return nil
}

func fetchRelease() (*releaseInfo, error) {
	req, err := http.NewRequest(http.MethodGet, releaseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "focusd-daemon-updater")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("checking latest release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %s", resp.Status)
	}

	var release releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decoding release: %w", err)
	}
	return &release, nil
}

func fetchManifest() (*releaseManifest, error) {
	resp, err := httpClient.Get(manifestURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest download returned %s", resp.Status)
	}

	var manifest releaseManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decoding manifest: %w", err)
	}
	return &manifest, nil
}

func downloadAndVerify(assetName, expectedHash, path string) error {
	url := "https://github.com/Focusd-Blocker/Focusd/releases/latest/download/" + assetName
	resp, err := httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", assetName, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %s", resp.Status)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("creating update file: %w", err)
	}
	hasher := sha256.New()
	if _, err := io.Copy(f, io.TeeReader(resp.Body, hasher)); err != nil {
		f.Close()
		return fmt.Errorf("writing update file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing update file: %w", err)
	}

	actual := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actual, expectedHash) {
		return fmt.Errorf("checksum mismatch for %s", assetName)
	}
	return nil
}

func ApplyPendingUpdate(args []string) error {
	if len(args) != 3 {
		return errors.New("usage: --apply-update <download> <target> <helper>")
	}
	downloadPath, targetPath, helperPath := args[0], args[1], args[2]

	for attempt := 0; attempt < 30; attempt++ {
		if err := replaceExecutable(downloadPath, targetPath); err == nil {
			cmd := exec.Command(targetPath, "--cleanup-updater", helperPath)
			cmd.Dir = filepath.Dir(targetPath)
			if err := cmd.Start(); err != nil {
				return fmt.Errorf("starting updated daemon: %w", err)
			}
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return errors.New("timed out waiting to replace current executable")
}

func replaceExecutable(downloadPath, targetPath string) error {
	backupPath := targetPath + ".previous"
	_ = os.Remove(backupPath)
	if err := os.Rename(targetPath, backupPath); err != nil {
		return err
	}
	if err := os.Rename(downloadPath, targetPath); err != nil {
		_ = os.Rename(backupPath, targetPath)
		return err
	}
	_ = os.Remove(backupPath)
	return nil
}

func copyFile(sourcePath, targetPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	info, err := source.Stat()
	if err != nil {
		return err
	}

	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(target, source); err != nil {
		target.Close()
		return err
	}
	return target.Close()
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
