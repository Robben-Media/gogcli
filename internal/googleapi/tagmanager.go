package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/tagmanager/v2"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewTagManager(ctx context.Context, email string) (*tagmanager.Service, error) {
	if opts, err := optionsForAccount(ctx, googleauth.ServiceTagManager, email); err != nil {
		return nil, fmt.Errorf("tag manager options: %w", err)
	} else if svc, err := tagmanager.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create tag manager service: %w", err)
	} else {
		return svc, nil
	}
}
