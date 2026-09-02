// Package bankdata models bank reference data and the stores that hold it.
//
// The same data is served from three interchangeable backends: a snapshot
// compiled into the binary, a snapshot file on disk, and PostgreSQL. Callers
// depend on Repository and never on which backend is in use.
package bankdata

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ErrNotFound is returned when no bank matches a lookup.
var ErrNotFound = errors.New("bankdata: not found")

// Bank is one institution as published by a national registry.
//
// BankCode is the national identifier that appears inside the IBAN (German
// BLZ, Czech payment code, Swiss BC number). It is the join key for IBAN
// lookups, and it is exactly what the pan-European directories do not carry.
type Bank struct {
	Country   string `json:"country"`
	BankCode  string `json:"bankCode"`
	Name      string `json:"name"`
	ShortName string `json:"shortName,omitempty"`
	Zip       string `json:"zip,omitempty"`
	City      string `json:"city,omitempty"`
	BIC       string `json:"bic,omitempty"`

	// CheckAlgo identifies the national account check digit method, currently
	// only populated for Germany where the Bundesbank publishes it.
	CheckAlgo string `json:"checkAlgo,omitempty"`

	// Source names the registry this record came from, so a reload can replace
	// exactly one country without touching the others.
	Source string `json:"source"`
}

// Key returns the canonical lookup key for a bank.
func (b Bank) Key() Key { return Key{Country: b.Country, BankCode: b.BankCode} }

// Key identifies a bank within a country.
type Key struct {
	Country  string
	BankCode string
}

// Normalize upper-cases the country and trims the bank code so that lookups
// are insensitive to how the caller formatted them.
func (k Key) Normalize() Key {
	return Key{
		Country:  strings.ToUpper(strings.TrimSpace(k.Country)),
		BankCode: strings.TrimSpace(k.BankCode),
	}
}

// SourceInfo records where one country's data came from and when, so the
// service can report the age of what it is serving instead of silently
// answering from a stale snapshot.
type SourceInfo struct {
	Country     string    `json:"country"`
	Name        string    `json:"name"`
	URL         string    `json:"url,omitempty"`
	RetrievedAt time.Time `json:"retrievedAt"`
	RecordCount int       `json:"recordCount"`
}

// Stats summarises the contents of a repository.
type Stats struct {
	TotalRecords int          `json:"totalRecords"`
	Sources      []SourceInfo `json:"sources"`
}

// OldestRetrieval returns the least recently refreshed source, which is the
// one that determines whether the dataset counts as stale. The boolean is
// false when the repository holds no sources at all.
func (s Stats) OldestRetrieval() (SourceInfo, bool) {
	if len(s.Sources) == 0 {
		return SourceInfo{}, false
	}
	oldest := s.Sources[0]
	for _, src := range s.Sources[1:] {
		if src.RetrievedAt.Before(oldest.RetrievedAt) {
			oldest = src
		}
	}
	return oldest, true
}

// Query filters a search. A zero Query matches everything, bounded by Limit.
type Query struct {
	Country string
	BIC     string
	Name    string
	Limit   int
}

// DefaultLimit bounds an unbounded search so that a single request cannot ask
// the service to materialise every record it holds.
const DefaultLimit = 100

// MaxLimit is the largest page a caller may request.
const MaxLimit = 1000

// clampLimit keeps Limit inside a sane range.
func (q Query) clampLimit() int {
	switch {
	case q.Limit <= 0:
		return DefaultLimit
	case q.Limit > MaxLimit:
		return MaxLimit
	default:
		return q.Limit
	}
}

// Repository is read access to bank data.
type Repository interface {
	// Find returns the bank with the given country and bank code.
	// It returns ErrNotFound when there is no such bank.
	Find(ctx context.Context, key Key) (Bank, error)

	// FindByBIC returns every bank registered under a BIC. A BIC can map to
	// more than one bank code, so this returns a slice.
	FindByBIC(ctx context.Context, bic string) ([]Bank, error)

	// Search returns banks matching q, capped by the query limit.
	Search(ctx context.Context, q Query) ([]Bank, error)

	// Stats describes what the repository currently holds.
	Stats(ctx context.Context) (Stats, error)
}

// Writer is write access, used by the update pipeline. Backends that serve
// read-only data (the embedded snapshot) do not implement it.
type Writer interface {
	// Replace atomically swaps all records for one source.
	Replace(ctx context.Context, info SourceInfo, banks []Bank) error
}
