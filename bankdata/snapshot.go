package bankdata

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// SnapshotVersion is the format version written into every snapshot header.
// Decode refuses anything it does not know rather than guessing at the layout.
const SnapshotVersion = 1

// SnapshotHeader is the first line of a snapshot.
type SnapshotHeader struct {
	Version   int          `json:"version"`
	CreatedAt time.Time    `json:"createdAt"`
	Sources   []SourceInfo `json:"sources"`
}

// Snapshot is gzipped JSON lines: one header object, then one Bank per line.
//
// The format is deliberately boring. It streams, so building or loading it
// never holds the whole dataset twice in memory, and a human can gunzip it and
// read the result when something looks wrong.

// WriteSnapshot serialises a repository to w.
func WriteSnapshot(ctx context.Context, w io.Writer, repo Repository) error {
	stats, err := repo.Stats(ctx)
	if err != nil {
		return fmt.Errorf("read stats: %w", err)
	}

	banks, err := repo.Search(ctx, Query{Limit: MaxLimit})
	if err != nil {
		return fmt.Errorf("read banks: %w", err)
	}
	// Search is capped, so pull everything through the store directly when the
	// repository can provide it. MemoryStore is the only writer target today.
	if ms, ok := repo.(*MemoryStore); ok {
		banks = ms.All()
	}

	gz := gzip.NewWriter(w)
	defer gz.Close()

	enc := json.NewEncoder(gz)
	header := SnapshotHeader{
		Version:   SnapshotVersion,
		CreatedAt: time.Now().UTC(),
		Sources:   stats.Sources,
	}
	if err := enc.Encode(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, b := range banks {
		if err := enc.Encode(b); err != nil {
			return fmt.Errorf("write record: %w", err)
		}
	}
	return gz.Close()
}

// All returns every record in deterministic order so that snapshots of equal
// content are byte identical, which keeps them diffable in git.
func (s *MemoryStore) All() []Bank {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Bank, 0, len(s.byKey))
	for _, b := range s.byKey {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Country != out[j].Country {
			return out[i].Country < out[j].Country
		}
		return out[i].BankCode < out[j].BankCode
	})
	return out
}

// ReadSnapshot loads a snapshot into a new MemoryStore.
func ReadSnapshot(r io.Reader) (*MemoryStore, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("open snapshot: %w", err)
	}
	defer gz.Close()

	sc := bufio.NewScanner(gz)
	// Records are short, but give the scanner room so that one long bank name
	// cannot truncate the stream.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return nil, fmt.Errorf("read header: %w", err)
		}
		return nil, fmt.Errorf("snapshot is empty")
	}
	var header SnapshotHeader
	if err := json.Unmarshal(sc.Bytes(), &header); err != nil {
		return nil, fmt.Errorf("parse header: %w", err)
	}
	if header.Version != SnapshotVersion {
		return nil, fmt.Errorf("snapshot version %d is not supported, want %d",
			header.Version, SnapshotVersion)
	}

	store := NewMemoryStore()
	line := 1
	for sc.Scan() {
		line++
		if len(sc.Bytes()) == 0 {
			continue
		}
		var b Bank
		if err := json.Unmarshal(sc.Bytes(), &b); err != nil {
			return nil, fmt.Errorf("parse record on line %d: %w", line, err)
		}
		store.byKey[b.Key().Normalize()] = b
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read snapshot: %w", err)
	}

	for _, src := range header.Sources {
		store.sources[src.Name] = src
	}
	store.reindexBIC()
	return store, nil
}

// LoadSnapshotFile reads a snapshot from disk.
func LoadSnapshotFile(path string) (*MemoryStore, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadSnapshot(f)
}

// SaveSnapshotFile writes a repository to path atomically, via a temporary file
// in the same directory followed by a rename, so that a crash midway cannot
// leave a half written snapshot where a valid one used to be.
func SaveSnapshotFile(ctx context.Context, path string, repo Repository) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := WriteSnapshot(ctx, tmp, repo); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// os.CreateTemp makes the file readable only by its owner, and the rename
	// carries that through. A snapshot written by a deployment or cron user
	// would then be unreadable to the service account running "openiban
	// serve". The data is a public bank register, so it gets ordinary file
	// permissions.
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// CopySources writes every source held in from into to, one source at a time.
//
// This is how reviewed data reaches a database without the database host ever
// contacting a publisher: the snapshot compiled into the image was fetched,
// tested and reviewed in the release pipeline, and this copies it across. Each
// source is replaced as a unit, so a partially applied copy cannot leave a
// country half loaded.
func CopySources(ctx context.Context, from *MemoryStore, to Writer) ([]SourceInfo, error) {
	stats, err := from.Stats(ctx)
	if err != nil {
		return nil, err
	}

	bySource := make(map[string][]Bank, len(stats.Sources))
	for _, b := range from.All() {
		bySource[b.Source] = append(bySource[b.Source], b)
	}

	copied := make([]SourceInfo, 0, len(stats.Sources))
	for _, info := range stats.Sources {
		banks := bySource[info.Name]
		if len(banks) == 0 {
			// A source with metadata but no records is a broken snapshot,
			// and replacing a country with nothing is not a copy.
			return copied, fmt.Errorf("source %q has no records", info.Name)
		}
		if err := to.Replace(ctx, info, banks); err != nil {
			return copied, fmt.Errorf("copy %s: %w", info.Name, err)
		}
		info.RecordCount = len(banks)
		copied = append(copied, info)
	}
	return copied, nil
}
