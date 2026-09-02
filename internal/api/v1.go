package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/netzfabrikcom/iban-pizza/bankdata"
	"github.com/netzfabrikcom/iban-pizza/iban"
)

// The v1 surface reproduces openiban.com exactly: the same JSON field names,
// the same message strings, the same two space indentation and the same status
// codes. Clients migrating from that service must not have to change anything,
// so the quirks below are preserved on purpose rather than tidied up.

// v1BankData mirrors the upstream BankInfo JSON shape.
type v1BankData struct {
	BankCode string `json:"bankCode"`
	Name     string `json:"name"`
	Zip      string `json:"zip,omitempty"`
	City     string `json:"city,omitempty"`
	BIC      string `json:"bic,omitempty"`
}

// v1Result mirrors the upstream ValidationResult.
type v1Result struct {
	Valid        bool            `json:"valid"`
	Messages     []string        `json:"messages"`
	IBAN         string          `json:"iban"`
	BankData     v1BankData      `json:"bankData"`
	CheckResults map[string]bool `json:"checkResults"`
}

func newV1Result(valid bool, message, value string) *v1Result {
	messages := []string{}
	if message != "" {
		messages = append(messages, message)
	}
	return &v1Result{
		Valid:        valid,
		Messages:     messages,
		IBAN:         value,
		CheckResults: map[string]bool{},
	}
}

// v1BankCodeLength is the bank code length table the upstream service used.
//
// It covers only the seven countries it had data for. Extending it here would
// change the messages v1 emits for every other country, so it stays as it was.
var v1BankCodeLength = map[string]int{
	"DE": 8, "BE": 3, "NL": 4, "LU": 3, "CH": 5, "AT": 5, "LI": 5,
}

// maxV1Input bounds the path parameter before any work happens.
//
// The upstream handler cached on the raw parameter with no length check, so
// arbitrary requests grew the cache without limit.
const maxV1Input = 64

func (s *Server) handleV1Validate(w http.ResponseWriter, r *http.Request) {
	value := r.PathValue("iban")

	if len(value) > maxV1Input {
		writeJSONIndent(w, http.StatusBadRequest,
			newV1Result(false, "Cannot parse as IBAN: Invalid BBAN length.", ""))
		return
	}

	wantBankCode := v1Bool(r.FormValue("validateBankCode"))
	wantBIC := v1Bool(r.FormValue("getBIC"))

	writeJSONIndent(w, http.StatusOK, s.validateV1(r.Context(), value, wantBankCode, wantBIC))
}

// validateV1 produces the upstream response for one IBAN.
func (s *Server) validateV1(ctx context.Context, value string, wantBankCode, wantBIC bool) *v1Result {
	parsed, err := iban.Parse(value)
	if err != nil {
		return newV1Result(false, "Cannot parse as IBAN: "+v1ParseMessage(err, value), value)
	}

	result := newV1Result(true, "", value)
	if err := parsed.Validate(); err != nil {
		result.Valid = false
		result.Messages = append(result.Messages, "Validation failed.")
	}

	if wantBankCode {
		s.v1ValidateBankCode(ctx, parsed, result)
	}
	if wantBIC {
		s.v1GetBIC(ctx, parsed, result)
	}
	return result
}

func (s *Server) v1ValidateBankCode(ctx context.Context, parsed *iban.IBAN, result *v1Result) {
	cc := parsed.CountryCode()
	length, ok := v1BankCodeLength[cc]
	if !ok {
		result.CheckResults["bankCode"] = false
		result.Messages = append(result.Messages,
			"Cannot validate bank code length. No information available.")
		return
	}
	bban := parsed.BBAN()
	if len(bban) < length {
		result.CheckResults["bankCode"] = false
		result.Valid = false
		result.Messages = append(result.Messages,
			"Bank code validation impossible; Invalid bank code length for country '"+
				cc+"' (expected "+strconv.Itoa(length)+" digits)")
		return
	}

	code := bban[:length]
	if _, err := s.cfg.Repo.Find(ctx, bankdata.Key{Country: cc, BankCode: code}); err != nil {
		result.Valid = false
		result.CheckResults["bankCode"] = false
		result.Messages = append(result.Messages, "Invalid bank code: "+code)
		return
	}
	result.CheckResults["bankCode"] = true
	result.Messages = append(result.Messages, "Bank code valid: "+code)
}

