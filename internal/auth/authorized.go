package auth

import (
	"bufio"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const handshakeSkew = 2 * time.Minute

type Entry struct {
	Key  ed25519.PublicKey
	Rule string
}

type AuthorizedKeys struct {
	entries []Entry
}

func NewAuthorizedKeys(entries []Entry) *AuthorizedKeys {
	return &AuthorizedKeys{entries: append([]Entry(nil), entries...)}
}

func (a *AuthorizedKeys) Entries() []Entry {
	if a == nil {
		return nil
	}
	return append([]Entry(nil), a.entries...)
}

func LoadAuthorizedKeys(path string) (*AuthorizedKeys, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var entries []Entry
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		keyBytes, err := base64.StdEncoding.DecodeString(fields[0])
		if err != nil || len(keyBytes) != ed25519.PublicKeySize {
			return nil, errors.New("invalid authorized key entry")
		}
		rule := ""
		if len(fields) > 1 {
			rule = fields[1]
		}
		entries = append(entries, Entry{Key: ed25519.PublicKey(keyBytes), Rule: rule})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return &AuthorizedKeys{entries: entries}, nil
}

func (a *AuthorizedKeys) Allow(pub ed25519.PublicKey, subdomain string) bool {
	if a == nil {
		return true
	}
	for _, entry := range a.entries {
		if !equalKeys(entry.Key, pub) {
			continue
		}
		if entry.Rule == "" || entry.Rule == "*" {
			return true
		}
		if strings.HasSuffix(entry.Rule, "*") {
			prefix := strings.TrimSuffix(entry.Rule, "*")
			if strings.HasPrefix(subdomain, prefix) {
				return true
			}
			continue
		}
		if subdomain == entry.Rule {
			return true
		}
	}
	return false
}

func equalKeys(a, b ed25519.PublicKey) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func BuildHandshakeMessage(subdomain string, ts int64) []byte {
	return []byte(fmt.Sprintf("%s|%d", subdomain, ts))
}

func FreshTimestamp(ts int64) bool {
	if ts == 0 {
		return false
	}
	deadline := time.Unix(ts, 0)
	if time.Since(deadline) > handshakeSkew {
		return false
	}
	if deadline.After(time.Now().Add(handshakeSkew)) {
		return false
	}
	return true
}
