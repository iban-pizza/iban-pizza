// Package sources fetches and parses the official bank registries.
//
// Each country gets its own Source. Registries differ in encoding, layout and
// how their download URL is found, so a generic reader would be wrong for most
// of them; the shared parts are HTTP fetching and text decoding, and those
// live here.
package sources

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/netzfabrikcom/iban-pizza/bankdata"
)

// Source is one national registry.
type Source interface {
	// Country is the ISO 3166-1 alpha-2 code this source supplies.
	Country() string

	// Name identifies the publisher. It is stored on every record so that a
	// reload replaces exactly this source and nothing else.
	Name() string

	// Fetch downloads the current dataset and returns the raw bytes together
	// with the URL it actually used, which for some publishers is resolved at
	// run time rather than fixed.
	Fetch(ctx context.Context, c *Client) (data []byte, url string, err error)

	// Parse converts raw bytes into bank records.
	Parse(data []byte) ([]bankdata.Bank, error)
}

// Result reports the outcome of updating one source.
type Result struct {
	Country     string
	Source      string
	URL         string
	Records     int
	RetrievedAt time.Time
	Err         error
}

// registry holds the available sources keyed by country code.
var registry = map[string]Source{}

// Register adds a source. It panics on a duplicate country because that is a
// programming error rather than a runtime condition.
func Register(s Source) {
	code := s.Country()
	if _, exists := registry[code]; exists {
		panic(fmt.Sprintf("sources: duplicate source for %s", code))
	}
	registry[code] = s
}

// All returns every registered source, ordered by country code so that update
// runs and their logs are reproducible.
func All() []Source {
	out := make([]Source, 0, len(registry))
	for _, s := range registry {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Country() < out[j].Country() })
	return out
}

// Get returns the source for a country code.
func Get(country string) (Source, bool) {
	s, ok := registry[country]
	return s, ok
}

// Countries lists the country codes that have a source.
func Countries() []string {
	out := make([]string, 0, len(registry))
	for c := range registry {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// Update fetches and parses one source and writes it into w. The store is only
// touched when both the download and the parse succeed, so a failed refresh
// leaves the previous data in place rather than emptying it.
func Update(ctx context.Context, c *Client, s Source, w bankdata.Writer) Result {
	res := Result{Country: s.Country(), Source: s.Name(), RetrievedAt: time.Now().UTC()}

	raw, url, err := s.Fetch(ctx, c)
	res.URL = url
	if err != nil {
		res.Err = fmt.Errorf("fetch %s: %w", s.Name(), err)
		return res
	}

	banks, err := s.Parse(raw)
	if err != nil {
		res.Err = fmt.Errorf("parse %s: %w", s.Name(), err)
		return res
	}
	if len(banks) == 0 {
		res.Err = fmt.Errorf("parse %s: no records found, refusing to replace existing data", s.Name())
		return res
	}
	res.Records = len(banks)

	info := bankdata.SourceInfo{
		Country:     s.Country(),
		Name:        s.Name(),
		URL:         url,
		RetrievedAt: res.RetrievedAt,
		RecordCount: len(banks),
	}
	if err := w.Replace(ctx, info, banks); err != nil {
		res.Err = fmt.Errorf("store %s: %w", s.Name(), err)
	}
	return res
}
