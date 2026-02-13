package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

type Identity struct {
	Private ed25519.PrivateKey
	Public  ed25519.PublicKey
}

type filePayload struct {
	Private string `json:"private"`
	Public  string `json:"public"`
}

func Generate() (*Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Identity{Private: priv, Public: pub}, nil
}

func Load(path string) (*Identity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var payload filePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	priv, err := base64.StdEncoding.DecodeString(payload.Private)
	if err != nil {
		return nil, err
	}
	pub, err := base64.StdEncoding.DecodeString(payload.Public)
	if err != nil {
		return nil, err
	}
	if len(priv) != ed25519.PrivateKeySize || len(pub) != ed25519.PublicKeySize {
		return nil, errors.New("invalid identity key size")
	}
	return &Identity{Private: ed25519.PrivateKey(priv), Public: ed25519.PublicKey(pub)}, nil
}

func (i *Identity) Save(path string) error {
	if i == nil {
		return errors.New("identity is nil")
	}
	payload := filePayload{
		Private: base64.StdEncoding.EncodeToString(i.Private),
		Public:  base64.StdEncoding.EncodeToString(i.Public),
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, fs.FileMode(0o600))
}

func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "identity.json"
	}
	return filepath.Join(home, ".remo", "identity.json")
}
