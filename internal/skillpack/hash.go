package skillpack

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// HashManagedFiles hashes the concatenation of managed relative paths and contents.
// Missing files contribute an empty content segment so absence is visible in the hash.
func HashManagedFiles(root string, managedFiles []string) (string, error) {
	files := normalizeManaged(managedFiles)
	h := sha256.New()
	for _, rel := range files {
		fmt.Fprintf(h, "%s\n", rel)
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			if os.IsNotExist(err) {
				h.Write([]byte{0})
				continue
			}
			return "", fmt.Errorf("read %s: %w", rel, err)
		}
		h.Write(b)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// HashManagedFromFS hashes managed files from an fs.FS under baseDir (e.g. "pack/google-calendar").
func HashManagedFromFS(fsys fs.FS, baseDir string, managedFiles []string) (string, error) {
	files := normalizeManaged(managedFiles)
	h := sha256.New()
	for _, rel := range files {
		fmt.Fprintf(h, "%s\n", rel)
		path := pathJoin(baseDir, rel)
		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			if os.IsNotExist(err) || errorsIsNotExist(err) {
				h.Write([]byte{0})
				continue
			}
			return "", fmt.Errorf("read pack %s: %w", path, err)
		}
		h.Write(b)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func normalizeManaged(managedFiles []string) []string {
	if len(managedFiles) == 0 {
		return []string{"SKILL.md"}
	}
	out := append([]string(nil), managedFiles...)
	sort.Strings(out)
	return out
}

func pathJoin(base, rel string) string {
	base = strings.TrimSuffix(base, "/")
	if base == "" || base == "." {
		return rel
	}
	return base + "/" + rel
}

func errorsIsNotExist(err error) bool {
	return err != nil && (os.IsNotExist(err) || strings.Contains(err.Error(), "file does not exist"))
}
