package store_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"path/filepath"
	"testing"

	"github.com/gleicon/remo/internal/store"
)

func TestAuthorizedEntries(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if err := st.UpsertAuthorizedKey(ctx, pub, "demo-*"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	entries, err := st.AuthorizedEntries(ctx)
	if err != nil {
		t.Fatalf("entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if string(entries[0].Key) != string(pub) {
		t.Fatalf("pubkey mismatch")
	}
	if entries[0].Rule != "demo-*" {
		t.Fatalf("rule mismatch")
	}
	if err := st.DeleteAuthorizedKey(ctx, base64.StdEncoding.EncodeToString(pub)); err != nil {
		t.Fatalf("delete: %v", err)
	}
	entries, err = st.AuthorizedEntries(ctx)
	if err != nil {
		t.Fatalf("entries after delete: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries after delete, got %d", len(entries))
	}
}

func TestReservations(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pubKeyStr := base64.StdEncoding.EncodeToString(pub)
	owner, err := st.ReservationOwner(ctx, "demo")
	if err != nil || owner != "" {
		t.Fatalf("expected empty owner: %v %s", err, owner)
	}
	if err := st.ReserveSubdomain(ctx, "demo", pubKeyStr); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	owner, err = st.ReservationOwner(ctx, "demo")
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	if owner != pubKeyStr {
		t.Fatalf("owner mismatch")
	}
	if err := st.SetSetting(ctx, "admin_secret", "secret123"); err != nil {
		t.Fatalf("set setting: %v", err)
	}
	value, err := st.GetSetting(ctx, "admin_secret")
	if err != nil {
		t.Fatalf("get setting: %v", err)
	}
	if value != "secret123" {
		t.Fatalf("setting mismatch: %s", value)
	}
}
