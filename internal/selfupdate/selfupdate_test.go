package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var errTestRename = errors.New("rename failed")

func TestNormalizeAndAssetName(t *testing.T) {
	t.Parallel()

	if got := NormalizeVersion("v1.2.3"); got != "1.2.3" {
		t.Fatalf("normalize: %s", got)
	}

	name := AssetNameFor("v1.2.3")
	if !strings.Contains(name, "gogcli_1.2.3_") {
		t.Fatalf("asset name: %s", name)
	}

	if runtime.GOOS == goosWindows {
		if !strings.HasSuffix(name, ".zip") {
			t.Fatalf("windows asset should be zip: %s", name)
		}
	} else if !strings.HasSuffix(name, ".tar.gz") {
		t.Fatalf("unix asset should be tar.gz: %s", name)
	}
}

func TestVersionLess(t *testing.T) {
	t.Parallel()

	if !versionLess("0.9.0", "0.10.0") {
		t.Fatal("0.9 < 0.10")
	}

	if versionLess("0.10.0", "0.9.0") {
		t.Fatal("0.10 should not be < 0.9")
	}

	if versionLess("1.0.0", "1.0.0") {
		t.Fatal("equal")
	}

	if !versionLess("dev", "1.0.0") {
		t.Fatal("dev older")
	}
}

func TestCheckAndApply(t *testing.T) {
	binaryPayload := []byte("#!/bin/sh\necho fake-gog\n")

	var archive bytes.Buffer

	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: "gog", Mode: 0o755, Size: int64(len(binaryPayload))}

	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}

	if _, err := tw.Write(binaryPayload); err != nil {
		t.Fatal(err)
	}

	_ = tw.Close()
	_ = gz.Close()

	raw := archive.Bytes()
	sum := sha256.Sum256(raw)
	sumHex := hex.EncodeToString(sum[:])
	assetName := AssetNameFor("v9.9.9")

	var srv *httptest.Server

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			_ = json.NewEncoder(w).Encode(Release{
				TagName: "v9.9.9",
				Assets: []Asset{
					{Name: assetName, BrowserDownloadURL: srv.URL + "/asset"},
					{Name: "checksums.txt", BrowserDownloadURL: srv.URL + "/checksums"},
				},
			})
		case r.URL.Path == "/asset":
			_, _ = w.Write(raw)
		case r.URL.Path == "/checksums":
			_, _ = fmt.Fprintf(w, "%s  %s\n", sumHex, assetName)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	ctx := context.Background()

	check, checkErr := Check(ctx, c, "1.0.0")
	if checkErr != nil {
		t.Fatal(checkErr)
	}

	if !check.Update || check.Latest != "9.9.9" {
		t.Fatalf("check: %+v", check)
	}

	dest := filepath.Join(t.TempDir(), "gog")
	if err := os.WriteFile(dest, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, applyErr := Apply(ctx, ApplyOptions{
		Client:     c,
		CurrentVer: "1.0.0",
		DestPath:   dest,
	})
	if applyErr != nil {
		t.Fatal(applyErr)
	}

	got, readErr := os.ReadFile(dest)
	if readErr != nil {
		t.Fatal(readErr)
	}

	if !bytes.Equal(got, binaryPayload) {
		t.Fatalf("binary not replaced")
	}
}

func TestChecksumLineMatch(t *testing.T) {
	t.Parallel()

	if !checksumLineMatch("abc  file.tar.gz\n", "file.tar.gz", "abc") {
		t.Fatal("expected match")
	}

	if checksumLineMatch("abc  other\n", "file.tar.gz", "abc") {
		t.Fatal("unexpected match")
	}
}

func TestMaybeNotifyThrottlesFailedChecks(t *testing.T) {
	originalTesting := isTestProcess
	isTestProcess = func() bool { return false }

	t.Cleanup(func() { isTestProcess = originalTesting })
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GOG_TEST", "")
	t.Setenv("GOG_SKIP_UPDATE_CHECK", "")

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++

		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	client := &Client{BaseURL: server.URL, HTTP: server.Client()}

	if notice := MaybeNotify(context.Background(), client, "1.2.3", time.Hour); notice != "" {
		t.Fatalf("first notice = %q, want silent failure", notice)
	}

	if notice := MaybeNotify(context.Background(), client, "1.2.3", time.Hour); notice != "" {
		t.Fatalf("second notice = %q, want cached silent failure", notice)
	}

	if requests != 1 {
		t.Fatalf("release checks = %d, want 1 within throttle interval", requests)
	}
}

func TestApplyPreservesExecutableWhenAtomicReplaceFails(t *testing.T) {
	if runtime.GOOS == goosWindows {
		t.Skip("Unix atomic replacement behavior")
	}

	originalRename := renameFile
	renameFile = func(_, _ string) error { return errTestRename }

	t.Cleanup(func() { renameFile = originalRename })

	binaryPayload := []byte("#!/bin/sh\necho replacement\n")
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)

	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "gog", Mode: 0o755, Size: int64(len(binaryPayload))}); err != nil {
		t.Fatal(err)
	}

	if _, err := tw.Write(binaryPayload); err != nil {
		t.Fatal(err)
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	assetName := AssetNameFor("v9.9.9")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/Robben-Media/gogcli/releases/latest":
			_ = json.NewEncoder(w).Encode(Release{
				TagName: "v9.9.9",
				Assets:  []Asset{{Name: assetName, BrowserDownloadURL: "http://" + r.Host + "/asset"}},
			})
		case "/asset":
			_, _ = w.Write(archive.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	dest := filepath.Join(t.TempDir(), "gog")
	if err := os.WriteFile(dest, []byte("original"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := Apply(context.Background(), ApplyOptions{
		Client:     &Client{BaseURL: server.URL, HTTP: server.Client()},
		CurrentVer: "1.2.3",
		DestPath:   dest,
	})
	if err == nil {
		t.Fatal("Apply succeeded despite rename failure")
	}

	got, readErr := os.ReadFile(dest)
	if readErr != nil {
		t.Fatalf("read original executable: %v", readErr)
	}

	if string(got) != "original" {
		t.Fatalf("original executable = %q, want preserved content", got)
	}
}
