package memory_test

import (
	"testing"

	"github.com/ugurcan-aytar/rampart/engine/internal/storage"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage/memory"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage/storagetest"
)

// The contract test drives every Storage behaviour against this backend.
// When SQLite / Postgres implementations land, each gets its own *_test.go
// that calls storagetest.Run the same way — the suite itself is the spec.
func TestContract(t *testing.T) {
	storagetest.Run(t, func() storage.Storage {
		return memory.New()
	})
}
