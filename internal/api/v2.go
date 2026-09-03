package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/iban-pizza/iban-pizza/bankdata"
	"github.com/iban-pizza/iban-pizza/iban"
	"github.com/iban-pizza/iban-pizza/internal/checkdigit"
	"github.com/iban-pizza/iban-pizza/internal/logo"
	"github.com/iban-pizza/iban-pizza/internal/sepa"
)

// MaxBatchSize bounds a batch request.
const MaxBatchSize = 100

// Check is one named check with its outcome.
//
// OK is a pointer so that "not checked" is distinct from "failed". A missing
// check must never read as a failure, which is the same rule the check digit
// package follows.
type Check struct {
	OK      *bool  `json:"ok"`
	Message string `json:"message"`
	Method  string `json:"method,omitempty"`
}

func ok(msg string) Check      { t := true; return Check{OK: &t, Message: msg} }
func notOK(msg string) Check   { f := false; return Check{OK: &f, Message: msg} }
func unchecked(m string) Check { return Check{OK: nil, Message: m} }

// IBANParts describes the parsed IBAN.
type IBANParts struct {
	Input         string `json:"input"`
	Formatted     string `json:"formatted,omitempty"`
	CountryCode   string `json:"countryCode,omitempty"`
	CheckDigits   string `json:"checkDigits,omitempty"`
	BankCode      string `json:"bankCode,omitempty"`
	BranchCode    string `json:"branchCode,omitempty"`
	AccountNumber string `json:"accountNumber,omitempty"`
}

// BankView is the bank data returned by v2.
type BankView struct {
	Country   string    `json:"country"`
	BankCode  string    `json:"bankCode"`
	Name      string    `json:"name"`
	ShortName string    `json:"shortName,omitempty"`
	Zip       string    `json:"zip,omitempty"`
	City      string    `json:"city,omitempty"`
	BIC       string    `json:"bic,omitempty"`
	Source    string    `json:"source,omitempty"`
	Logo      logo.Logo `json:"logo"`
}

// SchemeView reports EPC scheme membership.
type SchemeView struct {
	// Level states what the statuses describe. Scheme adherence is recorded
	// per institution, not per account, and callers have to know that: an
	// institution can be a scheme participant while not offering the product
	// to a given customer.
	Level string `json:"level"`

	Schemes map[sepa.Scheme]sepa.Membership `json:"schemes"`
	Source  string                          `json:"source"`
	AsOf    string                          `json:"asOf,omitempty"`
	Note    string                          `json:"note"`
}

// V2Result is the v2 response for one IBAN.
type V2Result struct {
	Valid    bool              `json:"valid"`
	IBAN     IBANParts         `json:"iban"`
	Checks   map[string]Check  `json:"checks"`
	Bank     *BankView         `json:"bank,omitempty"`
	Schemes  *SchemeView       `json:"schemes,omitempty"`
	DataAsOf map[string]string `json:"dataAsOf,omitempty"`
}

func (s *Server) handleV2IBAN(w http.ResponseWriter, r *http.Request) {
	value := r.PathValue("iban")
	if len(value) > maxV1Input {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "iban is too long"})
		return
	}
	writeJSON(w, http.StatusOK, s.validateV2(r.Context(), value))
}

// batchRequest is the body of the batch endpoint.
type batchRequest struct {
	IBANs []string `json:"ibans"`
}

