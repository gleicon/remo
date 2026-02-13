package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func generateKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return pub, priv
}

func TestNewAuthorizedKeys(t *testing.T) {
	pub, _ := generateKey(t)
	entries := []Entry{{Key: pub, Rule: "*"}}
	ak := NewAuthorizedKeys(entries)
	got := ak.Entries()
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if string(got[0].Key) != string(pub) {
		t.Fatal("key mismatch")
	}
}

func TestEntriesNilSafe(t *testing.T) {
	var ak *AuthorizedKeys
	if entries := ak.Entries(); entries != nil {
		t.Fatalf("expected nil, got %v", entries)
	}
}

func TestAllowNilAuthorizer(t *testing.T) {
	var ak *AuthorizedKeys
	pub, _ := generateKey(t)
	if !ak.Allow(pub, "anything") {
		t.Fatal("nil authorizer should allow all")
	}
}

func TestAllowWildcardRule(t *testing.T) {
	pub, _ := generateKey(t)
	ak := NewAuthorizedKeys([]Entry{{Key: pub, Rule: "*"}})
	if !ak.Allow(pub, "foo") {
		t.Fatal("wildcard rule should allow any subdomain")
	}
	if !ak.Allow(pub, "bar") {
		t.Fatal("wildcard rule should allow any subdomain")
	}
}

func TestAllowEmptyRule(t *testing.T) {
	pub, _ := generateKey(t)
	ak := NewAuthorizedKeys([]Entry{{Key: pub, Rule: ""}})
	if !ak.Allow(pub, "anything") {
		t.Fatal("empty rule should allow any subdomain")
	}
}

func TestAllowPrefixRule(t *testing.T) {
	pub, _ := generateKey(t)
	ak := NewAuthorizedKeys([]Entry{{Key: pub, Rule: "demo-*"}})
	if !ak.Allow(pub, "demo-app") {
		t.Fatal("prefix rule should allow matching subdomain")
	}
	if !ak.Allow(pub, "demo-") {
		t.Fatal("prefix rule should allow exact prefix")
	}
	if ak.Allow(pub, "other-app") {
		t.Fatal("prefix rule should not allow non-matching subdomain")
	}
}

func TestAllowExactRule(t *testing.T) {
	pub, _ := generateKey(t)
	ak := NewAuthorizedKeys([]Entry{{Key: pub, Rule: "myapp"}})
	if !ak.Allow(pub, "myapp") {
		t.Fatal("exact rule should allow matching subdomain")
	}
	if ak.Allow(pub, "other") {
		t.Fatal("exact rule should not allow different subdomain")
	}
}

func TestAllowUnknownKey(t *testing.T) {
	pub1, _ := generateKey(t)
	pub2, _ := generateKey(t)
	ak := NewAuthorizedKeys([]Entry{{Key: pub1, Rule: "*"}})
	if ak.Allow(pub2, "foo") {
		t.Fatal("should not allow unknown key")
	}
}

func TestAllowMultipleEntries(t *testing.T) {
	pub1, _ := generateKey(t)
	pub2, _ := generateKey(t)
	ak := NewAuthorizedKeys([]Entry{
		{Key: pub1, Rule: "app-*"},
		{Key: pub2, Rule: "demo-*"},
	})
	if !ak.Allow(pub1, "app-web") {
		t.Fatal("pub1 should be allowed for app-web")
	}
	if ak.Allow(pub1, "demo-web") {
		t.Fatal("pub1 should not be allowed for demo-web")
	}
	if !ak.Allow(pub2, "demo-api") {
		t.Fatal("pub2 should be allowed for demo-api")
	}
}

func TestLoadAuthorizedKeys(t *testing.T) {
	pub1, _ := generateKey(t)
	pub2, _ := generateKey(t)
	content := base64.StdEncoding.EncodeToString(pub1) + " app-*\n" +
		"# comment line\n" +
		"\n" +
		base64.StdEncoding.EncodeToString(pub2) + "\n"
	path := filepath.Join(t.TempDir(), "authorized_keys")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	ak, err := LoadAuthorizedKeys(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	entries := ak.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Rule != "app-*" {
		t.Fatalf("expected rule app-*, got %s", entries[0].Rule)
	}
	if entries[1].Rule != "" {
		t.Fatalf("expected empty rule, got %s", entries[1].Rule)
	}
}

func TestLoadAuthorizedKeysInvalidKey(t *testing.T) {
	content := "notbase64data\n"
	path := filepath.Join(t.TempDir(), "authorized_keys")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadAuthorizedKeys(path)
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestLoadAuthorizedKeysWrongSize(t *testing.T) {
	content := base64.StdEncoding.EncodeToString([]byte("tooshort")) + "\n"
	path := filepath.Join(t.TempDir(), "authorized_keys")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadAuthorizedKeys(path)
	if err == nil {
		t.Fatal("expected error for wrong key size")
	}
}

func TestLoadAuthorizedKeysMissingFile(t *testing.T) {
	_, err := LoadAuthorizedKeys("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestBuildHandshakeMessage(t *testing.T) {
	msg := BuildHandshakeMessage("foo", 1234567890)
	expected := "foo|1234567890"
	if string(msg) != expected {
		t.Fatalf("expected %q, got %q", expected, string(msg))
	}
}

func TestFreshTimestamp(t *testing.T) {
	if FreshTimestamp(0) {
		t.Fatal("zero timestamp should not be fresh")
	}
	if !FreshTimestamp(time.Now().Unix()) {
		t.Fatal("current timestamp should be fresh")
	}
	if FreshTimestamp(time.Now().Add(-3 * time.Minute).Unix()) {
		t.Fatal("3-minute-old timestamp should be stale")
	}
	if FreshTimestamp(time.Now().Add(3 * time.Minute).Unix()) {
		t.Fatal("3-minute-future timestamp should be rejected")
	}
	if !FreshTimestamp(time.Now().Add(-1 * time.Minute).Unix()) {
		t.Fatal("1-minute-old timestamp should be fresh")
	}
	if !FreshTimestamp(time.Now().Add(1 * time.Minute).Unix()) {
		t.Fatal("1-minute-future timestamp should be fresh")
	}
}

func TestHandshakeSignatureRoundTrip(t *testing.T) {
	pub, priv := generateKey(t)
	ts := time.Now().Unix()
	msg := BuildHandshakeMessage("myapp", ts)
	sig := ed25519.Sign(priv, msg)
	if !ed25519.Verify(pub, msg, sig) {
		t.Fatal("signature verification failed")
	}
}
