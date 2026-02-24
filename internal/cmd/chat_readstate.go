package cmd

import (
	"context"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/chat/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// ChatNotificationSettingsCmd contains subcommands for space notification settings.
type ChatNotificationSettingsCmd struct {
	Get   ChatNotificationSettingsGetCmd   `cmd:"" name:"get" help:"Get notification settings for a space"`
	Patch ChatNotificationSettingsPatchCmd `cmd:"" name:"patch" help:"Update notification settings"`
}

// ChatThreadReadStateCmd contains subcommands for thread read state.
type ChatThreadReadStateCmd struct {
	Get ChatThreadReadStateGetCmd `cmd:"" name:"get" help:"Get thread read state"`
}

// ChatSpaceReadStateCmd contains subcommands for space read state.
type ChatSpaceReadStateCmd struct {
	Update ChatSpaceReadStateUpdateCmd `cmd:"" name:"update" help:"Update space read state"`
}

// ChatNotificationSettingsGetCmd gets notification settings for a space.
type ChatNotificationSettingsGetCmd struct {
	Name string `arg:"" name:"name" help:"Notification setting resource name (users/me/spaces/.../spaceNotificationSetting)"`
}

func (c *ChatNotificationSettingsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("required: name")
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Users.Spaces.SpaceNotificationSetting.Get(name).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"notificationSetting": resp})
	}

	if resp.Name != "" {
		u.Out().Printf("name\t%s", resp.Name)
	}
	if resp.MuteSetting != "" {
		u.Out().Printf("muteSetting\t%s", resp.MuteSetting)
	}
	return nil
}

// ChatNotificationSettingsPatchCmd updates notification settings for a space.
type ChatNotificationSettingsPatchCmd struct {
	Name        string `arg:"" name:"name" help:"Notification setting resource name (users/me/spaces/.../spaceNotificationSetting)"`
	MuteSetting string `name:"mute-setting" help:"Mute setting (MUTE or UNMUTE)"`
}

func (c *ChatNotificationSettingsPatchCmd) Run(ctx context.Context, flags *RootFlags, kctx *kong.Context) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("required: name")
	}

	// Build update mask from provided flags
	var fields []string
	setting := &chat.SpaceNotificationSetting{}

	if flagProvided(kctx, "mute-setting") {
		fields = append(fields, "muteSetting")
		setting.MuteSetting = c.MuteSetting
	}

	if len(fields) == 0 {
		return usage("at least one field must be provided to update")
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	mask := strings.Join(fields, ",")
	resp, err := svc.Users.Spaces.SpaceNotificationSetting.Patch(name, setting).UpdateMask(mask).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"notificationSetting": resp})
	}

	if resp.Name != "" {
		u.Out().Printf("name\t%s", resp.Name)
	}
	if resp.MuteSetting != "" {
		u.Out().Printf("muteSetting\t%s", resp.MuteSetting)
	}
	return nil
}

// ChatThreadReadStateGetCmd gets the thread read state.
type ChatThreadReadStateGetCmd struct {
	Name string `arg:"" name:"name" help:"Thread read state resource name (users/me/spaces/.../threads/.../threadReadState)"`
}

func (c *ChatThreadReadStateGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("required: name")
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Users.Spaces.Threads.GetThreadReadState(name).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"threadReadState": resp})
	}

	if resp.Name != "" {
		u.Out().Printf("name\t%s", resp.Name)
	}
	if resp.LastReadTime != "" {
		u.Out().Printf("lastReadTime\t%s", resp.LastReadTime)
	}
	return nil
}

// ChatSpaceReadStateUpdateCmd updates the space read state.
type ChatSpaceReadStateUpdateCmd struct {
	Name         string `arg:"" name:"name" help:"Space read state resource name (users/me/spaces/.../spaceReadState)"`
	LastReadTime string `name:"last-read-time" help:"RFC3339 timestamp of last read time"`
}

func (c *ChatSpaceReadStateUpdateCmd) Run(ctx context.Context, flags *RootFlags, kctx *kong.Context) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("required: name")
	}

	// Build update mask from provided flags
	var fields []string
	readState := &chat.SpaceReadState{}

	if flagProvided(kctx, "last-read-time") {
		fields = append(fields, "lastReadTime")
		readState.LastReadTime = c.LastReadTime
	}

	if len(fields) == 0 {
		return usage("at least one field must be provided to update")
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	mask := strings.Join(fields, ",")
	resp, err := svc.Users.Spaces.UpdateSpaceReadState(name, readState).UpdateMask(mask).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"spaceReadState": resp})
	}

	if resp.Name != "" {
		u.Out().Printf("name\t%s", resp.Name)
	}
	if resp.LastReadTime != "" {
		u.Out().Printf("lastReadTime\t%s", resp.LastReadTime)
	}
	return nil
}
