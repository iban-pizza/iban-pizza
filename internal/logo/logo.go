// Package logo resolves a bank to a logo.
//
// Bank logos are trademarks of their owners. This package therefore never
// embeds third party artwork by default. It ships a mapping from BIC prefix to
// brand, and it can always generate a neutral monogram from the institution's
// own name, which raises no trademark question and guarantees that every bank
// has something to display. Real artwork is opt in, supplied through a
// Resolver that the operator configures.
package logo

import (
	"fmt"
	"strings"
)

// Kind describes what a Logo actually is, so a caller can tell generated
// artwork from a real mark.
type Kind string

const (
	// KindMonogram is generated from the bank's initials. Always available.
	KindMonogram Kind = "monogram"

	// KindBrand is a real logo supplied by a configured resolver.
	KindBrand Kind = "brand"
)

// Logo is the result of a lookup.
type Logo struct {
	Kind Kind `json:"kind"`

	// Brand is the institution group the BIC prefix belongs to, when known.
	Brand string `json:"brand,omitempty"`

	// URL points at the artwork. For a monogram this is the service's own
	// endpoint, which renders SVG on demand.
	URL string `json:"url"`
}

// Resolver supplies real brand artwork. Implementations may call an external
// service, which is why none is enabled by default: looking up a logo would
// otherwise tell a third party which bank a user just validated.
type Resolver interface {
	// Resolve returns a URL for the given BIC and brand, or false when it has
	// nothing for them.
	Resolve(bic, brand string) (string, bool)
}

// Service resolves logos.
type Service struct {
	// BaseURL prefixes generated monogram URLs.
	BaseURL string

	// Brands maps a four character BIC institution prefix to a brand name.
	Brands map[string]string

	// Resolver is consulted before falling back to a monogram. Nil means
	// monograms only, which is the default.
	Resolver Resolver
}

// New returns a Service with the built in brand mapping.
func New(baseURL string) *Service {
	return &Service{BaseURL: strings.TrimRight(baseURL, "/"), Brands: defaultBrands()}
}

// InstitutionPrefix returns the first four characters of a BIC, which identify
// the institution independently of country and branch.
func InstitutionPrefix(bic string) string {
	bic = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(bic), " ", ""))
	if len(bic) < 4 {
		return ""
	}
	return bic[:4]
}

// Brand returns the brand name for a BIC, if the mapping knows it.
func (s *Service) Brand(bic string) (string, bool) {
	prefix := InstitutionPrefix(bic)
	if prefix == "" {
		return "", false
	}
	// A shared BIC space is never a brand, even if someone adds one to the
	// map by mistake. Hundreds of independent banks sit behind these prefixes.
	if IsSharedSpace(prefix) {
		return "", false
	}
	name, ok := s.Brands[prefix]
	return name, ok
}

// For returns the logo to use for a bank.
//
// It never returns an empty result: when no brand artwork is available it
// points at the monogram endpoint, which renders from the name.
func (s *Service) For(country, bankCode, bic, name string) Logo {
	brand, hasBrand := s.Brand(bic)

	if s.Resolver != nil && hasBrand {
		if url, ok := s.Resolver.Resolve(bic, brand); ok {
			return Logo{Kind: KindBrand, Brand: brand, URL: url}
		}
	}

	l := Logo{
		Kind: KindMonogram,
		URL:  fmt.Sprintf("%s/v2/banks/%s/%s/logo.svg", s.BaseURL, country, bankCode),
	}
	if hasBrand {
		l.Brand = brand
	}
	_ = name
	return l
}
