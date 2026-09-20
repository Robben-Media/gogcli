package main

import (
	"fmt"
	"time"

	"github.com/steipete/gogcli/internal/accountconnect"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleapi"
)

func newClientProvider(registry accountconnect.Registry, tokens accountconnect.RefreshTokenStore, lifecycle *accountconnect.Lifecycle, timeout time.Duration, maxBytes int64, concurrency int) (*googleapi.NativeProvider, error) {
	provider, err := googleapi.NewNativeProvider(googleapi.NativeOptions{
		Registry:  registry,
		Tokens:    tokens,
		Lifecycle: lifecycle,
		Credentials: func(clientName string) (string, string, error) {
			creds, err := config.ReadClientCredentialsFor(clientName)
			if err != nil {
				return "", "", fmt.Errorf("read OAuth client credentials: %w", err)
			}

			return creds.ClientID, creds.ClientSecret, nil
		},
		MaxConcurrency:   concurrency,
		MaxResponseBytes: maxBytes,
		RequestTimeout:   timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("native google provider: %w", err)
	}

	return provider, nil
}
