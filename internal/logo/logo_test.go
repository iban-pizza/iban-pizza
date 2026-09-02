package logo

import (
	"strings"
	"testing"
)

func TestInstitutionPrefix(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"INGDDEFFXXX", "INGD"},
		{"ingddeffxxx", "INGD"},
		{" COBA DEFF XXX ", "COBA"},
		{"ABC", ""},
		{"", ""},
	} {
		if got := InstitutionPrefix(tc.in); got != tc.want {
			t.Errorf("InstitutionPrefix(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBrandLookup(t *testing.T) {
	s := New("https://iban.pizza")

	if brand, ok := s.Brand("INGDDEFFXXX"); !ok || brand != "ING" {
		t.Errorf("ING lookup returned %q, %v", brand, ok)
	}
	if brand, ok := s.Brand("COBADEFFXXX"); !ok || brand != "Commerzbank" {
		t.Errorf("Commerzbank lookup returned %q, %v", brand, ok)
	}
	if _, ok := s.Brand("ZZZZDEFFXXX"); ok {
		t.Error("an unknown prefix returned a brand")
	}
}

// TestSharedSpacesNeverGetABrand is the correctness guard for the mapping.
//
// GENO covers 1122 differently named cooperative banks and the Landesbank
// prefixes cover dozens of independent savings banks each. Assigning any of
// them a single brand would mislabel hundreds of institutions.
func TestSharedSpacesNeverGetABrand(t *testing.T) {
	s := New("https://iban.pizza")

	for _, bic := range []string{
		"GENODEFFXXX", "GENODED1AAC", "WELADEDDXXX",
		"NOLADE21XXX", "BYLADEM1XXX", "SOLADEST600",
		"HELADEFFXXX", "MALADE51XXX", "BRLADE21XXX",
	} {
		if brand, ok := s.Brand(bic); ok {
			t.Errorf("shared space %s was given the brand %q", bic, brand)
		}
		if !IsSharedSpace(bic) {
			t.Errorf("%s is not recognised as a shared space", bic)
		}
	}

	// A shared space bank must still get a usable logo, namely its own
	// monogram rather than nothing.
	got := s.For("DE", "50190000", "GENODEFFXXX", "Frankfurter Volksbank")
	if got.Kind != KindMonogram {
		t.Errorf("shared space bank got kind %q, want monogram", got.Kind)
	}
	if got.Brand != "" {
		t.Errorf("shared space bank carries brand %q", got.Brand)
	}
	if got.URL == "" {
		t.Error("shared space bank has no logo URL")
	}
}

func TestForAlwaysReturnsAURL(t *testing.T) {
	s := New("https://iban.pizza")
	for _, tc := range []struct{ country, code, bic, name string }{
		{"DE", "50010517", "INGDDEFFXXX", "ING-DiBa"},
		{"DE", "12345678", "", "Bank without a BIC"},
		{"CZ", "0100", "KOMBCZPP", "Komercni banka"},
	} {
		got := s.For(tc.country, tc.code, tc.bic, tc.name)
		if got.URL == "" {
			t.Errorf("%s/%s produced no logo URL", tc.country, tc.code)
		}
		if !strings.Contains(got.URL, tc.country+"/"+tc.code) {
			t.Errorf("URL %q does not identify the bank", got.URL)
		}
	}
}

type fakeResolver struct{ url string }

func (f fakeResolver) Resolve(string, string) (string, bool) {
	if f.url == "" {
		return "", false
	}
	return f.url, true
}

func TestResolverIsOptedIn(t *testing.T) {
	s := New("https://iban.pizza")

	// Without a resolver the service must never point at a third party, since
	// that would disclose which bank a user just looked up.
	got := s.For("DE", "50010517", "INGDDEFFXXX", "ING-DiBa")
	if got.Kind != KindMonogram {
		t.Errorf("default configuration returned kind %q, want monogram", got.Kind)
	}

	s.Resolver = fakeResolver{url: "https://cdn.example/ing.svg"}
	got = s.For("DE", "50010517", "INGDDEFFXXX", "ING-DiBa")
	if got.Kind != KindBrand || got.URL != "https://cdn.example/ing.svg" {
		t.Errorf("configured resolver was not used: %+v", got)
	}

	// A resolver that has nothing must fall back rather than return an empty
	// logo.
	s.Resolver = fakeResolver{}
	got = s.For("DE", "50010517", "INGDDEFFXXX", "ING-DiBa")
	if got.Kind != KindMonogram || got.URL == "" {
		t.Errorf("fallback did not happen: %+v", got)
	}
}

func TestMonogramInitials(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"Deutsche Bank", "DB"},
		{"ING-DiBa", "ID"},
		{"Commerzbank", "Co"},
		{"Frankfurter Volksbank eG", "FV"},
		// Legal form words alone must not become the monogram.
		{"Sparkasse AG", "Sp"},
		{"", "?"},
	} {
		if got := NewMonogram(tc.name, "TESTDEFFXXX").Initials; got != tc.want {
			t.Errorf("initials(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestMonogramIsDeterministic(t *testing.T) {
	a := NewMonogram("Deutsche Bank", "DEUTDEFFXXX")
	b := NewMonogram("Deutsche Bank", "DEUTDEFFXXX")
	if a != b {
		t.Errorf("monogram is not deterministic: %+v and %+v", a, b)
	}
	// Different institutions should generally not collide in colour, which is
	// the point of seeding from the BIC.
	c := NewMonogram("Commerzbank", "COBADEFFXXX")
	if a.Background == c.Background && a.Initials == c.Initials {
		t.Error("two different banks produced an identical monogram")
	}
}

// TestMonogramSVGEscapes covers bank names carrying XML significant
// characters. Names come from an external register, so they are untrusted.
func TestMonogramSVGEscapes(t *testing.T) {
	m := NewMonogram("<script>alert(1)</script> & Co", "TESTDEFFXXX")
	svg := m.SVG(128)

	if strings.Contains(svg, "<script") {
		t.Errorf("SVG contains unescaped markup: %s", svg)
	}
	for _, bad := range []string{">alert", "\"&\""} {
		if strings.Contains(svg, bad) {
			t.Errorf("SVG contains unescaped %q", bad)
		}
	}
	if !strings.HasPrefix(svg, "<svg") || !strings.HasSuffix(svg, "</svg>") {
		t.Errorf("SVG is malformed: %s", svg)
	}
}

func TestMonogramSVGSize(t *testing.T) {
	m := NewMonogram("Deutsche Bank", "DEUTDEFFXXX")
	if !strings.Contains(m.SVG(64), `width="64"`) {
		t.Error("requested size was not applied")
	}
	// A nonsensical size must still produce a valid document.
	if !strings.Contains(m.SVG(0), `width="128"`) {
		t.Error("zero size did not fall back to the default")
	}
}

func TestEscapeXML(t *testing.T) {
	got := escapeXML(`<&>"'`)
	want := "&lt;&amp;&gt;&quot;&apos;"
	if got != want {
		t.Errorf("escapeXML = %q, want %q", got, want)
	}
}
