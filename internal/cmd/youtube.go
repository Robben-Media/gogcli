package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/youtube/v3"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newYoutubeService = googleapi.NewYoutube

type YoutubeCmd struct {
	Channels      YoutubeChannelsCmd      `cmd:"" name:"channels" group:"Read" help:"List channels"`
	Videos        YoutubeVideosCmd        `cmd:"" name:"videos" group:"Read" help:"List videos for a channel"`
	Video         YoutubeVideoCmd         `cmd:"" name:"video" group:"Read" help:"Get video details"`
	Search        YoutubeSearchCmd        `cmd:"" name:"search" group:"Read" help:"Search YouTube"`
	Playlists     YoutubePlaylistsCmd     `cmd:"" name:"playlists" group:"Read" help:"List playlists"`
	PlaylistItems YoutubePlaylistItemsCmd `cmd:"" name:"playlist-items" group:"Read" help:"List items in a playlist"`
	Comments      YoutubeCommentsCmd      `cmd:"" name:"comments" group:"Read" help:"List comments for a video"`
}

// --- channels ---

type YoutubeChannelsCmd struct {
	Mine bool   `name:"mine" help:"List the authenticated user's channel(s)"`
	ID   string `name:"id" help:"Channel ID to look up"`
	Max  int64  `name:"max" aliases:"limit" help:"Max results" default:"5"`
}

