// Package sepa reports which EPC payment schemes an institution has joined.
//
// The data comes from the EPC Register of Participants, which is keyed by the
// full eleven character BIC. It describes scheme adherence at the level of the
// institution, not what a particular account or product can do. ING-DiBa is
// listed for SDD B2B yet does not offer business direct debit to retail
// customers; both statements are true, and the API says which one it reports.
package sepa

import (
	"sort"
	"strings"
	"time"
)

// Scheme identifies one EPC payment scheme.
type Scheme string

const (
	SCT     Scheme = "sct"     // SEPA Credit Transfer
	SCTInst Scheme = "sctInst" // SEPA Instant Credit Transfer
	SDDCore Scheme = "sddCore" // SEPA Direct Debit, core
	SDDB2B  Scheme = "sddB2B"  // SEPA Direct Debit, business to business
	OCTInst Scheme = "octInst" // One-Leg Out Instant Credit Transfer
	VOP     Scheme = "vop"     // Verification of Payee
)

// AllSchemes lists every scheme in a stable order.
var AllSchemes = []Scheme{SCT, SCTInst, SDDCore, SDDB2B, OCTInst, VOP}

// file returns the EPC export basename for a scheme.
func (s Scheme) file() string {
	switch s {
	case SCTInst:
		return "sct_inst"
	case SDDCore:
		return "sdd_core"
	case SDDB2B:
		return "sdd_b2b"
	case OCTInst:
		return "oct_inst"
	default:
		return string(s)
	}
}

// Status is how confident the answer is for one scheme.
//
// The distinction between NotParticipant and Unknown is the point of this
// type. Only about a third of German institutions appear in the register at
// all; the rest reach SEPA through a central institution without being listed
// individually. Reporting those as "not supported" would be wrong for roughly
// two thirds of German banks.
type Status string

const (
	// Participant means the institution is listed for this scheme.
	StatusParticipant Status = "participant"

	// StatusNotParticipant means the institution appears in the register for
	// other schemes but not this one. That is a reliable negative.
	StatusNotParticipant Status = "not_participant"

	// StatusLeft means the register records a scheme leaving date.
	StatusLeft Status = "left"

	// StatusUnknown means the BIC is in no register at all. The institution
	// may still be reachable indirectly through a central institution, so this
	// must not be read as "not supported".
	StatusUnknown Status = "unknown"
)

// Membership is one institution's position in one scheme.
type Membership struct {
	Status        Status `json:"status"`
	ReadinessDate string `json:"readinessDate,omitempty"`
	LeavingDate   string `json:"leavingDate,omitempty"`

	// Role is populated for Verification of Payee, where an institution can be
	// a requesting party, a responding party, or both.
	Role string `json:"role,omitempty"`
}

// Participant is one row of the register.
type Participant struct {
	BIC           string
	Name          string
	Country       string
	City          string
	ReadinessDate string
	LeavingDate   string
	Role          string
}

// Registry answers scheme membership questions.
type Registry struct {
	// bySchemeBIC[scheme][bic] is the membership for one institution.
	bySchemeBIC map[Scheme]map[string]Membership

	// known holds every BIC that appears in any scheme, which is what makes a
	// confident negative possible.
	known map[string]bool

	asOf time.Time
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		bySchemeBIC: make(map[Scheme]map[string]Membership, len(AllSchemes)),
		known:       make(map[string]bool),
	}
}

// AsOf reports when the register was retrieved.
func (r *Registry) AsOf() time.Time { return r.asOf }

// SetAsOf records the retrieval time.
func (r *Registry) SetAsOf(t time.Time) { r.asOf = t }

// Add stores the participants of one scheme.
func (r *Registry) Add(scheme Scheme, participants []Participant) {
	m := make(map[string]Membership, len(participants))
	for _, p := range participants {
		bic := NormalizeBIC(p.BIC)
		if bic == "" {
			continue
		}
		status := StatusParticipant
		if p.LeavingDate != "" {
			status = StatusLeft
		}
		m[bic] = Membership{
			Status:        status,
			ReadinessDate: p.ReadinessDate,
			LeavingDate:   p.LeavingDate,
			Role:          p.Role,
		}
		r.known[bic] = true
	}
	r.bySchemeBIC[scheme] = m
}

// Lookup returns the membership of a BIC in every scheme.
//
// A BIC that appears nowhere in the register yields StatusUnknown for all
// schemes rather than a set of negatives.
func (r *Registry) Lookup(bic string) map[Scheme]Membership {
	bic = NormalizeBIC(bic)
	out := make(map[Scheme]Membership, len(AllSchemes))

	inRegister := r.known[bic]
	for _, scheme := range AllSchemes {
		if m, ok := r.bySchemeBIC[scheme][bic]; ok {
			out[scheme] = m
			continue
		}
		if inRegister {
			out[scheme] = Membership{Status: StatusNotParticipant}
		} else {
			out[scheme] = Membership{Status: StatusUnknown}
		}
	}
	return out
}

// Known reports whether a BIC appears in the register at all.
func (r *Registry) Known(bic string) bool { return r.known[NormalizeBIC(bic)] }

// Len returns the number of distinct institutions in the register.
func (r *Registry) Len() int { return len(r.known) }

// Counts returns the number of participants per scheme, used by the update
// command and by the staleness report.
func (r *Registry) Counts() map[Scheme]int {
	out := make(map[Scheme]int, len(r.bySchemeBIC))
	for scheme, m := range r.bySchemeBIC {
		out[scheme] = len(m)
	}
	return out
}

// Schemes lists the schemes that hold data, in stable order.
func (r *Registry) Schemes() []Scheme {
	out := make([]Scheme, 0, len(r.bySchemeBIC))
	for s := range r.bySchemeBIC {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// NormalizeBIC upper-cases a BIC and strips spaces.
//
// It does not pad an eight character BIC to eleven. The register uses full
// eleven character BICs, and padding would invent a branch that may not exist.
func NormalizeBIC(bic string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(bic), " ", ""))
}
