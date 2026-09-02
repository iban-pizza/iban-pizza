package sepa

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Fetcher downloads a URL. The sources package provides the implementation;
// declaring the dependency this way keeps the two packages independent.
type Fetcher interface {
	Get(ctx context.Context, url string) ([]byte, error)
}

// Download fetches every scheme export and returns a populated registry.
//
// A scheme that fails to download is reported but does not abort the run: a
// partial register is more useful than none, and the missing scheme simply
// reports unknown.
func Download(ctx context.Context, f Fetcher) (*Registry, []error) {
	r := NewRegistry()
	var errs []error

	for _, scheme := range AllSchemes {
		data, err := f.Get(ctx, URL(scheme))
		if err != nil {
			errs = append(errs, fmt.Errorf("fetch %s: %w", scheme, err))
			continue
		}
		participants, err := ParseCSV(data)
		if err != nil {
			errs = append(errs, fmt.Errorf("parse %s: %w", scheme, err))
			continue
		}
		r.Add(scheme, participants)
	}

	r.SetAsOf(time.Now().UTC())
	return r, errs
}

// SaveDir writes the raw exports so that a later start can load them without
// contacting the EPC again.
func SaveDir(dir string, files map[Scheme][]byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for scheme, data := range files {
		path := filepath.Join(dir, scheme.file()+".csv")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

// LoadDir reads scheme exports previously written by SaveDir.
//
// A missing scheme file is not an error. The register is optional as a whole,
// and a scheme that is absent reports unknown rather than a false negative.
func LoadDir(dir string) (*Registry, error) {
	r := NewRegistry()
	found := 0
	var newest time.Time

	for _, scheme := range AllSchemes {
		path := filepath.Join(dir, scheme.file()+".csv")
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		participants, err := ParseCSV(data)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		r.Add(scheme, participants)
		found++

		if info, err := os.Stat(path); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}

	if found == 0 {
		return nil, fmt.Errorf("no scheme exports found in %s", dir)
	}
	r.SetAsOf(newest.UTC())
	return r, nil
}
