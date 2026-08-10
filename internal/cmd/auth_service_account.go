package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type AuthServiceAccountCmd struct {
	Set    AuthServiceAccountSetCmd    `cmd:"" name:"set" help:"Store a service account key for impersonation"`
	Unset  AuthServiceAccountUnsetCmd  `cmd:"" name:"unset" help:"Remove stored service account key"`
	Status AuthServiceAccountStatusCmd `cmd:"" name:"status" help:"Show stored service account key status"`
}

type serviceAccountJSONInfo struct {
	ClientEmail string
	ClientID    string
}

type serviceAccountTempFile interface {
	Write([]byte) (int, error)
	Chmod(os.FileMode) error
	Close() error
	Name() string
}

var (
	createServiceAccountTempFile = func(dir, pattern string) (serviceAccountTempFile, error) {
		return os.CreateTemp(dir, pattern)
	}
	renameServiceAccountFile = os.Rename
)

func parseServiceAccountJSON(data []byte) (serviceAccountJSONInfo, error) {
	var saJSON map[string]any
	if err := json.Unmarshal(data, &saJSON); err != nil {
		return serviceAccountJSONInfo{}, fmt.Errorf("invalid service account JSON: %w", err)
	}
	if saJSON["type"] != "service_account" {
		return serviceAccountJSONInfo{}, fmt.Errorf("invalid service account JSON: expected type=service_account")
	}

	info := serviceAccountJSONInfo{}
	if v, ok := saJSON["client_email"].(string); ok {
		info.ClientEmail = strings.TrimSpace(v)
	}
	if v, ok := saJSON["client_id"].(string); ok {
		info.ClientID = strings.TrimSpace(v)
	}
	return info, nil
}

type stagedServiceAccountFile struct {
	path           string
	tmpPath        string
	backupPath     string
	existed        bool
	committed      bool
	preserveBackup bool
}

func writeServiceAccountFile(path string, data []byte) error {
	return writeServiceAccountFiles([]string{path}, data)
}

func writeServiceAccountFiles(paths []string, data []byte) error {
	files := make([]stagedServiceAccountFile, 0, len(paths))
	defer func() {
		for _, file := range files {
			_ = os.Remove(file.tmpPath)
			if !file.preserveBackup {
				_ = os.Remove(file.backupPath)
			}
		}
	}()

	for _, path := range paths {
		tmp, err := createServiceAccountTempFile(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
		if err != nil {
			return err
		}
		file := stagedServiceAccountFile{path: path, tmpPath: tmp.Name()}
		files = append(files, file)
		n, err := tmp.Write(data)
		if err != nil {
			_ = tmp.Close()
			return err
		}
		if n != len(data) {
			_ = tmp.Close()
			return io.ErrShortWrite
		}
		if err := tmp.Chmod(0o600); err != nil {
			_ = tmp.Close()
			return err
		}
		if err := tmp.Close(); err != nil {
			return err
		}
	}

	rollback := func() error {
		var rollbackErr error
		for i := len(files) - 1; i >= 0; i-- {
			file := &files[i]
			if file.backupPath != "" {
				if err := renameServiceAccountFile(file.backupPath, file.path); err != nil {
					file.preserveBackup = true
					rollbackErr = errors.Join(rollbackErr, fmt.Errorf("restore %s from %s: %w", file.path, file.backupPath, err))
					continue
				}
				file.backupPath = ""
				file.committed = false
				continue
			}
			if file.committed && !file.existed {
				if err := os.Remove(file.path); err != nil && !os.IsNotExist(err) {
					rollbackErr = errors.Join(rollbackErr, fmt.Errorf("remove newly installed %s: %w", file.path, err))
				}
			}
		}
		return rollbackErr
	}

	for i := range files {
		file := &files[i]
		if _, err := os.Stat(file.path); err == nil {
			file.existed = true
			backup, createErr := os.CreateTemp(filepath.Dir(file.path), "."+filepath.Base(file.path)+".backup-*")
			if createErr != nil {
				return errors.Join(createErr, rollback())
			}
			backupPath := backup.Name()
			if closeErr := backup.Close(); closeErr != nil {
				_ = os.Remove(backupPath)
				return errors.Join(closeErr, rollback())
			}
			if removeErr := os.Remove(backupPath); removeErr != nil {
				return errors.Join(removeErr, rollback())
			}
			if renameErr := renameServiceAccountFile(file.path, backupPath); renameErr != nil {
				return errors.Join(renameErr, rollback())
			}
			file.backupPath = backupPath
		} else if !os.IsNotExist(err) {
			return errors.Join(err, rollback())
		}
	}

	for i := range files {
		file := &files[i]
		if err := renameServiceAccountFile(file.tmpPath, file.path); err != nil {
			return errors.Join(err, rollback())
		}
		file.tmpPath = ""
		file.committed = true
	}
	return nil
}

func storeServiceAccountKey(impersonateEmail string, keyPath string) (string, serviceAccountJSONInfo, error) {
	keyPath = strings.TrimSpace(keyPath)
	if keyPath == "" {
		return "", serviceAccountJSONInfo{}, usage("empty key path")
	}
	keyPath, err := config.ExpandPath(keyPath)
	if err != nil {
		return "", serviceAccountJSONInfo{}, err
	}

	data, err := os.ReadFile(keyPath) //nolint:gosec // user-provided path
	if err != nil {
		return "", serviceAccountJSONInfo{}, fmt.Errorf("read service account key: %w", err)
	}

	info, err := parseServiceAccountJSON(data)
	if err != nil {
		return "", serviceAccountJSONInfo{}, err
	}

	destPath, err := config.ServiceAccountPath(impersonateEmail)
	if err != nil {
		return "", serviceAccountJSONInfo{}, err
	}

	if _, err := config.EnsureDir(); err != nil {
		return "", serviceAccountJSONInfo{}, err
	}

	if err := writeServiceAccountFile(destPath, data); err != nil {
		return "", serviceAccountJSONInfo{}, fmt.Errorf("write service account: %w", err)
	}

	return destPath, info, nil
}

type AuthServiceAccountSetCmd struct {
	Email string `arg:"" name:"email" help:"Email to impersonate (Workspace user email)" required:""`
	Key   string `name:"key" required:"" help:"Path to service account JSON key file"`
}

func (c *AuthServiceAccountSetCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)

	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("empty email")
	}

	destPath, info, err := storeServiceAccountKey(email, c.Key)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"stored":       true,
			"email":        email,
			"path":         destPath,
			"client_email": info.ClientEmail,
			"client_id":    info.ClientID,
		})
	}
	u.Out().Printf("email\t%s", email)
	u.Out().Printf("path\t%s", destPath)
	if info.ClientEmail != "" {
		u.Out().Printf("client_email\t%s", info.ClientEmail)
	}
	if info.ClientID != "" {
		u.Out().Printf("client_id\t%s", info.ClientID)
	}
	advice := "Service account configured. Use: gog <cmd> --account " + email
	if outfmt.IsPlain(ctx) {
		u.Err().Println(advice)
	} else {
		u.Out().Println(advice)
	}
	return nil
}

