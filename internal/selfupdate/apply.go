package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ApplyOptions controls binary replacement.
type ApplyOptions struct {
	Client      *Client
	CurrentVer  string
	Force       bool // allow replacing dev/dirty builds
	DestPath    string // default: os.Executable()
}

// CheckResult is a non-mutating version comparison.
type CheckResult struct {
	Current string
	Latest  string
	Update  bool
	Asset   string
}

// Check reports whether a newer release exists.
func Check(ctx context.Context, client *Client, current string) (CheckResult, error) {
	if client == nil {
		client = &Client{}
	}
	rel, err := client.LatestRelease(ctx)
	if err != nil {
		return CheckResult{}, err
	}
	latest := NormalizeVersion(rel.TagName)
	cur := NormalizeVersion(current)
	// strip dirty suffixes from git describe e.g. 0.9.0-52-gae841c6-dirty
	curBase := strings.Split(cur, "-")[0]
	update := latest != "" && curBase != latest && curBase != "dev" && cur != ""
	// also treat equal base as no update when current is pure tag
	if cur == latest {
		update = false
	}
	// If current is newer-looking dirty describe of same base, still allow if tag differs
	if curBase == latest {
		update = false
	}
	assetName := AssetNameFor(rel.TagName)
	return CheckResult{
		Current: current,
		Latest:  latest,
		Update:  update && latest != curBase,
		Asset:   assetName,
	}, nil
}

// Apply downloads the latest release binary and replaces DestPath.
func Apply(ctx context.Context, opts ApplyOptions) (CheckResult, error) {
	client := opts.Client
	if client == nil {
		client = &Client{}
	}
	cur := NormalizeVersion(opts.CurrentVer)
	if !opts.Force && (cur == "" || cur == "dev" || strings.Contains(opts.CurrentVer, "dirty")) {
		return CheckResult{}, fmt.Errorf("refusing to self-update dev/dirty build %q (pass --force)", opts.CurrentVer)
	}

	rel, err := client.LatestRelease(ctx)
	if err != nil {
		return CheckResult{}, err
	}
	latest := NormalizeVersion(rel.TagName)
	check := CheckResult{Current: opts.CurrentVer, Latest: latest, Update: true, Asset: AssetNameFor(rel.TagName)}
	curBase := strings.Split(cur, "-")[0]
	if curBase == latest && !opts.Force {
		check.Update = false
		return check, fmt.Errorf("already on latest version %s", latest)
	}

	asset, checksums, err := FindAsset(rel)
	if err != nil {
		return check, err
	}

	var buf bytes.Buffer
	if err := client.Download(ctx, asset.BrowserDownloadURL, &buf); err != nil {
		return check, err
	}
	raw := buf.Bytes()

	if checksums.BrowserDownloadURL != "" {
		var cbuf bytes.Buffer
		if err := client.Download(ctx, checksums.BrowserDownloadURL, &cbuf); err != nil {
			return check, fmt.Errorf("download checksums: %w", err)
		}
		sum := sha256.Sum256(raw)
		want := hex.EncodeToString(sum[:])
		if !checksumLineMatch(cbuf.String(), asset.Name, want) {
			return check, fmt.Errorf("checksum mismatch for %s", asset.Name)
		}
	}

	bin, err := extractBinary(raw, asset.Name)
	if err != nil {
		return check, err
	}

	dest := opts.DestPath
	if dest == "" {
		dest, err = os.Executable()
		if err != nil {
			return check, err
		}
		dest, err = filepath.EvalSymlinks(dest)
		if err != nil {
			return check, err
		}
	}

	if err := replaceExecutable(dest, bin); err != nil {
		return check, err
	}
	return check, nil
}

func checksumLineMatch(checksums, assetName, gotHex string) bool {
	for _, line := range strings.Split(checksums, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// format: <hex>  <filename>  or <hex> *filename
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		sum := fields[0]
		name := fields[len(fields)-1]
		name = strings.TrimPrefix(name, "*")
		if name == assetName && strings.EqualFold(sum, gotHex) {
			return true
		}
	}
	return false
}

func extractBinary(archive []byte, assetName string) ([]byte, error) {
	if strings.HasSuffix(assetName, ".zip") {
		return extractZipBinary(archive)
	}
	return extractTarGzBinary(archive)
}

func extractTarGzBinary(archive []byte) ([]byte, error) {
	gr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		base := filepath.Base(hdr.Name)
		if hdr.Typeflag == tar.TypeReg && (base == "gog" || base == "gog.exe") {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("archive missing gog binary")
}

func extractZipBinary(archive []byte) ([]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, err
	}
	for _, f := range r.File {
		base := filepath.Base(f.Name)
		if base == "gog" || base == "gog.exe" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("zip missing gog binary")
}

func replaceExecutable(dest string, data []byte) error {
	dir := filepath.Dir(dest)
	tmp, err := os.CreateTemp(dir, "gog-update-*")
	if err != nil {
		return fmt.Errorf("temp binary: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// On Windows, cannot replace running exe easily; still try rename dance.
	backup := dest + ".bak"
	_ = os.Remove(backup)
	if runtime.GOOS == "windows" {
		if err := os.Rename(dest, backup); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("backup current binary: %w", err)
		}
		if err := os.Rename(tmpName, dest); err != nil {
			_ = os.Rename(backup, dest)
			return err
		}
		_ = os.Remove(backup)
		return nil
	}

	if err := os.Rename(tmpName, dest); err != nil {
		// If dest is busy, try replace via remove+rename
		if err2 := os.Remove(dest); err2 == nil {
			if err3 := os.Rename(tmpName, dest); err3 == nil {
				return nil
			}
		}
		return fmt.Errorf("replace binary: %w", err)
	}
	return nil
}
