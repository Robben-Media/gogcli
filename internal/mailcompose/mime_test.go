package mailcompose

import (
	"bytes"
	"fmt"
	"mime"
	"mime/multipart"
	"net/mail"
	"testing"
)

// RFC 2387 section 3.1 requires type to match the root part's media type.
func TestRelatedRootMediaType(t *testing.T) {
	t.Parallel()

	for _, format := range []ContentFormat{FormatHTML, FormatPlainAndHTML} {
		for _, regular := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/regular=%t", format, regular), func(t *testing.T) {
				t.Parallel()
				in := validInput()

				in.Content = Content{Format: format, HTML: `<img src="cid:logo@example.com">`}
				if format == FormatPlainAndHTML {
					in.Content.Plain = "logo"
				}

				in.Attachments = []Attachment{{Filename: "logo.png", ContentType: "image/png", Data: []byte("png"), Disposition: "inline", ContentID: "logo@example.com"}}
				if regular {
					in.Attachments = append(in.Attachments, Attachment{Filename: "report.txt", ContentType: "text/plain", Data: []byte("report")})
				}

				out, err := Compose(in)
				if err != nil {
					t.Fatal(err)
				}

				msg, err := mail.ReadMessage(bytes.NewReader(out.Raw))
				if err != nil {
					t.Fatal(err)
				}

				mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
				if err != nil {
					t.Fatal(err)
				}

				body := msg.Body
				if regular {
					part, partErr := multipart.NewReader(body, params["boundary"]).NextPart()
					if partErr != nil {
						t.Fatal(partErr)
					}

					mediaType, params, err = mime.ParseMediaType(part.Header.Get("Content-Type"))
					if err != nil {
						t.Fatal(err)
					}
					body = part
				}

				if mediaType != "multipart/related" {
					t.Fatalf("media type = %q", mediaType)
				}

				part, err := multipart.NewReader(body, params["boundary"]).NextPart()
				if err != nil {
					t.Fatal(err)
				}

				rootType, _, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
				if err != nil {
					t.Fatal(err)
				}

				if params["type"] != rootType {
					t.Fatalf("related type %q differs from root %q", params["type"], rootType)
				}
			})
		}
	}
}
