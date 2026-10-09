package businessprofile

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func TestBusinessProfileListReviewsProjectsOneBoundedReadOnlyPage(t *testing.T) {
	t.Parallel()
	operation, provider, recorder := fixture(t, "businessprofile_list_reviews", http.StatusOK, `{
		"reviews": [
			{"name": "accounts/1/locations/55/reviews/r1", "reviewId": "r1", "reviewer": {"displayName": "Pat", "profilePhotoUrl": "https://example.test/p.png"},
			 "starRating": "FIVE", "comment": "Great", "createTime": "2026-10-09T12:44:32Z", "updateTime": "2026-10-09T12:44:32Z",
			 "reviewReply": {"comment": "Thanks Pat", "updateTime": "2026-10-09T13:00:00Z"}},
			{"name": "accounts/1/locations/55/reviews/r2", "reviewId": "r2", "reviewer": {"isAnonymous": true}, "starRating": "THREE",
			 "createTime": "2026-10-01T00:00:00Z", "updateTime": "2026-10-02T00:00:00Z"},
			null
		],
		"averageRating": 4.8, "totalReviewCount": 777, "nextPageToken": "rev-tok-2"
	}`)

	result, err := decodeRun(t, operation, `{"account_id":"opaque-account","parent":"accounts/1","location":"locations/55","page_token":"rev-tok-1"}`, testIdentity("opaque-account"))
	if err != nil {
		t.Fatal(err)
	}

	out := result.(mcpcontract.Result[ReviewsData])
	if out.Data.TotalReviewCount != 777 || out.Data.AverageRating != 4.8 || len(out.Data.Reviews) != 2 || out.NextPageToken != "rev-tok-2" {
		t.Fatalf("unexpected projection: %#v next=%q", out.Data, out.NextPageToken)
	}

	first, second := out.Data.Reviews[0], out.Data.Reviews[1]
	if first.ReviewerDisplayName != "Pat" || first.StarRating != "FIVE" || first.Comment != "Great" || !first.ReplyPresent || first.ReplyComment != "Thanks Pat" || first.CreateTime == "" {
		t.Fatalf("unexpected first review: %#v", first)
	}

	if second.ReplyPresent || second.ReplyComment != "" || !second.ReviewerIsAnonymous {
		t.Fatalf("unexpected second review: %#v", second)
	}

	if recorder.calls != 1 || recorder.path != "/v4/accounts/1/locations/55/reviews" {
		t.Fatalf("calls=%d path=%q", recorder.calls, recorder.path)
	}

	if recorder.query.Get("pageSize") != "50" || recorder.query.Get("orderBy") != "updateTime desc" || recorder.query.Get("pageToken") != "rev-tok-1" {
		t.Fatalf("unexpected query: %v", recorder.query)
	}

	want := mcpcontract.CallOptions{Operation: "businessprofile_list_reviews", Retry: mcpcontract.SafeRead}
	if provider.options[0] != want {
		t.Fatalf("unexpected call options: %#v", provider.options[0])
	}
}

func TestBusinessProfileListReviewsRejectsInexactNamesBeforeUpstream(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`{"account_id":"a","parent":"accounts/1","location":"locations/55/reviews/x"}`,
		`{"account_id":"a","parent":"accounts/1/locations/2","location":"locations/55"}`,
		`{"account_id":"a","parent":"accounts/1","location":"locations/..%2F"}`,
		`{"account_id":"a","parent":"accounts/1","location":"locations/55?x=1"}`,
		`{"account_id":"a","parent":"accounts/1","location":"locations/55","page_size":51}`,
		`{"account_id":"a","parent":"accounts/1","location":"locations/55","page_size":-1}`,
		`{"account_id":"a","location":"locations/55"}`,
		`{"account_id":"a","parent":"accounts/1","location":"locations/55","reply":"hi"}`,
		`{"account_id":"a","parent":"accounts/1","location":"locations/2:updateReply"}`,
		`{"account_id":"a","parent":"accounts/1:batchGetReviews","location":"locations/2"}`,
		`{"account_id":"a","parent":"accounts/1","location":"locations/2;x"}`,
		`{"account_id":"a","parent":"accounts/1","location":"locations/.."}`,
		`{"account_id":"a","parent":" accounts/1","location":"locations/2"}`,
	} {
		operation, _, recorder := fixture(t, "businessprofile_list_reviews", http.StatusOK, `{}`)
		if _, err := decodeRun(t, operation, raw, testIdentity("a")); err == nil {
			t.Fatalf("accepted %s", raw)
		}

		if recorder.calls != 0 {
			t.Fatalf("%s reached upstream", raw)
		}
	}
}

func TestBusinessProfileListReviewsMapsAccessDeniedWithoutLeaking(t *testing.T) {
	t.Parallel()
	const secret = "project-330901745019-detail"
	operation, _, _ := fixture(t, "businessprofile_list_reviews", http.StatusForbidden, `{"error":{"code":403,"message":"`+secret+`","status":"PERMISSION_DENIED"}}`)

	_, err := decodeRun(t, operation, `{"account_id":"a","parent":"accounts/1","location":"locations/55"}`, testIdentity("a"))
	safe := contractError(err)
	if safe == nil || safe.Category != mcpcontract.Forbidden || strings.Contains(safe.Message, secret) {
		t.Fatalf("unexpected error: %#v (%v)", safe, err)
	}
}

