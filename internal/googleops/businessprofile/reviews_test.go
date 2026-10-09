package businessprofile

import (
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
