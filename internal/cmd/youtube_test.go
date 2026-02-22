package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

func TestExecute_YoutubeChannels_JSON(t *testing.T) {
	origNew := newYoutubeService
	t.Cleanup(func() { newYoutubeService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/channels") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id": "UC123",
						"snippet": map[string]any{
							"title": "Test Channel",
						},
						"statistics": map[string]any{
							"subscriberCount": "1000",
							"videoCount":      "50",
						},
					},
				},
				"nextPageToken": "page2",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := youtube.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newYoutubeService = func(context.Context, string) (*youtube.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "youtube", "channels", "--mine"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Channels []struct {
			ID      string `json:"id"`
			Snippet struct {
				Title string `json:"title"`
			} `json:"snippet"`
		} `json:"channels"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Channels) != 1 || parsed.Channels[0].ID != "UC123" {
		t.Fatalf("unexpected channels: %#v", parsed.Channels)
	}
	if parsed.NextPageToken != "page2" {
		t.Fatalf("unexpected nextPageToken: %q", parsed.NextPageToken)
	}
}

func TestExecute_YoutubeSearch_JSON(t *testing.T) {
	origNew := newYoutubeService
	t.Cleanup(func() { newYoutubeService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/search") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id": map[string]any{
							"kind":    "youtube#video",
							"videoId": "vid1",
						},
						"snippet": map[string]any{
							"title":       "Search Result 1",
							"publishedAt": "2026-01-01T00:00:00Z",
						},
					},
					{
						"id": map[string]any{
							"kind":      "youtube#channel",
							"channelId": "ch1",
						},
						"snippet": map[string]any{
							"title":       "Channel Result",
							"publishedAt": "2025-06-15T00:00:00Z",
						},
					},
				},
				"nextPageToken": "",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := youtube.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newYoutubeService = func(context.Context, string) (*youtube.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "youtube", "search", "test query"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Results []struct {
			ID struct {
				VideoID   string `json:"videoId"`
				ChannelID string `json:"channelId"`
			} `json:"id"`
			Snippet struct {
				Title string `json:"title"`
			} `json:"snippet"`
		} `json:"results"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(parsed.Results))
	}
	if parsed.Results[0].ID.VideoID != "vid1" {
		t.Fatalf("unexpected first result videoId: %q", parsed.Results[0].ID.VideoID)
	}
	if parsed.Results[1].ID.ChannelID != "ch1" {
		t.Fatalf("unexpected second result channelId: %q", parsed.Results[1].ID.ChannelID)
	}
}

func TestExecute_YoutubeVideo_JSON(t *testing.T) {
	origNew := newYoutubeService
	t.Cleanup(func() { newYoutubeService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/videos") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id": "vid123",
						"snippet": map[string]any{
							"title":        "My Video",
							"channelTitle": "My Channel",
							"publishedAt":  "2026-01-01T00:00:00Z",
							"description":  "A great video",
						},
						"statistics": map[string]any{
							"viewCount":    "5000",
							"likeCount":    "100",
							"commentCount": "10",
						},
						"contentDetails": map[string]any{
							"duration": "PT10M30S",
						},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := youtube.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newYoutubeService = func(context.Context, string) (*youtube.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "youtube", "video", "vid123"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Video struct {
			ID      string `json:"id"`
			Snippet struct {
				Title string `json:"title"`
			} `json:"snippet"`
		} `json:"video"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Video.ID != "vid123" {
		t.Fatalf("unexpected video id: %q", parsed.Video.ID)
	}
	if parsed.Video.Snippet.Title != "My Video" {
		t.Fatalf("unexpected video title: %q", parsed.Video.Snippet.Title)
	}
}

func TestExecute_YoutubeChannels_NoMineNoID(t *testing.T) {
	err := Execute([]string{"--json", "--account", "a@b.com", "youtube", "channels"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("expected exit code 2, got %v", ExitCode(err))
	}
}

func TestExecute_YoutubeSearch_EmptyQuery(t *testing.T) {
	err := Execute([]string{"--json", "--account", "a@b.com", "youtube", "search", " "})
	if err == nil {
		t.Fatalf("expected error")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("expected exit code 2, got %v", ExitCode(err))
	}
}

func TestExecute_YoutubePlaylists_JSON(t *testing.T) {
	origNew := newYoutubeService
	t.Cleanup(func() { newYoutubeService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/playlists") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id": "PL123",
						"snippet": map[string]any{
							"title": "My Playlist",
						},
						"contentDetails": map[string]any{
							"itemCount": 5,
						},
					},
				},
				"nextPageToken": "",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := youtube.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newYoutubeService = func(context.Context, string) (*youtube.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "youtube", "playlists", "--mine"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Playlists []struct {
			ID string `json:"id"`
		} `json:"playlists"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Playlists) != 1 || parsed.Playlists[0].ID != "PL123" {
		t.Fatalf("unexpected playlists: %#v", parsed.Playlists)
	}
}
