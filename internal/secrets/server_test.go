package secrets

import (
	"errors"
	"runtime"
	"testing"

	"github.com/99designs/keyring"
)

func TestOpenDefaultNonInteractivePasswordCallbacks(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("isolates config through XDG_CONFIG_HOME on Linux")
	}

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(keyringBackendEnv, "file")
	original := keyringOpenFunc

	t.Cleanup(func() { keyringOpenFunc = original })

	for _, password := range []string{"", "fixture-password"} {
		t.Run(password, func(t *testing.T) {
			t.Setenv(keyringPasswordEnv, password)
			configs := make(chan keyring.Config, 1)
			keyringOpenFunc = func(cfg keyring.Config) (keyring.Keyring, error) {
				configs <- cfg
				return keyring.NewArrayKeyring(nil), nil
			}

			if _, err := OpenDefaultNonInteractive(); err != nil {
				t.Fatal(err)
			}
			cfg := <-configs

			for index, prompt := range []keyring.PromptFunc{cfg.FilePasswordFunc, cfg.KeychainPasswordFunc} {
				got, err := prompt("must not reach terminal")
				if password == "" || index == 1 {
					if !errors.Is(err, errNoTTY) {
						t.Fatalf("missing password: got %v", err)
					}
				} else if err != nil || got != password {
					t.Fatal("configured password was not supplied")
				}
			}
		})
	}
}