type AuthServiceAccountUnsetCmd struct {
	Email string `arg:"" name:"email" help:"Email (impersonated user)" required:""`
}

func (c *AuthServiceAccountUnsetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("empty email")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("remove stored service account for %s", email)); err != nil {
		return err
	}

	path, err := config.ServiceAccountPath(email)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			if outfmt.IsJSON(ctx) {
				return outfmt.WriteJSON(os.Stdout, map[string]any{
					"deleted": false,
					"email":   email,
					"path":    path,
				})
			}
			u.Out().Printf("deleted\tfalse")
			u.Out().Printf("email\t%s", email)
			u.Out().Printf("path\t%s", path)
			return nil
		}
		return fmt.Errorf("remove service account: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"deleted": true,
			"email":   email,
			"path":    path,
		})
	}
	u.Out().Printf("deleted\ttrue")
	u.Out().Printf("email\t%s", email)
	u.Out().Printf("path\t%s", path)
	return nil
}

type AuthServiceAccountStatusCmd struct {
	Email string `arg:"" name:"email" help:"Email (impersonated user)" required:""`
}

func (c *AuthServiceAccountStatusCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)

	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("empty email")
	}

	path, err := config.ServiceAccountPath(email)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path) //nolint:gosec // stored in user config dir
	if err != nil {
		if os.IsNotExist(err) {
			if outfmt.IsJSON(ctx) {
				return outfmt.WriteJSON(os.Stdout, map[string]any{
					"email":   email,
					"path":    path,
					"exists":  false,
					"stored":  false,
					"message": "no service account configured",
				})
			}
			u.Out().Printf("email\t%s", email)
			u.Out().Printf("path\t%s", path)
			u.Out().Printf("exists\tfalse")
			return nil
		}
		return fmt.Errorf("read service account: %w", err)
	}

	info, parseErr := parseServiceAccountJSON(data)
	if parseErr != nil {
		return parseErr
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"email":        email,
			"path":         path,
			"exists":       true,
			"stored":       true,
			"client_email": info.ClientEmail,
			"client_id":    info.ClientID,
		})
	}
	u.Out().Printf("email\t%s", email)
	u.Out().Printf("path\t%s", path)
	u.Out().Printf("exists\ttrue")
	if info.ClientEmail != "" {
		u.Out().Printf("client_email\t%s", info.ClientEmail)
	}
	if info.ClientID != "" {
		u.Out().Printf("client_id\t%s", info.ClientID)
	}
	return nil
}
