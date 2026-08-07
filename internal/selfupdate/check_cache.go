package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/config"
)

// CheckCache stores last successful remote version check.
type CheckCache struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
	Current   string    `json:"current"`
}

func checkCachePath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "update-check.json"), nil
}

// LoadCheckCache returns the cache (zero if missing).
func LoadCheckCache() (CheckCache, error) {
	path, err := checkCachePath()
	if err != nil {
		return CheckCache{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return CheckCache{}, nil
		}
		return CheckCache{}, err
	}
	var c CheckCache
	if err := json.Unmarshal(b, &c); err != nil {
		return CheckCache{}, err
	}
	return c, nil
}

// SaveCheckCache writes the cache.
func SaveCheckCache(c CheckCache) error {
	if _, err := config.EnsureDir(); err != nil {
		return err
	}
	path, err := checkCachePath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// MaybeNotify checks for updates at most every interval and returns a notice line if update available.
// Errors and "up to date" return empty string.
func MaybeNotify(ctx context.Context, client *Client, current string, interval time.Duration) string {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	if os.Getenv("GOG_SKIP_UPDATE_CHECK") == "1" || os.Getenv("GOG_SKIP_UPDATE_CHECK") == "true" {
		return ""
	}
	// Avoid network during `go test` and when callers set GOG_TEST=1.
	if testing.Testing() || os.Getenv("GOG_TEST") == "1" {
		return ""
	}
	cache, _ := LoadCheckCache()
	if !cache.CheckedAt.IsZero() && time.Since(cache.CheckedAt) < interval {
		// Re-use cached latest for notice without network.
		if cache.Latest != "" {
			curBase := NormalizeVersion(current)
			curBase = splitBase(curBase)
			if cache.Latest != curBase && curBase != "dev" && curBase != "" {
				return fmt.Sprintf("gog: update available %s → %s; run: gog update", curBase, cache.Latest)
			}
		}
		return ""
	}

	res, err := Check(ctx, client, current)
	if err != nil {
		return ""
	}
	_ = SaveCheckCache(CheckCache{
		CheckedAt: time.Now().UTC(),
		Latest:    res.Latest,
		Current:   current,
	})
	if res.Update {
		return fmt.Sprintf("gog: update available %s → %s; run: gog update", splitBase(NormalizeVersion(current)), res.Latest)
	}
	return ""
}

func splitBase(v string) string {
	if i := indexDash(v); i >= 0 {
		return v[:i]
	}
	return v
}

func indexDash(v string) int {
	for i := 0; i < len(v); i++ {
		if v[i] == '-' {
			return i
		}
	}
	return -1
}
