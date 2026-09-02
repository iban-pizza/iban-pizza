package sepa

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name+".csv"))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// loadedRegistry builds a registry from the four fixtures. ING-DiBa appears in
// sct_inst, sdd_b2b and vop but deliberately not in oct_inst, which is what
// makes the confident negative testable.
func loadedRegistry(t *testing.T) *Registry {
	t.Helper()
	r := NewRegistry()
	for scheme, file := range map[Scheme]string{
		SCTInst: "sct_inst",
		SDDB2B:  "sdd_b2b",
		OCTInst: "oct_inst",
		VOP:     "vop",
	} {
		p, err := ParseCSV(loadFixture(t, file))
		if err != nil {
			t.Fatalf("ParseCSV(%s) returned %v", file, err)
		}
		r.Add(scheme, p)
	}
	r.SetAsOf(time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC))
	return r
}

// TestParseHandlesDifferentColumnLayouts is the regression guard for the EPC
// exports not sharing a schema. The four fixtures have eight, nine and ten
// columns; a parser that read fields by position would take the wrong values
// for at least two of them.
func TestParseHandlesDifferentColumnLayouts(t *testing.T) {
	cases := []struct {
		file        string
		wantRole    bool
		wantColumns int
	}{
		{"sct_inst", false, 8},
		{"sdd_b2b", false, 8},
		{"oct_inst", true, 9},
		{"vop", true, 10},
	}
	for _, tc := range cases {
		participants, err := ParseCSV(loadFixture(t, tc.file))
		if err != nil {
			t.Errorf("ParseCSV(%s) returned %v", tc.file, err)
			continue
		}
		if len(participants) == 0 {
			t.Errorf("ParseCSV(%s) returned no rows", tc.file)
			continue
		}
		for _, p := range participants {
			if p.BIC == "" {
				t.Errorf("%s: a row parsed without a BIC", tc.file)
			}
			if p.Name == "" {
				t.Errorf("%s: %s parsed without a name", tc.file, p.BIC)
			}
			// A leaving date must never pick up role text, which is what
			// happens when columns are read positionally.
			if p.LeavingDate != "" && len(p.LeavingDate) != 10 {
				t.Errorf("%s: %s has leaving date %q, which is not a date",
					tc.file, p.BIC, p.LeavingDate)
			}
		}
		if tc.wantRole {
			found := false
			for _, p := range participants {
				if p.Role != "" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s has a Role column but no row carried a role", tc.file)
			}
		}
	}
}

func TestVOPColumnsAreNotShifted(t *testing.T) {
	participants, err := ParseCSV(loadFixture(t, "vop"))
	if err != nil {
		t.Fatalf("ParseCSV returned %v", err)
	}
	var ing Participant
	for _, p := range participants {
		if p.BIC == "INGDDEFFXXX" {
			ing = p
		}
	}
	if ing.BIC == "" {
		t.Fatal("ING-DiBa is missing from the VOP fixture")
	}
	if ing.ReadinessDate != "2025-10-09" {
		t.Errorf("ReadinessDate = %q, want 2025-10-09", ing.ReadinessDate)
	}
	if ing.Role != "Requesting PSP;Responding PSP" {
		t.Errorf("Role = %q, want the role text", ing.Role)
	}
	// The trap: reading position seven as the leaving date puts the role
	// string into it.
	if ing.LeavingDate != "" {
		t.Errorf("LeavingDate = %q, want empty", ing.LeavingDate)
	}
}

// TestFourStateStatus covers the distinction the whole package exists for.
func TestFourStateStatus(t *testing.T) {
	r := loadedRegistry(t)

	got := r.Lookup("INGDDEFFXXX")

	// Listed for this scheme.
	if got[SCTInst].Status != StatusParticipant {
		t.Errorf("SCT Inst = %q, want participant", got[SCTInst].Status)
	}
	if got[SDDB2B].Status != StatusParticipant {
		t.Errorf("SDD B2B = %q, want participant", got[SDDB2B].Status)
	}

	// In the register, but not in this scheme. That is a reliable negative.
	if got[OCTInst].Status != StatusNotParticipant {
		t.Errorf("OCT Inst = %q, want not_participant", got[OCTInst].Status)
	}

	// A BIC that appears in no register at all. Many German savings and
	// cooperative banks are reachable through a central institution without
	// being listed, so this must not become a negative.
	unknown := r.Lookup("GENODEF1XXX")
	for scheme, m := range unknown {
		if m.Status != StatusUnknown {
			t.Errorf("unlisted BIC has %s = %q, want unknown", scheme, m.Status)
		}
	}
}

func TestLookupCoversEveryScheme(t *testing.T) {
	r := loadedRegistry(t)
	got := r.Lookup("INGDDEFFXXX")
	if len(got) != len(AllSchemes) {
		t.Fatalf("Lookup returned %d schemes, want %d", len(got), len(AllSchemes))
	}
	for _, s := range AllSchemes {
		if _, ok := got[s]; !ok {
			t.Errorf("scheme %s missing from the result", s)
		}
	}
	// Schemes that were never loaded must report unknown rather than be absent.
	if got[SCT].Status != StatusNotParticipant && got[SCT].Status != StatusUnknown {
		t.Errorf("unloaded scheme SCT = %q", got[SCT].Status)
	}
}

func TestNormalizeBIC(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"ingddeffxxx", "INGDDEFFXXX"},
		{" INGD DEFF XXX ", "INGDDEFFXXX"},
		{"", ""},
		// An eight character BIC must not be padded to eleven, which would
		// invent a branch that may not exist.
		{"INGDDEFF", "INGDDEFF"},
	} {
		if got := NormalizeBIC(tc.in); got != tc.want {
			t.Errorf("NormalizeBIC(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	if _, err := ParseCSV(nil); err == nil {
		t.Error("ParseCSV accepted an empty file")
	}
	if _, err := ParseCSV([]byte("\"Country\",\"Name\"\n\"DE\",\"x\"\n")); err == nil {
		t.Error("ParseCSV accepted a file with no BIC column")
	}
	if _, err := ParseCSV([]byte("\"Country\",\"BIC\"\n")); err == nil {
		t.Error("ParseCSV accepted a header with no rows")
	}
}

func TestSchemeURL(t *testing.T) {
	for _, tc := range []struct {
		scheme Scheme
		want   string
	}{
		{SCTInst, BaseURL + "/sct_inst/sct_inst.csv"},
		{SDDB2B, BaseURL + "/sdd_b2b/sdd_b2b.csv"},
		{SCT, BaseURL + "/sct/sct.csv"},
		{VOP, BaseURL + "/vop/vop.csv"},
	} {
		if got := URL(tc.scheme); got != tc.want {
			t.Errorf("URL(%s) = %q, want %q", tc.scheme, got, tc.want)
		}
	}
}

func TestRegistryCounts(t *testing.T) {
	r := loadedRegistry(t)
	counts := r.Counts()
	if counts[SCTInst] == 0 {
		t.Error("SCT Inst count is zero")
	}
	if r.Len() == 0 {
		t.Error("registry reports no known institutions")
	}
	if !r.Known("INGDDEFFXXX") {
		t.Error("ING-DiBa should be known")
	}
	if r.Known("ZZZZZZZZZZZ") {
		t.Error("an invented BIC should not be known")
	}
}
