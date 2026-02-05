package googleapi

import (
	"context"
	"fmt"

	mybusinessaccountmanagement "google.golang.org/api/mybusinessaccountmanagement/v1"
	mybusinessbusinessinformation "google.golang.org/api/mybusinessbusinessinformation/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewBusinessProfileInfo(ctx context.Context, email string) (*mybusinessbusinessinformation.Service, error) {
	if opts, err := optionsForAccount(ctx, googleauth.ServiceBusinessProfile, email); err != nil {
		return nil, fmt.Errorf("business profile info options: %w", err)
	} else if svc, err := mybusinessbusinessinformation.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create business profile info service: %w", err)
	} else {
		return svc, nil
	}
}

func NewBusinessProfileAccounts(ctx context.Context, email string) (*mybusinessaccountmanagement.Service, error) {
	if opts, err := optionsForAccount(ctx, googleauth.ServiceBusinessProfile, email); err != nil {
		return nil, fmt.Errorf("business profile accounts options: %w", err)
	} else if svc, err := mybusinessaccountmanagement.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create business profile accounts service: %w", err)
	} else {
		return svc, nil
	}
}
