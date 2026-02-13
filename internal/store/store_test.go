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

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func genKey(t *testing.T) ed25519.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return pub
}

func TestAuthorizedEntries(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	pub := genKey(t)
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

func TestUpsertAuthorizedKeyUpdatesRule(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	pub := genKey(t)
	if err := st.UpsertAuthorizedKey(ctx, pub, "old-*"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := st.UpsertAuthorizedKey(ctx, pub, "new-*"); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	entries, err := st.AuthorizedEntries(ctx)
	if err != nil {
		t.Fatalf("entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Rule != "new-*" {
		t.Fatalf("expected updated rule, got %s", entries[0].Rule)
	}
}

func TestReservations(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	pub := genKey(t)
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
}

func TestReservationsList(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	pub1 := genKey(t)
	pub2 := genKey(t)
	if err := st.ReserveSubdomain(ctx, "alpha", base64.StdEncoding.EncodeToString(pub1)); err != nil {
		t.Fatalf("reserve alpha: %v", err)
	}
	if err := st.ReserveSubdomain(ctx, "beta", base64.StdEncoding.EncodeToString(pub2)); err != nil {
		t.Fatalf("reserve beta: %v", err)
	}
	list, err := st.Reservations(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 reservations, got %d", len(list))
	}
	if list[0].Subdomain != "alpha" || list[1].Subdomain != "beta" {
		t.Fatalf("expected sorted by subdomain: %v", list)
	}
}

func TestReserveSubdomainOverwrite(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	pub1 := genKey(t)
	pub2 := genKey(t)
	pub1Str := base64.StdEncoding.EncodeToString(pub1)
	pub2Str := base64.StdEncoding.EncodeToString(pub2)
	if err := st.ReserveSubdomain(ctx, "app", pub1Str); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := st.ReserveSubdomain(ctx, "app", pub2Str); err != nil {
		t.Fatalf("reserve overwrite: %v", err)
	}
	owner, err := st.ReservationOwner(ctx, "app")
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	if owner != pub2Str {
		t.Fatal("expected owner to be updated")
	}
}

func TestSettings(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
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

func TestSettingUpdate(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	if err := st.SetSetting(ctx, "key", "v1"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := st.SetSetting(ctx, "key", "v2"); err != nil {
		t.Fatalf("update: %v", err)
	}
	value, err := st.GetSetting(ctx, "key")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if value != "v2" {
		t.Fatalf("expected v2, got %s", value)
	}
}

func TestGetSettingMissing(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	value, err := st.GetSetting(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if value != "" {
		t.Fatalf("expected empty, got %s", value)
	}
}

func TestGetSettingNilStore(t *testing.T) {
	var st *store.Store
	value, err := st.GetSetting(context.Background(), "key")
	if err != nil {
		t.Fatalf("get nil store: %v", err)
	}
	if value != "" {
		t.Fatal("expected empty for nil store")
	}
}

func TestSetSettingNilStore(t *testing.T) {
	var st *store.Store
	err := st.SetSetting(context.Background(), "key", "val")
	if err == nil {
		t.Fatal("expected error for nil store")
	}
}

func TestStats(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	stats, err := st.Stats(ctx)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.AuthorizedKeys != 0 || stats.Reservations != 0 {
		t.Fatal("expected zero stats for empty store")
	}
	pub := genKey(t)
	st.UpsertAuthorizedKey(ctx, pub, "*")
	st.ReserveSubdomain(ctx, "foo", base64.StdEncoding.EncodeToString(pub))
	stats, err = st.Stats(ctx)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.AuthorizedKeys != 1 {
		t.Fatalf("expected 1 key, got %d", stats.AuthorizedKeys)
	}
	if stats.Reservations != 1 {
		t.Fatalf("expected 1 reservation, got %d", stats.Reservations)
	}
}

func TestStatsNilStore(t *testing.T) {
	var st *store.Store
	stats, err := st.Stats(context.Background())
	if err != nil {
		t.Fatalf("stats nil: %v", err)
	}
	if stats.AuthorizedKeys != 0 || stats.Reservations != 0 {
		t.Fatal("expected zero stats for nil store")
	}
}

func TestLogEvent(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	st.LogEvent(ctx, "connect", "foo", "pubkey123")
}

func TestCountAuthorizedKeys(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	count, err := st.CountAuthorizedKeys(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}
	st.UpsertAuthorizedKey(ctx, genKey(t), "*")
	st.UpsertAuthorizedKey(ctx, genKey(t), "foo-*")
	count, err = st.CountAuthorizedKeys(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2, got %d", count)
	}
}

func TestCountReservations(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	count, err := st.CountReservations(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}
	st.ReserveSubdomain(ctx, "a", "key1")
	st.ReserveSubdomain(ctx, "b", "key2")
	count, err = st.CountReservations(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2, got %d", count)
	}
}

func TestCloseNilStore(t *testing.T) {
	var st *store.Store
	if err := st.Close(); err != nil {
		t.Fatalf("close nil: %v", err)
	}
}

func TestDeleteAuthorizedKeyNonexistent(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	if err := st.DeleteAuthorizedKey(ctx, "nonexistent"); err != nil {
		t.Fatalf("delete nonexistent: %v", err)
	}
}
