package root

import (
	"os"
	"path/filepath"

	"github.com/gleicon/remo/internal/store"
)

func defaultStatePath() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "remo", "state.db")
	}
	return "remo-state.db"
}

func openState(path string) (*store.Store, error) {
	if path == "" {
		path = defaultStatePath()
	}
	return store.Open(path)
}
