package services

import (
	"os"
	"testing"

	"github.com/adiecho/oci-panel/internal/models"
)

func TestClientCacheTracksCredentialChanges(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.Mkdir("keys", 0700); err != nil {
		t.Fatal(err)
	}
	// Configuration providers read the key here; RSA parsing happens only when
	// creating an SDK client. No OCI request is made by this test.
	if err := os.WriteFile("keys/old.pem", []byte("old test key"), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewOCIService(nil)
	user := models.OciUser{ID: "config", OciRegion: "region", OciKeyPath: "old.pem"}
	oldClients, err := s.getCachedClients(&user)
	if err != nil {
		t.Fatal(err)
	}
	s.InvalidateClientCache(user.ID)
	// An old request may refill its cache entry after invalidation.
	if _, err := s.getCachedClients(&user); err != nil {
		t.Fatal(err)
	}
	updated := user
	updated.OciKeyPath = "new.pem"
	if _, err := s.getCachedClients(&updated); err == nil {
		t.Fatal("updated credentials reused the old key instead of reading the new file")
	}
	if err := os.WriteFile("keys/new.pem", []byte("new test key"), 0600); err != nil {
		t.Fatal(err)
	}
	newClients, err := s.getCachedClients(&updated)
	if err != nil || newClients == oldClients {
		t.Fatalf("credential revision did not create a new provider: %v", err)
	}
	again, err := s.getCachedClients(&updated)
	if err != nil || again != newClients {
		t.Fatalf("unchanged credentials did not reuse their provider: %v", err)
	}
	s.InvalidateClientCache(user.ID)
	entries := 0
	s.clientCache.Range(func(_, _ any) bool { entries++; return true })
	if entries != 0 {
		t.Fatalf("invalidation left %d credential revisions", entries)
	}
}
