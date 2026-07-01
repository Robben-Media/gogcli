package cmd

import (
	"context"

	"google.golang.org/api/chat/v1"

	"github.com/Robben-Media/gogcli/internal/googleapi"
)

var newChatService func(ctx context.Context, email string) (*chat.Service, error) = googleapi.NewChat
