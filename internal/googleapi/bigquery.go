package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/bigquery/v2"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewBigquery(ctx context.Context, email string) (*bigquery.Service, error) {
	if opts, err := optionsForAccount(ctx, googleauth.ServiceBigquery, email); err != nil {
		return nil, fmt.Errorf("bigquery options: %w", err)
	} else if svc, err := bigquery.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create bigquery service: %w", err)
	} else {
		return svc, nil
	}
}