func (s *Server) handleV2Batch(w http.ResponseWriter, r *http.Request) {
	var req batchRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "invalid JSON body"})
		return
	}
	if len(req.IBANs) == 0 {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "ibans must not be empty"})
		return
	}
	if len(req.IBANs) > MaxBatchSize {
		writeJSON(w, http.StatusBadRequest, errorBody{
			Error: fmt.Sprintf("a batch may hold at most %d entries", MaxBatchSize)})
		return
	}

	results := make([]V2Result, 0, len(req.IBANs))
	for _, value := range req.IBANs {
		if len(value) > maxV1Input {
			results = append(results, V2Result{
				IBAN:   IBANParts{Input: value},
				Checks: map[string]Check{"length": notOK("Input is too long to be an IBAN")},
			})
			continue
		}
		results = append(results, s.validateV2(r.Context(), value))
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// validateV2 assembles the full v2 answer for one IBAN.
func (s *Server) validateV2(ctx context.Context, value string) V2Result {
	res := V2Result{
		IBAN:   IBANParts{Input: value},
		Checks: map[string]Check{},
	}

	parsed, err := iban.Parse(value)
	if err != nil {
		res.Checks["length"] = notOK(v2ParseMessage(err))
		return res
	}

	res.IBAN = IBANParts{
		Input:         value,
		Formatted:     parsed.Formatted(),
		CountryCode:   parsed.CountryCode(),
		CheckDigits:   parsed.CheckDigits(),
		BankCode:      parsed.BankCode(),
		BranchCode:    parsed.BranchCode(),
		AccountNumber: parsed.AccountNumber(),
	}

	country := parsed.Country()
	res.Checks["length"] = ok(fmt.Sprintf("Correct length for %s (%d characters)",
		country.Code, country.Length))

	checksumOK := parsed.Validate() == nil
	if checksumOK {
		res.Checks["ibanChecksum"] = ok("The IBAN checksum is correct")
	} else {
		res.Checks["ibanChecksum"] = notOK("The IBAN checksum is wrong")
	}

	bank := s.lookupBank(ctx, parsed, res.Checks)
	if bank != nil {
		res.Bank = s.bankView(*bank)
		res.Schemes = s.schemeView(bank.BIC)
	}

	res.Valid = allChecksPass(res.Checks) && checksumOK
	res.DataAsOf = s.dataAsOf(ctx)
	return res
}

// lookupBank resolves the bank and records the bank code and account number
// checks that depend on it.
func (s *Server) lookupBank(ctx context.Context, parsed *iban.IBAN, checks map[string]Check) *bankdata.Bank {
	code := parsed.BankCode()
	if code == "" {
		checks["bankCode"] = unchecked(
			"The registry does not record where the bank code sits for " + parsed.CountryCode())
		return nil
	}

	bank, err := s.cfg.Repo.Find(ctx, bankdata.Key{Country: parsed.CountryCode(), BankCode: code})
	if err != nil {
		if !errors.Is(err, bankdata.ErrNotFound) {
			s.cfg.Logger.Error("bank lookup failed", "error", err)
			checks["bankCode"] = unchecked("Bank data is unavailable")
			return nil
		}
		// No data for the country at all is a different statement from a bank
		// code that is genuinely unknown within a country that is covered.
		if !s.haveCountry(ctx, parsed.CountryCode()) {
			checks["bankCode"] = unchecked(
				"No bank directory is loaded for " + parsed.CountryCode())
			return nil
		}
		checks["bankCode"] = notOK("Unknown bank code " + code)
		return nil
	}

	checks["bankCode"] = ok("Bank code " + code + " is valid")
	checks["accountNumber"] = s.accountCheck(bank, parsed.AccountNumber())
	return &bank
}

// accountCheck runs the national account check digit method.
func (s *Server) accountCheck(bank bankdata.Bank, account string) Check {
	if bank.Country != "DE" || bank.CheckAlgo == "" || account == "" {
		return unchecked("No account number check is available for this bank")
	}

	result, err := checkdigit.Validate(bank.CheckAlgo, account)
	if err != nil {
		return unchecked("The account number could not be checked")
	}

	switch result {
	case checkdigit.ResultValid:
		c := ok("Account number " + account + " has a correct check digit")
		c.Method = bank.CheckAlgo
		return c
	case checkdigit.ResultInvalid:
		c := notOK("Account number " + account + " has a wrong check digit")
		c.Method = bank.CheckAlgo
		return c
	default:
		c := unchecked("Check digit method " + bank.CheckAlgo + " is not verified, so no statement is made")
		c.Method = bank.CheckAlgo
		return c
	}
}

// haveCountry reports whether any bank data is loaded for a country.
func (s *Server) haveCountry(ctx context.Context, country string) bool {
	stats, err := s.cfg.Repo.Stats(ctx)
	if err != nil {
		return false
	}
	for _, src := range stats.Sources {
		if src.Country == country {
			return true
		}
	}
	return false
}

// allChecksPass reports whether no check failed. Unchecked entries do not
// count against the result.
func allChecksPass(checks map[string]Check) bool {
	for _, c := range checks {
		if c.OK != nil && !*c.OK {
			return false
		}
	}
	return true
}

func (s *Server) bankView(b bankdata.Bank) *BankView {
	return &BankView{
		Country:   b.Country,
		BankCode:  b.BankCode,
		Name:      b.Name,
		ShortName: b.ShortName,
		Zip:       b.Zip,
		City:      b.City,
		BIC:       b.BIC,
		Source:    b.Source,
		Logo:      s.cfg.Logos.For(b.Country, b.BankCode, b.BIC, b.Name),
	}
}

// schemeView reports scheme membership, or nil when no register is loaded.
func (s *Server) schemeView(bic string) *SchemeView {
	if s.cfg.Schemes == nil || bic == "" {
		return nil
	}
	v := &SchemeView{
		Level:   "institution",
		Schemes: s.cfg.Schemes.Lookup(bic),
		Source:  "EPC Register of Participants",
		Note: "Status describes the institution's scheme adherence, not whether a " +
			"particular account or product supports it. A status of unknown means the " +
			"BIC is absent from the register; such an institution may still be reachable " +
			"through a central institution, so unknown must not be read as unsupported.",
	}
	if t := s.cfg.Schemes.AsOf(); !t.IsZero() {
		v.AsOf = t.Format("2006-01-02")
	}
	return v
}

// dataAsOf reports the retrieval date of each loaded source.
func (s *Server) dataAsOf(ctx context.Context) map[string]string {
	stats, err := s.cfg.Repo.Stats(ctx)
	if err != nil || len(stats.Sources) == 0 {
		return nil
	}
	out := make(map[string]string, len(stats.Sources)+1)
	for _, src := range stats.Sources {
		out[src.Country] = src.RetrievedAt.Format("2006-01-02")
	}
	if s.cfg.Schemes != nil {
		if t := s.cfg.Schemes.AsOf(); !t.IsZero() {
			out["schemes"] = t.Format("2006-01-02")
		}
	}
	return out
}

func v2ParseMessage(err error) string {
	switch {
	case errors.Is(err, iban.ErrTooShort):
		return "Input is too short to be an IBAN"
	case errors.Is(err, iban.ErrTooLong):
		return "Input is too long to be an IBAN"
	case errors.Is(err, iban.ErrCountryCode):
		return "The first two characters are not a country code"
	case errors.Is(err, iban.ErrCheckDigits):
		return "Characters three and four are not check digits"
	case errors.Is(err, iban.ErrUnknownCountry):
		return "This country does not issue IBANs, or is not in the registry"
	default:
		return err.Error()
	}
}

func (s *Server) handleV2Banks(w http.ResponseWriter, r *http.Request) {
	q := bankdata.Query{
		Country: r.URL.Query().Get("country"),
		BIC:     r.URL.Query().Get("bic"),
		Name:    r.URL.Query().Get("name"),
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorBody{Error: "limit must be a number"})
			return
		}
		q.Limit = n
	}

	banks, err := s.cfg.Repo.Search(r.Context(), q)
	if err != nil {
		s.cfg.Logger.Error("search failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorBody{Error: "search failed"})
		return
	}

	out := make([]*BankView, 0, len(banks))
	for _, b := range banks {
		out = append(out, s.bankView(b))
	}
	writeJSON(w, http.StatusOK, map[string]any{"banks": out, "count": len(out)})
}

