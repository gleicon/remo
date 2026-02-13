package identity

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerate(t *testing.T) {
	id, err := Generate()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(id.Public) != ed25519.PublicKeySize {
		t.Fatalf("public key size: %d", len(id.Public))
	}
	if len(id.Private) != ed25519.PrivateKeySize {
		t.Fatalf("private key size: %d", len(id.Private))
	}
	msg := []byte("test message")
	sig := ed25519.Sign(id.Private, msg)
	if !ed25519.Verify(id.Public, msg, sig) {
		t.Fatal("generated keypair does not sign/verify correctly")
	}
}

func TestSaveAndLoad(t *testing.T) {
	id, err := Generate()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	path := filepath.Join(t.TempDir(), "subdir", "identity.json")
	if err := id.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected mode 0600, got %o", info.Mode().Perm())
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if string(loaded.Public) != string(id.Public) {
		t.Fatal("public key mismatch after load")
	}
	if string(loaded.Private) != string(id.Private) {
		t.Fatal("private key mismatch after load")
	}
}

func TestSaveNilIdentity(t *testing.T) {
	var id *Identity
	err := id.Save(filepath.Join(t.TempDir(), "identity.json"))
	if err == nil {
		t.Fatal("expected error for nil identity")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/identity.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadInvalidBase64(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")
	data, _ := json.Marshal(filePayload{Private: "!!!invalid!!!", Public: "!!!invalid!!!"})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestLoadWrongKeySize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")
	data, _ := json.Marshal(filePayload{Private: "AAAA", Public: "AAAA"})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for wrong key size")
	}
}

func TestDefaultPath(t *testing.T) {
	path := DefaultPath()
	if path == "" {
		t.Fatal("default path should not be empty")
	}
}