func TestBusinessProfileListReviewsIsReadOnlyDefinition(t *testing.T) {
	t.Parallel()
	def, ok := mcpcontract.Lookup("businessprofile_list_reviews")
	if !ok || def.Retry != mcpcontract.SafeRead || len(def.Actions) != 1 || def.Actions[0] != "businessprofile:reviews.list" || len(def.Scopes) != 1 || def.Scopes[0] != mcpcontract.BusinessManageScope {
		t.Fatalf("unexpected definition: %#v", def)
	}
}

// TestBusinessProfileListReviewsKeepsDeployedOutputContract pins the data
// fields the deployed google-mcp:16a4495 image returns for Midwest (average
// rating, total count, nested review_reply) so a redeploy from main keeps them.
func TestBusinessProfileListReviewsKeepsDeployedOutputContract(t *testing.T) {
	t.Parallel()
	operation, _, recorder := fixture(t, "businessprofile_list_reviews", http.StatusOK, `{
		"reviews": [
			{"name": "accounts/114569381988673522973/locations/3912355503421668705/reviews/r1", "reviewId": "r1",
			 "reviewer": {"displayName": "Pat Doe", "profilePhotoUrl": "https://photo.example/p"}, "starRating": "FIVE", "comment": "Great work",
			 "createTime": "2026-09-01T10:00:00Z", "updateTime": "2026-09-02T10:00:00Z",
			 "reviewReply": {"comment": "Thanks Pat", "updateTime": "2026-09-03T10:00:00Z"}},
			{"name": "accounts/114569381988673522973/locations/3912355503421668705/reviews/r2", "reviewId": "r2",
			 "reviewer": {"displayName": "Sam"}, "starRating": "FOUR", "createTime": "2026-08-01T10:00:00Z", "updateTime": "2026-08-01T10:00:00Z"}
		],
		"averageRating": 4.5, "totalReviewCount": 44, "nextPageToken": "midwest-tok-2"
	}`)

	result, err := decodeRun(t, operation, `{"account_id":"a","parent":"accounts/114569381988673522973","location":"locations/3912355503421668705"}`, testIdentity("a"))
	if err != nil {
		t.Fatal(err)
	}

	if recorder.calls != 1 || recorder.path != "/v4/accounts/114569381988673522973/locations/3912355503421668705/reviews" || recorder.query.Get("pageSize") != "50" || recorder.query.Get("orderBy") != "updateTime desc" {
		t.Fatalf("calls=%d path=%q query=%v", recorder.calls, recorder.path, recorder.query)
	}

	out := result.(mcpcontract.Result[ReviewsData])
	if out.NextPageToken != "midwest-tok-2" {
		t.Fatalf("next_page_token = %q", out.NextPageToken)
	}

	got, err := json.Marshal(out.Data)
	if err != nil {
		t.Fatal(err)
	}

	// The data object google-mcp:16a4495 emits for this upstream page, plus
	// the reply_present/reply_comment/reply_update_time fields #275 added.
	const want = `{"parent":"accounts/114569381988673522973","location":"locations/3912355503421668705","average_rating":4.5,"total_review_count":44,"reviews":[` +
		`{"name":"accounts/114569381988673522973/locations/3912355503421668705/reviews/r1","review_id":"r1","reviewer_display_name":"Pat Doe","star_rating":"FIVE","comment":"Great work","create_time":"2026-09-01T10:00:00Z","update_time":"2026-09-02T10:00:00Z","reply_present":true,"reply_comment":"Thanks Pat","reply_update_time":"2026-09-03T10:00:00Z","review_reply":{"comment":"Thanks Pat","update_time":"2026-09-03T10:00:00Z"}},` +
		`{"name":"accounts/114569381988673522973/locations/3912355503421668705/reviews/r2","review_id":"r2","reviewer_display_name":"Sam","star_rating":"FOUR","create_time":"2026-08-01T10:00:00Z","update_time":"2026-08-01T10:00:00Z","reply_present":false}]}`
	if string(got) != want {
		t.Fatalf("data contract drifted:\n got %s\nwant %s", got, want)
	}
}

func TestBusinessProfileListReviewsEmitsZeroAggregates(t *testing.T) {
	t.Parallel()
	operation, _, _ := fixture(t, "businessprofile_list_reviews", http.StatusOK, `{}`)

	result, err := decodeRun(t, operation, `{"account_id":"a","parent":"accounts/1","location":"locations/2"}`, testIdentity("a"))
	if err != nil {
		t.Fatal(err)
	}

	got, err := json.Marshal(result.(mcpcontract.Result[ReviewsData]).Data)
	if err != nil {
		t.Fatal(err)
	}

	if want := `{"parent":"accounts/1","location":"locations/2","average_rating":0,"total_review_count":0,"reviews":[]}`; string(got) != want {
		t.Fatalf("got %s want %s", got, want)
	}
}
