package mediaartifact

import (
	"bytes"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func mediaIdentity() mcpcontract.Identity {
	return mcpcontract.Identity{AccountID: "a", Subject: "sub", PrincipalID: "p", Email: "a@example.test", ClientName: "app", AuthMode: "oauth", Generation: 1, Scopes: []string{mcpcontract.GmailReadScope}}
}

func TestStoreIsolationCopiesAndExpiry(t *testing.T) {
	t.Parallel()
	s := New()
	t.Cleanup(s.Close)
	now := time.Now()
	s.now = func() time.Time { return now }
	id := mediaIdentity()
	input := []byte("private")

	ref, err := s.Put(t.Context(), id, "gmail_get_attachment", "", "", input)
	if err != nil {
		t.Fatal(err)
	}
	input[0] = 'x'
	id.Scopes[0] = "changed"

	bound, ok := s.Lookup(ref.URI)
	if !ok || bound.Identity.Scopes[0] != mcpcontract.GmailReadScope {
		t.Fatal("identity not cloned")
	}
	bound.Identity.Scopes[0] = "changed again"
	id.Scopes[0] = mcpcontract.GmailReadScope

	got, _, err := s.Read(t.Context(), ref.URI, id)
	if err != nil || string(got) != "private" {
		t.Fatalf("read: %q %v", got, err)
	}
	got[0] = 'x'

	for _, mutate := range []func(*mcpcontract.Identity){
		func(i *mcpcontract.Identity) { i.AccountID = "b" },
		func(i *mcpcontract.Identity) { i.Subject = "other" },
		func(i *mcpcontract.Identity) { i.PrincipalID = "other" },
		func(i *mcpcontract.Identity) { i.Email = "other@example.test" },
		func(i *mcpcontract.Identity) { i.ClientName = "other" },
		func(i *mcpcontract.Identity) { i.Generation++ },
		func(i *mcpcontract.Identity) { i.AuthMode = "service_account" },
		func(i *mcpcontract.Identity) { i.Scopes = []string{mcpcontract.GmailModifyScope} },
	} {
		changed := id.Clone()
		mutate(&changed)

		if _, _, readErr := s.Read(t.Context(), ref.URI, changed); readErr == nil {
			t.Fatal("identity boundary bypass")
		}
	}

	got, _, err = s.Read(t.Context(), ref.URI, id)
	if err != nil || string(got) != "private" {
		t.Fatal("read data not cloned")
	}
	now = now.Add(lifetime)

	if _, ok := s.Lookup(ref.URI); ok {
		t.Fatal("expired reference available")
	}

	if s.bytes != 0 || len(s.entries) != 0 {
		t.Fatal("expired data retained")
	}
}

func TestStoreBounds(t *testing.T) {
	t.Parallel()
	s := New()
	t.Cleanup(s.Close)

	id := mediaIdentity()
	if _, err := s.Put(t.Context(), id, "gmail_get_attachment", "", "", make([]byte, MaxItemBytes+1)); err == nil {
		t.Fatal("oversize accepted")
	}

	if _, err := s.Put(t.Context(), id, "drive_create_file", "", "", nil); err == nil {
		t.Fatal("write binding accepted")
	}

	for range maxEntries {
		if _, err := s.Put(t.Context(), id, "gmail_get_attachment", "", "", nil); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := s.Put(t.Context(), id, "gmail_get_attachment", "", "", nil); err == nil {
		t.Fatal("entry limit bypass")
	}
	s = New()
	t.Cleanup(s.Close)

	data := bytes.Repeat([]byte("x"), MaxItemBytes)
	for range maxBytes/MaxItemBytes - 1 {
		if _, err := s.Put(t.Context(), id, "gmail_get_attachment", "", "", data); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := s.Put(t.Context(), id, "gmail_get_attachment", "", "", data); err == nil {
		t.Fatal("byte limit bypass")
	}
}

func TestStoreExpiresWhileIdle(t *testing.T) {
	t.Parallel()
	s := New()
	t.Cleanup(s.Close)
	s.ttl = 10 * time.Millisecond

	ref, err := s.Put(t.Context(), mediaIdentity(), "gmail_get_attachment", "", "", []byte("private"))
	if err != nil {
		t.Fatal(err)
	}

	s.mu.Lock()
	raw := s.entries[ref.URI].data
	s.mu.Unlock()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		empty := len(s.entries) == 0
		zeroed := bytes.Equal(raw, make([]byte, len(raw)))
		s.mu.Unlock()

		if empty && zeroed {
			return
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatal("idle expired data retained")
}

func TestStoreMetadataAndClose(t *testing.T) {
	t.Parallel()
	s := New()
	t.Cleanup(s.Close)
	id := mediaIdentity()

	id.Label = string(make([]byte, maxMetadataBytes+1))
	if _, err := s.Put(t.Context(), id, "gmail_get_attachment", "", "", nil); err == nil {
		t.Fatal("unbounded identity accepted")
	}
	id = mediaIdentity()

	id.Scopes = make([]string, maxIdentityScopes+1)
	if _, err := s.Put(t.Context(), id, "gmail_get_attachment", "", "", nil); err == nil {
		t.Fatal("unbounded scope list accepted")
	}

	ref, err := s.Put(t.Context(), mediaIdentity(), "gmail_get_attachment", "", "", []byte("private"))
	if err != nil {
		t.Fatal(err)
	}

	s.Close()

	if _, ok := s.Lookup(ref.URI); ok {
		t.Fatal("closed store retains reference")
	}

	if _, err := s.Put(t.Context(), mediaIdentity(), "gmail_get_attachment", "", "", nil); err == nil {
		t.Fatal("closed store accepted data")
	}
}
