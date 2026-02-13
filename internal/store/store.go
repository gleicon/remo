package store

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/gleicon/remo/internal/auth"
)

type Store struct {
	db *sql.DB
}

type StoreStats struct {
	AuthorizedKeys int
	Reservations   int
}

type Reservation struct {
	Subdomain string
	Pubkey    string
	CreatedAt time.Time
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	schema := []string{
		`CREATE TABLE IF NOT EXISTS authorized_keys (
			pubkey TEXT PRIMARY KEY,
			rule TEXT NOT NULL DEFAULT '*',
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS reservations (
			subdomain TEXT PRIMARY KEY,
			pubkey TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event TEXT NOT NULL,
			subdomain TEXT,
			pubkey TEXT,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
	}
	for _, stmt := range schema {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) AuthorizedEntries(ctx context.Context) ([]auth.Entry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT pubkey, rule FROM authorized_keys`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []auth.Entry
	for rows.Next() {
		var keyStr, rule string
		if err := rows.Scan(&keyStr, &rule); err != nil {
			return nil, err
		}
		keyBytes, err := base64.StdEncoding.DecodeString(keyStr)
		if err != nil {
			return nil, err
		}
		if len(keyBytes) != ed25519.PublicKeySize {
			return nil, errors.New("invalid key size in store")
		}
		entries = append(entries, auth.Entry{Key: ed25519.PublicKey(keyBytes), Rule: rule})
	}
	return entries, rows.Err()
}

func (s *Store) UpsertAuthorizedKey(ctx context.Context, key ed25519.PublicKey, rule string) error {
	keyStr := base64.StdEncoding.EncodeToString(key)
	_, err := s.db.ExecContext(ctx, `INSERT INTO authorized_keys(pubkey, rule, created_at)
		VALUES(?, ?, ?)
		ON CONFLICT(pubkey) DO UPDATE SET rule=excluded.rule`, keyStr, rule, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) DeleteAuthorizedKey(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM authorized_keys WHERE pubkey = ?`, key)
	return err
}

func (s *Store) ReservationOwner(ctx context.Context, subdomain string) (string, error) {
	var owner string
	err := s.db.QueryRowContext(ctx, `SELECT pubkey FROM reservations WHERE subdomain = ?`, subdomain).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return owner, err
}

func (s *Store) ReserveSubdomain(ctx context.Context, subdomain, pubkey string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO reservations(subdomain, pubkey, created_at)
		VALUES(?, ?, ?)
		ON CONFLICT(subdomain) DO UPDATE SET pubkey=excluded.pubkey`, subdomain, pubkey, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) LogEvent(ctx context.Context, event, subdomain, pubkey string) {
	_, _ = s.db.ExecContext(ctx, `INSERT INTO audit_log(event, subdomain, pubkey, created_at) VALUES(?, ?, ?, ?)`, event, subdomain, pubkey, time.Now().UTC().Format(time.RFC3339Nano))
}

func (s *Store) CountAuthorizedKeys(ctx context.Context) (int, error) {
	return s.count(ctx, `SELECT COUNT(1) FROM authorized_keys`)
}

func (s *Store) CountReservations(ctx context.Context) (int, error) {
	return s.count(ctx, `SELECT COUNT(1) FROM reservations`)
}

func (s *Store) count(ctx context.Context, query string) (int, error) {
	if s == nil {
		return 0, nil
	}
	var value int
	err := s.db.QueryRowContext(ctx, query).Scan(&value)
	return value, err
}

func (s *Store) Reservations(ctx context.Context) ([]Reservation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT subdomain, pubkey, created_at FROM reservations ORDER BY subdomain ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Reservation
	for rows.Next() {
		var subdomain, pubkey, createdAt string
		if err := rows.Scan(&subdomain, &pubkey, &createdAt); err != nil {
			return nil, err
		}
		timestamp, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			timestamp = time.Time{}
		}
		list = append(list, Reservation{Subdomain: subdomain, Pubkey: pubkey, CreatedAt: timestamp})
	}
	return list, rows.Err()
}

func (s *Store) Stats(ctx context.Context) (StoreStats, error) {
	if s == nil {
		return StoreStats{}, nil
	}
	keys, err := s.CountAuthorizedKeys(ctx)
	if err != nil {
		return StoreStats{}, err
	}
	resv, err := s.CountReservations(ctx)
	if err != nil {
		return StoreStats{}, err
	}
	return StoreStats{AuthorizedKeys: keys, Reservations: resv}, nil
}

func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	if s == nil {
		return "", nil
	}
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	if s == nil {
		return errors.New("nil store")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}
