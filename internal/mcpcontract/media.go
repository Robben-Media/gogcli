package mcpcontract

import (
	"context"
	"time"
)

// MediaReference keeps binary payloads out of ordinary tool-result context.
// It describes temporary content, not a public URL or a persistent file.
type MediaReference struct {
	URI       string    `json:"uri"`
	Name      string    `json:"name,omitempty"`
	MIMEType  string    `json:"mime_type"`
	SizeBytes int       `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
	ExpiresAt time.Time `json:"expires_at"`
}

// MediaArtifacts stores a bounded copy of a successful read. The operation name
// binds subsequent resource retrieval to the original authorization contract.
type MediaArtifacts interface {
	Put(ctx context.Context, identity Identity, operation, name, mimeType string, data []byte) (MediaReference, error)
}