func (c *YoutubeChannelsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if !c.Mine && c.ID == "" {
		return usage("specify --mine or --id")
	}

	svc, err := newYoutubeService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Channels.List([]string{"snippet", "statistics"}).MaxResults(c.Max)
	if c.Mine {
		call = call.Mine(true)
	}
	if c.ID != "" {
		call = call.Id(c.ID)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"channels":      resp.Items,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Items) == 0 {
		u.Err().Println("No channels")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tTITLE\tSUBSCRIBERS\tVIDEOS")
	for _, ch := range resp.Items {
		title := ""
		if ch.Snippet != nil {
			title = ch.Snippet.Title
		}
		subs := uint64(0)
		vids := uint64(0)
		if ch.Statistics != nil {
			subs = ch.Statistics.SubscriberCount
			vids = ch.Statistics.VideoCount
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\n", ch.Id, title, subs, vids)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// --- videos (list by channel) ---

type YoutubeVideosCmd struct {
	ChannelID string `name:"channel-id" required:"" help:"Channel ID"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"10"`
	Order     string `name:"order" help:"Sort order: date, viewCount, rating" default:"date"`
	Page      string `name:"page" help:"Page token"`
}

func (c *YoutubeVideosCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	channelID := strings.TrimSpace(c.ChannelID)
	if channelID == "" {
		return usage("--channel-id required")
	}

	svc, err := newYoutubeService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Search.List([]string{"snippet"}).
		ChannelId(channelID).
		Type("video").
		Order(c.Order).
		MaxResults(c.Max).
		PageToken(c.Page)

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"videos":        resp.Items,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Items) == 0 {
		u.Err().Println("No videos")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "VIDEO_ID\tTITLE\tPUBLISHED")
	for _, item := range resp.Items {
		videoID := ""
		if item.Id != nil {
			videoID = item.Id.VideoId
		}
		title := ""
		published := ""
		if item.Snippet != nil {
			title = item.Snippet.Title
			published = item.Snippet.PublishedAt
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", videoID, title, published)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// --- video (get single) ---

type YoutubeVideoCmd struct {
	VideoID string `arg:"" name:"videoId" help:"Video ID"`
}

func (c *YoutubeVideoCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	videoID := strings.TrimSpace(c.VideoID)
	if videoID == "" {
		return usage("videoId required")
	}

	svc, err := newYoutubeService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Videos.List([]string{"snippet", "statistics", "contentDetails"}).Id(videoID).Do()
	if err != nil {
		return err
	}

	if len(resp.Items) == 0 {
		return fmt.Errorf("video %q not found", videoID)
	}

	video := resp.Items[0]
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"video": video})
	}

	u.Out().Printf("id\t%s", video.Id)
	if video.Snippet != nil {
		u.Out().Printf("title\t%s", video.Snippet.Title)
		u.Out().Printf("channel\t%s", video.Snippet.ChannelTitle)
		u.Out().Printf("published\t%s", video.Snippet.PublishedAt)
		u.Out().Printf("description\t%s", video.Snippet.Description)
	}
	if video.Statistics != nil {
		u.Out().Printf("views\t%d", video.Statistics.ViewCount)
		u.Out().Printf("likes\t%d", video.Statistics.LikeCount)
		u.Out().Printf("comments\t%d", video.Statistics.CommentCount)
	}
	if video.ContentDetails != nil {
		u.Out().Printf("duration\t%s", video.ContentDetails.Duration)
	}
	return nil
}

// --- search ---

type YoutubeSearchCmd struct {
	Query []string `arg:"" name:"query" help:"Search query"`
	Type  string   `name:"type" help:"Type filter: video, channel, playlist" default:"video"`
	Max   int64    `name:"max" aliases:"limit" help:"Max results" default:"10"`
	Page  string   `name:"page" help:"Page token"`
}

func (c *YoutubeSearchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	query := strings.TrimSpace(strings.Join(c.Query, " "))
	if query == "" {
		return usage("missing query")
	}

	svc, err := newYoutubeService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Search.List([]string{"snippet"}).
		Q(query).
		Type(c.Type).
		MaxResults(c.Max).
		PageToken(c.Page)

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"results":       resp.Items,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Items) == 0 {
		u.Err().Println("No results")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "KIND\tID\tTITLE\tPUBLISHED")
	for _, item := range resp.Items {
		kind := ""
		id := ""
		if item.Id != nil {
			switch {
			case item.Id.VideoId != "":
				kind = "video"
				id = item.Id.VideoId
			case item.Id.ChannelId != "":
				kind = "channel"
				id = item.Id.ChannelId
			case item.Id.PlaylistId != "":
				kind = "playlist"
				id = item.Id.PlaylistId
			}
		}
		title := ""
		published := ""
		if item.Snippet != nil {
			title = item.Snippet.Title
			published = item.Snippet.PublishedAt
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", kind, id, title, published)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// --- playlists ---

type YoutubePlaylistsCmd struct {
	ChannelID string `name:"channel-id" help:"Channel ID"`
	Mine      bool   `name:"mine" help:"List the authenticated user's playlists"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"25"`
	Page      string `name:"page" help:"Page token"`
}

func (c *YoutubePlaylistsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if !c.Mine && c.ChannelID == "" {
		return usage("specify --mine or --channel-id")
	}

	svc, err := newYoutubeService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Playlists.List([]string{"snippet", "contentDetails"}).
		MaxResults(c.Max).
		PageToken(c.Page)
	if c.Mine {
		call = call.Mine(true)
	}
	if c.ChannelID != "" {
		call = call.ChannelId(c.ChannelID)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"playlists":     resp.Items,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Items) == 0 {
		u.Err().Println("No playlists")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tTITLE\tITEMS")
	for _, pl := range resp.Items {
		title := ""
		if pl.Snippet != nil {
			title = pl.Snippet.Title
		}
		count := int64(0)
		if pl.ContentDetails != nil {
			count = pl.ContentDetails.ItemCount
		}
		fmt.Fprintf(w, "%s\t%s\t%d\n", pl.Id, title, count)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// --- playlist-items ---

type YoutubePlaylistItemsCmd struct {
	PlaylistID string `arg:"" name:"playlistId" help:"Playlist ID"`
	Max        int64  `name:"max" aliases:"limit" help:"Max results" default:"25"`
	Page       string `name:"page" help:"Page token"`
}

func (c *YoutubePlaylistItemsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	playlistID := strings.TrimSpace(c.PlaylistID)
	if playlistID == "" {
		return usage("playlistId required")
	}

	svc, err := newYoutubeService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.PlaylistItems.List([]string{"snippet", "contentDetails"}).
		PlaylistId(playlistID).
		MaxResults(c.Max).
		PageToken(c.Page)

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"items":         resp.Items,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Items) == 0 {
		u.Err().Println("No playlist items")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "POSITION\tVIDEO_ID\tTITLE")
	for _, item := range resp.Items {
		pos := int64(0)
		videoID := ""
		title := ""
		if item.Snippet != nil {
			pos = item.Snippet.Position
			title = item.Snippet.Title
			if item.Snippet.ResourceId != nil {
				videoID = item.Snippet.ResourceId.VideoId
			}
		}
		fmt.Fprintf(w, "%d\t%s\t%s\n", pos, videoID, title)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// --- comments ---

type YoutubeCommentsCmd struct {
	VideoID string `arg:"" name:"videoId" help:"Video ID"`
	Max     int64  `name:"max" aliases:"limit" help:"Max results" default:"20"`
	Page    string `name:"page" help:"Page token"`
}

func (c *YoutubeCommentsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	videoID := strings.TrimSpace(c.VideoID)
	if videoID == "" {
		return usage("videoId required")
	}

	svc, err := newYoutubeService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.CommentThreads.List([]string{"snippet"}).
		VideoId(videoID).
		MaxResults(c.Max).
		PageToken(c.Page).
		TextFormat("plainText")

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"commentThreads": resp.Items,
			"nextPageToken":  resp.NextPageToken,
		})
	}

	if len(resp.Items) == 0 {
		u.Err().Println("No comments")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "AUTHOR\tCOMMENT\tLIKES\tPUBLISHED")
	for _, thread := range resp.Items {
		if thread.Snippet == nil || thread.Snippet.TopLevelComment == nil || thread.Snippet.TopLevelComment.Snippet == nil {
			continue
		}
		cs := thread.Snippet.TopLevelComment.Snippet
		text := cs.TextDisplay
		if len(text) > 80 {
			text = text[:80] + "..."
		}
		text = strings.ReplaceAll(text, "\n", " ")
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", cs.AuthorDisplayName, text, cs.LikeCount, cs.PublishedAt)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// Ensure youtube.Service is used to avoid import cycle lint errors.
var _ *youtube.Service
