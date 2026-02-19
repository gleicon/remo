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

// MarshalPrivateKey exports the private key in OpenSSH format
func (i *Identity) MarshalPrivateKey() ([]byte, error) {
	if i == nil || i.Private == nil {
		return nil, errors.New("identity is nil")
	}
	// OpenSSH private key format for Ed25519
	// This is a simplified format - the full OpenSSH format includes
	// encryption headers and checksums, but for unencrypted keys:
	// -----BEGIN OPENSSH PRIVATE KEY-----
	// base64-encoded key
	// -----END OPENSSH PRIVATE KEY-----

	// For Ed25519, the private key is 64 bytes (seed + public key)
	// We need to construct the proper OpenSSH format
	return marshalOpenSSHKey(i.Private, i.Public)
}

func marshalOpenSSHKey(priv ed25519.PrivateKey, pub ed25519.PublicKey) ([]byte, error) {
	// OpenSSH private key format:
	// "openssh-key-v1\0"
	// ciphername (string) - "none"
	// kdfname (string) - "none"
	// kdfoptions (string) - ""
	// number of keys (uint32) - 1
	// public key (string) - ssh public key format
	// private key (string) - contains check ints + key pairs

	var buf []byte

	// Magic header
	buf = append(buf, []byte("openssh-key-v1\x00")...)

	// Cipher name (none)
	buf = appendString(buf, "none")

	// KDF name (none)
	buf = appendString(buf, "none")

	// KDF options (empty)
	buf = appendString(buf, "")

	// Number of keys (1)
	buf = appendUint32(buf, 1)

	// Public key in SSH wire format
	pubKeyWire := marshalEd25519PublicKey(pub)
	buf = appendString(buf, string(pubKeyWire))

	// Private key section
	var privSection []byte
	// Check int (random, used for decryption verification)
	privSection = appendUint32(privSection, 0x12345678)
	privSection = appendUint32(privSection, 0x12345678)
	// Key type
	privSection = appendString(privSection, "ssh-ed25519")
	// Public key
	privSection = appendString(privSection, string(pub))
	// Private key (seed only - first 32 bytes)
	privSection = appendString(privSection, string(priv[:32]))
	// Comment
	privSection = appendString(privSection, "remo")
	// Padding to 8-byte boundary
	for len(privSection)%8 != 0 {
		privSection = append(privSection, byte(len(privSection)%8))
	}

	buf = appendString(buf, string(privSection))

	// Base64 encode
	encoded := base64.StdEncoding.EncodeToString(buf)

	// Format as PEM
	result := []byte("-----BEGIN OPENSSH PRIVATE KEY-----\n")
	// Split into 70-char lines
	for i := 0; i < len(encoded); i += 70 {
		end := i + 70
		if end > len(encoded) {
			end = len(encoded)
		}
		result = append(result, encoded[i:end]...)
		result = append(result, '\n')
	}
	result = append(result, []byte("-----END OPENSSH PRIVATE KEY-----\n")...)

	return result, nil
}

func appendString(buf []byte, s string) []byte {
	buf = appendUint32(buf, uint32(len(s)))
	buf = append(buf, s...)
	return buf
}

func appendUint32(buf []byte, n uint32) []byte {
	return append(buf, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
}

func marshalEd25519PublicKey(pub ed25519.PublicKey) []byte {
	// SSH wire format: "ssh-ed25519" || len(pub) || pub
	var buf []byte
	buf = appendString(buf, "ssh-ed25519")
	buf = appendString(buf, string(pub))
	return buf
}

func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "identity.json"
	}
	return filepath.Join(home, ".remo", "identity.json")
}
