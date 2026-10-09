package businessprofile

// Reviews are served only by the legacy Google My Business v4 API, which has
// no generated Go client and no discovery scopes. Method and scope reference
// verified 2026-10-09:
//
//   - accounts.locations.reviews.list requires
//     https://www.googleapis.com/auth/business.manage:
//     https://developers.google.com/my-business/reference/rest/v4/accounts.locations.reviews/list
//
// The operation is read-only: one GET, one bounded page. No reply, update or
// delete method is implemented here.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	gapi "google.golang.org/api/googleapi"

	nativegoogleapi "github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

const (
	// reviewsMaxPageSize is the documented maximum page size for reviews.list.
	reviewsMaxPageSize = 50
	// reviewsMaxResponseBytes bounds one page of reviews (50 reviews with
	// comments and replies stay far below this).
	reviewsMaxResponseBytes = 2 << 20
	// reviewsOrderBy returns the most recently updated reviews first.
	reviewsOrderBy = "updateTime desc"
)

// reviewsEndpoint is the fixed v4 base; tests redirect at the transport layer.
const reviewsEndpoint = "https://mybusiness.googleapis.com/v4/"

type listReviewsInput struct {
	mcpcontract.Selection
	Parent    string `json:"parent" jsonschema:"Exact opaque parent account resource name returned by businessprofile_list_accounts, in accounts/{id} form"`
	Location  string `json:"location" jsonschema:"Exact opaque location resource name returned by businessprofile_list_locations, in locations/{id} form"`
	PageSize  int64  `json:"page_size,omitempty" jsonschema:"Maximum reviews in the single returned page; default and maximum 50"`
	PageToken string `json:"page_token,omitempty" jsonschema:"Opaque next_page_token from a previous businessprofile_list_reviews result"`
}

// ReviewReply is the owner's public reply to a review, when one exists.
type ReviewReply struct {
	Comment    string `json:"comment,omitempty"`
	UpdateTime string `json:"update_time,omitempty"`
}

// Review is the read-only projection of one v4 review. Reviewer photo URLs
// are deliberately omitted. ReviewReply carries the same reply as the flat
// reply_* fields in the nested shape deployed callers already read.
type Review struct {
	Name                string       `json:"name"`
	ReviewID            string       `json:"review_id,omitempty"`
	ReviewerDisplayName string       `json:"reviewer_display_name,omitempty"`
	ReviewerIsAnonymous bool         `json:"reviewer_is_anonymous,omitempty"`
	StarRating          string       `json:"star_rating,omitempty"`
	Comment             string       `json:"comment,omitempty"`
	CreateTime          string       `json:"create_time,omitempty"`
	UpdateTime          string       `json:"update_time,omitempty"`
	ReplyPresent        bool         `json:"reply_present"`
	ReplyComment        string       `json:"reply_comment,omitempty"`
	ReplyUpdateTime     string       `json:"reply_update_time,omitempty"`
	ReviewReply         *ReviewReply `json:"review_reply,omitempty"`
}

type ReviewsData struct {
	Parent           string   `json:"parent"`
	Location         string   `json:"location"`
	AverageRating    float64  `json:"average_rating"`
	TotalReviewCount int64    `json:"total_review_count"`
	Reviews          []Review `json:"reviews"`
}

type v4ReviewsResponse struct {
	Reviews []*struct {
		Name     string `json:"name"`
		ReviewID string `json:"reviewId"`
		Reviewer *struct {
			DisplayName string `json:"displayName"`
			IsAnonymous bool   `json:"isAnonymous"`
		} `json:"reviewer"`
		StarRating  string `json:"starRating"`
		Comment     string `json:"comment"`
		CreateTime  string `json:"createTime"`
		UpdateTime  string `json:"updateTime"`
		ReviewReply *struct {
			Comment    string `json:"comment"`
			UpdateTime string `json:"updateTime"`
		} `json:"reviewReply"`
	} `json:"reviews"`
	AverageRating    float64 `json:"averageRating"`
	TotalReviewCount int64   `json:"totalReviewCount"`
	NextPageToken    string  `json:"nextPageToken"`
}

var errReviewsOversize = errors.New("reviews response exceeded the bound")