func (s *Server) handleV2Bank(w http.ResponseWriter, r *http.Request) {
	bank, err := s.findPathBank(r)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorBody{Error: "no such bank"})
		return
	}
	writeJSON(w, http.StatusOK, s.bankView(bank))
}

func (s *Server) handleV2Logo(w http.ResponseWriter, r *http.Request) {
	bank, err := s.findPathBank(r)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorBody{Error: "no such bank"})
		return
	}

	size := 128
	if v := r.URL.Query().Get("size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 16 && n <= 512 {
			size = n
		}
	}

	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// The mark is derived from data that changes at most quarterly.
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write([]byte(logo.NewMonogram(bank.Name, bank.BIC).SVG(size)))
}

func (s *Server) findPathBank(r *http.Request) (bankdata.Bank, error) {
	country := strings.ToUpper(r.PathValue("country"))
	code := r.PathValue("bankCode")
	if len(country) != 2 || code == "" || len(code) > 32 {
		return bankdata.Bank{}, bankdata.ErrNotFound
	}
	return s.cfg.Repo.Find(r.Context(), bankdata.Key{Country: country, BankCode: code})
}

// v2Country describes one registry entry.
type v2Country struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Length      int    `json:"ibanLength"`
	BBAN        string `json:"bbanStructure,omitempty"`
	HasBankData bool   `json:"hasBankData"`
}

func (s *Server) handleV2Countries(w http.ResponseWriter, r *http.Request) {
	loaded := map[string]bool{}
	if stats, err := s.cfg.Repo.Stats(r.Context()); err == nil {
		for _, src := range stats.Sources {
			loaded[src.Country] = true
		}
	}

	codes := iban.Countries()
	out := make([]v2Country, 0, len(codes))
	for _, code := range codes {
		c, _ := iban.Lookup(code)
		out = append(out, v2Country{
			Code:        c.Code,
			Name:        c.Name,
			Length:      c.Length,
			BBAN:        c.BBAN,
			HasBankData: loaded[c.Code],
		})
	}
	// Countries() has no defined order, so sort for a stable response.
	sortCountries(out)
	writeJSON(w, http.StatusOK, map[string]any{"countries": out, "count": len(out)})
}

func sortCountries(c []v2Country) {
	for i := 1; i < len(c); i++ {
		for j := i; j > 0 && c[j].Code < c[j-1].Code; j-- {
			c[j], c[j-1] = c[j-1], c[j]
		}
	}
}
