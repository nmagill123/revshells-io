package agents

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// GitSyncConfig downloads published rs-agent binaries into agents-dir at startup.
type GitSyncConfig struct {
	Repo string // https://github.com/nmagill123/revshells-io
	Tag  string // v0.1.0; empty uses latest release
	Dir  string
}

// releaseAsset maps agents-dir platform name to GitHub release filename.
var releaseAssets = []struct {
	Platform string
	Asset    string
}{
	{Platform: "linux-amd64", Asset: "rs-agent-linux-amd64-x86_64"},
	{Platform: "linux-arm64", Asset: "rs-agent-linux-arm64-aarch64"},
	{Platform: "linux-386", Asset: "rs-agent-linux-386-x86"},
	{Platform: "darwin-amd64", Asset: "rs-agent-darwin-amd64-x86_64"},
	{Platform: "darwin-arm64", Asset: "rs-agent-darwin-arm64-aarch64"},
}

// ReleaseDownloadBase returns the URL prefix for release asset downloads.
func ReleaseDownloadBase(cfg GitSyncConfig) string {
	repo := strings.TrimRight(cfg.Repo, "/")
	if repo == "" {
		repo = "https://github.com/nmagill123/revshells-io"
	}
	if cfg.Tag != "" {
		tag := cfg.Tag
		if !strings.HasPrefix(tag, "v") {
			tag = "v" + tag
		}
		return fmt.Sprintf("%s/releases/download/%s", repo, tag)
	}
	return repo + "/releases/latest/download"
}

// SyncFromGitHub downloads all release rs-agent binaries into Dir (platform-named files).
func SyncFromGitHub(cfg GitSyncConfig) error {
	if cfg.Dir == "" {
		return fmt.Errorf("agents directory required")
	}
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return err
	}
	base := ReleaseDownloadBase(cfg)
	client := &http.Client{Timeout: 10 * time.Minute}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)
	for _, p := range releaseAssets {
		wg.Add(1)
		go func(p struct{ Platform, Asset string }) {
			defer wg.Done()
			url := base + "/" + p.Asset
			if err := downloadReleaseAsset(client, url, filepath.Join(cfg.Dir, p.Platform)); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("%s: %w", p.Platform, err)
				}
				mu.Unlock()
			}
		}(p)
	}
	wg.Wait()
	return firstErr
}

func downloadReleaseAsset(client *http.Client, url, dest string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	tmp := dest + ".download"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, resp.Body)
	closeErr := f.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}
