package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	errRefuseDevDirty   = errors.New("refusing to self-update dev/dirty build")
	errAlreadyLatest    = errors.New("already on latest version")
	errChecksumMismatch = errors.New("checksum mismatch")
	errArchiveNoBinary  = errors.New("archive missing gog binary")
)

// ApplyOptions controls binary replacement.
type ApplyOptions struct {
	Client     *Client
	CurrentVer string
	Force      bool   // allow replacing dev/dirty builds
	DestPath   string // default: os.Executable()
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
	curBase := strings.Split(cur, "-")[0]
	update := versionLess(curBase, latest)
	assetName := AssetNameFor(rel.TagName)

	return CheckResult{
		Current: current,
		Latest:  latest,
		Update:  update,
		Asset:   assetName,
	}, nil
}

// versionLess reports whether a is strictly older than b (semver-ish X.Y.Z).
func versionLess(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)

	if b == "" || a == b {
		return false
	}

	if a == "" || a == "dev" {
		return true
	}

	ap := parseVersionParts(a)
	bp := parseVersionParts(b)

	if ap == nil || bp == nil {
		return a != b && bp != nil
	}

	for i := 0; i < 3; i++ {
		if ap[i] < bp[i] {
			return true
		}

		if ap[i] > bp[i] {
			return false
		}
	}

	return false
}

func parseVersionParts(v string) []int {
	v = strings.Split(v, "-")[0]
	parts := strings.Split(v, ".")

	if len(parts) < 1 || len(parts) > 4 {
		return nil
	}

	out := make([]int, 3)

	for i := 0; i < len(parts) && i < 3; i++ {
		n := 0

		for _, ch := range parts[i] {
			if ch < '0' || ch > '9' {
				return nil
			}

			n = n*10 + int(ch-'0')
		}

		out[i] = n
	}

	return out
}

// Apply downloads the latest release binary and replaces DestPath.
func Apply(ctx context.Context, opts ApplyOptions) (CheckResult, error) {
	client := opts.Client
	if client == nil {
		client = &Client{}
	}

	cur := NormalizeVersion(opts.CurrentVer)
	if !opts.Force && (cur == "" || cur == "dev" || strings.Contains(opts.CurrentVer, "dirty")) {
		return CheckResult{}, fmt.Errorf("%w %q (pass --force-binary)", errRefuseDevDirty, opts.CurrentVer)
	}

	rel, err := client.LatestRelease(ctx)
	if err != nil {
		return CheckResult{}, err
	}

	latest := NormalizeVersion(rel.TagName)
	check := CheckResult{Current: opts.CurrentVer, Latest: latest, Update: true, Asset: AssetNameFor(rel.TagName)}
	curBase := strings.Split(cur, "-")[0]

	if !versionLess(curBase, latest) && !opts.Force {
		check.Update = false

		return check, fmt.Errorf("%w %s", errAlreadyLatest, latest)
	}

	asset, checksums, err := FindAsset(rel)
	if err != nil {
		return check, err
	}

	var buf bytes.Buffer
	if err = client.Download(ctx, asset.BrowserDownloadURL, &buf); err != nil {
		return check, err
	}

	raw := buf.Bytes()

	if checksums.BrowserDownloadURL != "" {
		var cbuf bytes.Buffer
		if err = client.Download(ctx, checksums.BrowserDownloadURL, &cbuf); err != nil {
			return check, fmt.Errorf("download checksums: %w", err)
		}

		sum := sha256.Sum256(raw)
		want := hex.EncodeToString(sum[:])

		if !checksumLineMatch(cbuf.String(), asset.Name, want) {
			return check, fmt.Errorf("%w for %s", errChecksumMismatch, asset.Name)
		}
	}

	bin, err := extractBinary(raw, asset.Name)
	if err != nil {
		return check, err
	}

	dest := opts.DestPath
	if dest == "" {
		exe, exeErr := os.Executable()
		if exeErr != nil {
			return check, fmt.Errorf("resolve executable: %w", exeErr)
		}

		exe, linkErr := filepath.EvalSymlinks(exe)
		if linkErr != nil {
			return check, fmt.Errorf("resolve executable path: %w", linkErr)
		}

		dest = exe
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
		hdr, nextErr := tr.Next()
		if nextErr == io.EOF {
			break
		}

		if nextErr != nil {
			return nil, fmt.Errorf("tar next: %w", nextErr)
		}

		base := filepath.Base(hdr.Name)
		if hdr.Typeflag == tar.TypeReg && (base == "gog" || base == "gog.exe") {
			b, readErr := io.ReadAll(tr)
			if readErr != nil {
				return nil, fmt.Errorf("read binary from tar: %w", readErr)
			}

			return b, nil
		}
	}

	return nil, errArchiveNoBinary
}

func extractZipBinary(archive []byte) ([]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("zip: %w", err)
	}

	for _, f := range r.File {
		base := filepath.Base(f.Name)
		if base != "gog" && base != "gog.exe" {
			continue
		}

		rc, openErr := f.Open()
		if openErr != nil {
			return nil, fmt.Errorf("open zip entry: %w", openErr)
		}

		b, readErr := io.ReadAll(rc)
		_ = rc.Close()

		if readErr != nil {
			return nil, fmt.Errorf("read zip entry: %w", readErr)
		}

		return b, nil
	}

	return nil, errArchiveNoBinary
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

		return fmt.Errorf("write temp binary: %w", err)
	}

	if err := tmp.Chmod(0o755); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("chmod temp binary: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp binary: %w", err)
	}

	backup := dest + ".bak"
	_ = os.Remove(backup)

	if runtime.GOOS == "windows" {
		if err := os.Rename(dest, backup); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("backup current binary: %w", err)
		}

		if err := os.Rename(tmpName, dest); err != nil {
			_ = os.Rename(backup, dest)

			return fmt.Errorf("replace binary: %w", err)
		}

		_ = os.Remove(backup)

		return nil
	}

	if err := os.Rename(tmpName, dest); err != nil {
		if err2 := os.Remove(dest); err2 == nil {
			if err3 := os.Rename(tmpName, dest); err3 == nil {
				return nil
			}
		}

		return fmt.Errorf("replace binary: %w", err)
	}

	return nil
}