func reviewsOperation(provider mcpcontract.ClientProvider) mcpcontract.Operation {
	return mcpcontract.NewOperation("businessprofile_list_reviews", validateListReviews, func(ctx context.Context, id mcpcontract.Identity, in listReviewsInput) (mcpcontract.Result[ReviewsData], error) {
		httpClient, err := provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: "businessprofile_list_reviews", Retry: mcpcontract.SafeRead})
		if err != nil {
			return mcpcontract.Result[ReviewsData]{}, nativegoogleapi.NativePublicError(err)
		}

		pageSize := in.PageSize
		if pageSize == 0 {
			pageSize = reviewsMaxPageSize
		}

		query := url.Values{}
		query.Set("pageSize", fmt.Sprint(pageSize))
		query.Set("orderBy", reviewsOrderBy)

		if in.PageToken != "" {
			query.Set("pageToken", in.PageToken)
		}

		// Parent and location were validated as exact single-segment names.
		target := reviewsEndpoint + in.Parent + "/" + in.Location + "/reviews?" + query.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return mcpcontract.Result[ReviewsData]{}, nativegoogleapi.NativePublicError(err)
		}

		req.Header.Set("Accept", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			return mcpcontract.Result[ReviewsData]{}, nativegoogleapi.NativePublicError(err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(io.LimitReader(resp.Body, reviewsMaxResponseBytes+1))
		if err != nil {
			return mcpcontract.Result[ReviewsData]{}, nativegoogleapi.NativePublicError(err)
		}

		if checkErr := gapi.CheckResponseWithBody(resp, body); checkErr != nil {
			return mcpcontract.Result[ReviewsData]{}, nativegoogleapi.NativePublicError(checkErr)
		}

		if len(body) > reviewsMaxResponseBytes {
			return mcpcontract.Result[ReviewsData]{}, &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: errReviewsOversize.Error()}
		}

		var decoded v4ReviewsResponse
		if err := json.Unmarshal(body, &decoded); err != nil {
			return mcpcontract.Result[ReviewsData]{}, &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: "Google returned an unreadable reviews page"}
		}

		out := mcpcontract.NewResult(id, ReviewsData{Parent: in.Parent, Location: in.Location, AverageRating: decoded.AverageRating, TotalReviewCount: decoded.TotalReviewCount, Reviews: []Review{}})
		for _, review := range decoded.Reviews {
			if review == nil {
				continue
			}

			projected := Review{
				Name: review.Name, ReviewID: review.ReviewID, StarRating: review.StarRating, Comment: review.Comment,
				CreateTime: review.CreateTime, UpdateTime: review.UpdateTime,
			}
			if review.Reviewer != nil {
				projected.ReviewerDisplayName = review.Reviewer.DisplayName
				projected.ReviewerIsAnonymous = review.Reviewer.IsAnonymous
			}

			if review.ReviewReply != nil {
				projected.ReplyPresent = true
				projected.ReplyComment = review.ReviewReply.Comment
				projected.ReplyUpdateTime = review.ReviewReply.UpdateTime
				projected.ReviewReply = &ReviewReply{Comment: review.ReviewReply.Comment, UpdateTime: review.ReviewReply.UpdateTime}
			}

			out.Data.Reviews = append(out.Data.Reviews, projected)
		}
		out.NextPageToken = decoded.NextPageToken

		return out, nil
	})
}

// validateListReviews checks structure only; account and location IDs stay opaque.
func validateListReviews(in listReviewsInput) error {
	if !plainResourceName(in.Parent, "accounts/") {
		return invalid("parent must be the exact accounts/{id} resource name returned by businessprofile_list_accounts")
	}

	if !plainResourceName(in.Location, "locations/") {
		return invalid("location must be the exact locations/{id} resource name returned by businessprofile_list_locations")
	}

	if in.PageSize < 0 || in.PageSize > reviewsMaxPageSize {
		return invalid("page_size must be between 1 and 50 when supplied")
	}

	return nil
}

// plainResourceName is stricter than exactResourceName: the ID segment may only
// hold ASCII letters, digits, '-' and '_'. That excludes ':' custom-method
// suffixes (for example :updateReply), so the composed v4 URL can only ever
// name the reviews collection.
func plainResourceName(value, prefix string) bool {
	rest, ok := strings.CutPrefix(value, prefix)
	if !ok || rest == "" {
		return false
	}

	for _, r := range rest {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}

	return true
}