func (s *Server) v1GetBIC(ctx context.Context, parsed *iban.IBAN, result *v1Result) {
	cc := parsed.CountryCode()
	length, ok := v1BankCodeLength[cc]
	if !ok {
		result.Messages = append(result.Messages, "Cannot get BIC. No information available.")
		return
	}
	bban := parsed.BBAN()
	if len(bban) < length {
		result.Messages = append(result.Messages, "Cannot get BIC for BBAN "+bban)
		return
	}

	code := bban[:length]
	bank, err := s.cfg.Repo.Find(ctx, bankdata.Key{Country: cc, BankCode: code})
	if err != nil {
		if !errors.Is(err, bankdata.ErrNotFound) {
			s.cfg.Logger.Error("bank lookup failed", "error", err)
		}
		result.Messages = append(result.Messages, "No BIC found for bank code: "+code)
		return
	}

	bic := bank.BIC
	// Commerzbank branches share a BIC that the register does not carry on
	// every record. The rule comes from the Bundesbank IBAN rules and is
	// reproduced from the upstream service.
	if cc == "DE" && len(bank.BankCode) > 6 && bank.BankCode[3:6] == "400" {
		bic = "COBADEFFXXX"
	}

	result.BankData = v1BankData{
		BankCode: bank.BankCode,
		Name:     bank.Name,
		Zip:      bank.Zip,
		City:     bank.City,
		BIC:      bic,
	}
}

// v1ParseMessage maps a parse error onto the message the upstream produced.
func v1ParseMessage(err error, value string) string {
	switch {
	case errors.Is(err, iban.ErrCountryCode), errors.Is(err, iban.ErrUnknownCountry):
		return "Invalid country code."
	case errors.Is(err, iban.ErrCheckDigits):
		return "Invalid / no check digits found."
	case errors.Is(err, iban.ErrTooShort), errors.Is(err, iban.ErrTooLong):
		return "Invalid BBAN length."
	case errors.Is(err, iban.ErrLength):
		cc := ""
		if len(value) >= 2 {
			cc = value[:2]
		}
		if c, ok := iban.Lookup(cc); ok {
			return "IBAN length invalid. Expected length for " + c.Code + " is " +
				strconv.Itoa(c.Length) + "."
		}
		return "Invalid BBAN length."
	default:
		return "Invalid BBAN length."
	}
}

// v1Bool mirrors the upstream truthiness: only "1" and "true" count.
func v1Bool(value string) bool { return value == "1" || value == "true" }

func (s *Server) handleV1Countries(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, countryNameToCode)
}

// v1Calculate is the success shape of the calculate endpoints.
type v1Calculate struct {
	Valid bool   `json:"valid"`
	IBAN  string `json:"iban"`
}

type v1CalculateError struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
}

func (s *Server) handleV1Calculate(w http.ResponseWriter, r *http.Request) {
	built, err := s.calculateV1(r)
	if err != nil {
		writeJSON(w, http.StatusOK, v1CalculateError{Valid: false, Message: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, v1Calculate{Valid: true, IBAN: built.String()})
}

func (s *Server) handleV1CalculateAndValidate(w http.ResponseWriter, r *http.Request) {
	built, err := s.calculateV1(r)
	if err != nil {
		writeJSON(w, http.StatusOK, v1CalculateError{Valid: false, Message: err.Error()})
		return
	}
	// The upstream delegated to validation with both extras switched on.
	writeJSONIndent(w, http.StatusOK, s.validateV1(r.Context(), built.String(), true, true))
}

func (s *Server) calculateV1(r *http.Request) (*iban.IBAN, error) {
	cc := r.PathValue("countryCode")
	bank := r.PathValue("bankCode")
	account := r.PathValue("accountNumber")

	if len(cc) > maxV1Input || len(bank) > maxV1Input || len(account) > maxV1Input {
		return nil, errors.New("Invalid country code.")
	}
	return iban.Calculate(cc, bank, account)
}
