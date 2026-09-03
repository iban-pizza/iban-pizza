// Package embedded holds the bank data snapshot compiled into the binary.
//
// This is what makes the zero configuration single binary possible: it starts
// and answers correctly with no database, no data directory and no network.
// The snapshot is refreshed by "iban-pizza update --write-snapshot" and checked
// in, so its age is visible in the repository rather than hidden in a build.
package embedded

import (
	"bytes"
	_ "embed"
	"errors"

	"github.com/iban-pizza/iban-pizza/bankdata"
)

//go:embed snapshot.jsonl.gz
var snapshot []byte

// ErrEmpty is returned when the binary was built without a snapshot.
var ErrEmpty = errors.New("embedded: no snapshot was compiled in")

// Available reports whether a usable snapshot is present.
func Available() bool { return len(snapshot) > 64 }

// Load returns the embedded snapshot as a store.
func Load() (*bankdata.MemoryStore, error) {
	if !Available() {
		return nil, ErrEmpty
	}
	return bankdata.ReadSnapshot(bytes.NewReader(snapshot))
}

// Size reports the compressed size of the embedded snapshot.
func Size() int { return len(snapshot) }
