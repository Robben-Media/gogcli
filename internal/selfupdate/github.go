package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"
)

const defaultRepo = "Robben-Media/gogcli"

// Release is a subset of the GitHub release API payload.
type Release struct {
	TagName string  `json:"tag_name"`
	Name    string  `json:"name"`
	Assets  []Asset `json:"assets"`
}

// Asset is a release asset.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// Client fetches release metadata.
type Client struct {
	HTTP    *http.Client
	Repo    string
	BaseURL string // default https://api.github.com
	Token   string // optional GITHUB_TOKEN
}

func (c *Client) repo() string {
	if strings.TrimSpace(c.Repo) != "" {
		return strings.TrimSpace(c.Repo)
	}
	return defaultRepo
}

func (c *Client) base() string {
	if strings.TrimSpace(c.BaseURL) != "" {
		return strings.TrimSuffix(strings.TrimSpace(c.BaseURL), "/")
	}
	return "https://api.github.com"
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// LatestRelease fetches /repos/{repo}/releases/latest.
func (c *Client) LatestRelease(ctx context.Context) (Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", c.base(), c.repo())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "gogcli-selfupdate")
	if t := strings.TrimSpace(c.Token); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	res, err := c.httpClient().Do(req)
	if err != nil {
		return Release{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4<<10))
		return Release{}, fmt.Errorf("github releases: %s: %s", res.Status, strings.TrimSpace(string(b)))
	}
	var rel Release
	if err := json.NewDecoder(res.Body).Decode(&rel); err != nil {
		return Release{}, fmt.Errorf("decode release: %w", err)
	}
	if strings.TrimSpace(rel.TagName) == "" {
		return Release{}, fmt.Errorf("release missing tag_name")
	}
	return rel, nil
}

// NormalizeVersion strips a leading v from tags.
func NormalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	return strings.TrimPrefix(v, "v")
}

// AssetNameFor builds the GoReleaser asset name for this platform.
func AssetNameFor(version string) string {
	ver := NormalizeVersion(version)
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("gogcli_%s_%s_%s.%s", ver, goos, goarch, ext)
}

// FindAsset returns the platform asset and checksums asset if present.
func FindAsset(rel Release) (asset Asset, checksums Asset, err error) {
	want := AssetNameFor(rel.TagName)
	for _, a := range rel.Assets {
		switch {
		case a.Name == want:
			asset = a
		case a.Name == "checksums.txt":
			checksums = a
		}
	}
	if asset.Name == "" {
		return Asset{}, Asset{}, fmt.Errorf("no release asset for %s (looked for %s)", runtime.GOOS+"/"+runtime.GOARCH, want)
	}
	return asset, checksums, nil
}

// Download writes the URL body to w.
func (c *Client) Download(ctx context.Context, url string, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "gogcli-selfupdate")
	if t := strings.TrimSpace(c.Token); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	res, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4<<10))
		return fmt.Errorf("download %s: %s: %s", url, res.Status, strings.TrimSpace(string(b)))
	}
	_, err = io.Copy(w, res.Body)
	return err
}
