package googleapi

import (
	"context"
	"fmt"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
	analyticsdata "google.golang.org/api/analyticsdata/v1beta"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewAnalyticsData(ctx context.Context, email string) (*analyticsdata.Service, error) {
	if opts, err := optionsForAccount(ctx, googleauth.ServiceAnalytics, email); err != nil {
		return nil, fmt.Errorf("analytics data options: %w", err)
	} else if svc, err := analyticsdata.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create analytics data service: %w", err)
	} else {
		return svc, nil
	}
}

func NewAnalyticsAdmin(ctx context.Context, email string) (*analyticsadmin.Service, error) {
	if opts, err := optionsForAccount(ctx, googleauth.ServiceAnalytics, email); err != nil {
		return nil, fmt.Errorf("analytics admin options: %w", err)
	} else if svc, err := analyticsadmin.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create analytics admin service: %w", err)
	} else {
		return svc, nil
	}
}
