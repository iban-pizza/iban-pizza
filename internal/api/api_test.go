package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/netzfabrikcom/iban-pizza/bankdata"
	"github.com/netzfabrikcom/iban-pizza/internal/logo"
	"github.com/netzfabrikcom/iban-pizza/internal/sepa"
)

func testServer(t *testing.T, mutate func(*Config)) http.Handler {
	t.Helper()

	store := bankdata.NewMemoryStore()
	info := bankdata.SourceInfo{Country: "DE", Name: "Deutsche Bundesbank", RetrievedAt: time.Now().UTC()}
	banks := []bankdata.Bank{
		{Country: "DE", BankCode: "50010517", Name: "ING-DiBa", Zip: "60628",
			City: "Frankfurt am Main", BIC: "INGDDEFFXXX", CheckAlgo: "C1", Source: info.Name},
		{Country: "DE", BankCode: "37040044", Name: "Commerzbank", Zip: "50447",
			City: "Koeln", BIC: "COBADEFFXXX", CheckAlgo: "13", Source: info.Name},
	}
	if err := store.Replace(context.Background(), info, banks); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	registry := sepa.NewRegistry()
	registry.Add(sepa.SCTInst, []sepa.Participant{
		{BIC: "INGDDEFFXXX", Name: "ING-DiBa AG", ReadinessDate: "2021-10-20"},
	})
	registry.Add(sepa.OCTInst, []sepa.Participant{
		{BIC: "COBADEFFXXX", Name: "Commerzbank AG", ReadinessDate: "2024-01-01"},
	})
	registry.SetAsOf(time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC))

	cfg := Config{
		Repo:    store,
		Schemes: registry,
		Logos:   logo.New(""),
		Version: "test",
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if mutate != nil {
		mutate(&cfg)
	}
	return New(cfg).Handler()
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode response: %v, body was %s", err, rec.Body.String())
	}
	return v
}

// TestV1ResponseShape locks the compatibility surface. Clients migrating from
// openiban.com compare these bodies, so field names, message strings and the
// two space indentation are part of the contract.
func TestV1ResponseShape(t *testing.T) {
	h := testServer(t, nil)

	rec := get(t, h, "/validate/DE89370400440532013000?validateBankCode=true&getBIC=true")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()

	if !strings.Contains(body, "\n  \"valid\": true") {
		t.Errorf("response is not indented with two spaces:\n%s", body)
	}
	for _, want := range []string{
		`"messages"`, `"iban"`, `"bankData"`, `"checkResults"`,
		`"bankCode": "37040044"`, `"bic": "COBADEFFXXX"`,
		"Bank code valid: 37040044",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("response is missing %q:\n%s", want, body)
		}
	}
}

func TestV1UnknownBankCode(t *testing.T) {
	h := testServer(t, nil)
	rec := get(t, h, "/validate/DE02120300000000202051?validateBankCode=true")

	result := decode[map[string]any](t, rec)
	if result["valid"] != false {
		t.Errorf("valid = %v, want false", result["valid"])
	}
	if !strings.Contains(rec.Body.String(), "Invalid bank code: 12030000") {
		t.Errorf("missing the upstream message:\n%s", rec.Body.String())
	}
}

// TestV1FormatVerbsAreNotInterpreted is the regression guard for the upstream
// bug where request data reached fmt.Fprintf as a format string.
func TestV1FormatVerbsAreNotInterpreted(t *testing.T) {
	h := testServer(t, nil)

	for _, input := range []string{"%s", "%!d(MISSING)", "%v%v%v", "DE89%s0400440532013000"} {
		rec := get(t, h, "/validate/"+strings.ReplaceAll(input, "%", "%25"))
		if rec.Code >= 500 {
			t.Errorf("input %q produced status %d", input, rec.Code)
		}
		body := rec.Body.String()
		for _, artefact := range []string{"%!", "(MISSING)", "(EXTRA"} {
			if strings.Contains(body, artefact) && !strings.Contains(input, artefact) {
				t.Errorf("input %q produced a formatting artefact: %s", input, body)
			}
		}
	}
}

// TestOverlongInputIsRejectedEarly guards the other upstream failure, where the
// response cache was keyed on unbounded request data.
func TestOverlongInputIsRejectedEarly(t *testing.T) {
	h := testServer(t, nil)

	long := strings.Repeat("9", 5000)
	if rec := get(t, h, "/validate/DE89"+long); rec.Code != http.StatusBadRequest {
		t.Errorf("v1 accepted an overlong input with status %d", rec.Code)
	}
	if rec := get(t, h, "/v2/iban/DE89"+long); rec.Code != http.StatusBadRequest {
		t.Errorf("v2 accepted an overlong input with status %d", rec.Code)
	}
}

func TestV1Calculate(t *testing.T) {
	h := testServer(t, nil)

	rec := get(t, h, "/calculate/DE/37040044/0532013000")
	result := decode[map[string]any](t, rec)
	if result["valid"] != true || result["iban"] != "DE89370400440532013000" {
		t.Errorf("calculate returned %v", result)
	}

	rec = get(t, h, "/calculate/ZZ/1/2")
	result = decode[map[string]any](t, rec)
	if result["valid"] != false {
		t.Errorf("invalid country returned %v", result)
	}
	if _, ok := result["message"]; !ok {
		t.Error("error response carries no message field")
	}
}

func TestV1Countries(t *testing.T) {
	h := testServer(t, nil)
	got := decode[map[string]string](t, get(t, h, "/countries"))

	if got["Germany"] != "DE" || got["Switzerland"] != "CH" {
		t.Errorf("countries map looks wrong: Germany=%q Switzerland=%q", got["Germany"], got["Switzerland"])
	}
	if len(got) < 90 {
		t.Errorf("countries map has %d entries, expected the full upstream list", len(got))
	}
}

// TestV2ReproducesAnIndependentChecker verifies the whole pipeline against the
// output of a third party IBAN checker for a real ING-DiBa account: correct
// length, valid bank code, wrong account check digit, correct IBAN checksum.
func TestV2ReproducesAnIndependentChecker(t *testing.T) {
	h := testServer(t, nil)
	res := decode[V2Result](t, get(t, h, "/v2/iban/DE49500105179144355668"))

	if res.Valid {
		t.Error("valid = true, but the account check digit is wrong")
	}
	expect := map[string]*bool{
		"length":        boolPtr(true),
		"bankCode":      boolPtr(true),
		"ibanChecksum":  boolPtr(true),
		"accountNumber": boolPtr(false),
	}
	for name, want := range expect {
		got, ok := res.Checks[name]
		if !ok {
			t.Errorf("check %q is missing", name)
			continue
		}
		if got.OK == nil || *got.OK != *want {
			t.Errorf("check %q = %v, want %v", name, got.OK, *want)
		}
	}
	if res.Checks["accountNumber"].Method != "C1" {
		t.Errorf("account check method = %q, want C1", res.Checks["accountNumber"].Method)
	}
	if res.Bank == nil || res.Bank.Name != "ING-DiBa" || res.Bank.BIC != "INGDDEFFXXX" {
		t.Errorf("bank = %+v", res.Bank)
	}
	if res.IBAN.Formatted != "DE49 5001 0517 9144 3556 68" {
		t.Errorf("formatted = %q", res.IBAN.Formatted)
	}
}

func boolPtr(b bool) *bool { return &b }

// TestV2UncheckedIsNotAFailure covers the distinction the Check type exists
// for. A null ok must not drag the overall result down.
func TestV2UncheckedIsNotAFailure(t *testing.T) {
	h := testServer(t, nil)

	// Austria has no bank data in this fixture, so the bank code cannot be
	// checked. The IBAN itself is well formed, so it must still be valid.
	res := decode[V2Result](t, get(t, h, "/v2/iban/AT611904300234573201"))
	if !res.Valid {
		t.Errorf("a well formed IBAN with no bank data was reported invalid: %+v", res.Checks)
	}
	if c := res.Checks["bankCode"]; c.OK != nil {
		t.Errorf("bank code check = %v, want no statement", c.OK)
	}
}

func TestV2SchemeStatuses(t *testing.T) {
	h := testServer(t, nil)

	// ING is in the register for SCT Inst but not for OCT Inst, which makes
	// the negative reliable.
	res := decode[V2Result](t, get(t, h, "/v2/iban/DE49500105179144355668"))
	if res.Schemes == nil {
		t.Fatal("no schemes block")
	}
	if got := res.Schemes.Schemes[sepa.SCTInst].Status; got != sepa.StatusParticipant {
		t.Errorf("SCT Inst = %q, want participant", got)
	}
	if got := res.Schemes.Schemes[sepa.OCTInst].Status; got != sepa.StatusNotParticipant {
		t.Errorf("OCT Inst = %q, want not_participant", got)
	}
	if res.Schemes.Level != "institution" {
		t.Errorf("level = %q, want institution", res.Schemes.Level)
	}
	if res.Schemes.Note == "" {
		t.Error("the schemes block carries no note explaining what unknown means")
	}
}

func TestV2Batch(t *testing.T) {
	h := testServer(t, nil)

	body := `{"ibans":["DE89370400440532013000","not-an-iban"]}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v2/iban:batch", strings.NewReader(body)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	out := decode[struct {
		Results []V2Result `json:"results"`
	}](t, rec)

	if len(out.Results) != 2 {
		t.Fatalf("returned %d results, want 2", len(out.Results))
	}
	if !out.Results[0].Valid {
		t.Error("the valid IBAN was rejected")
	}
	if out.Results[1].Valid {
		t.Error("the invalid input was accepted")
	}
}

func TestV2BatchLimits(t *testing.T) {
	h := testServer(t, nil)

	post := func(body string) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v2/iban:batch", strings.NewReader(body)))
		return rec.Code
	}

	if got := post(`{"ibans":[]}`); got != http.StatusBadRequest {
		t.Errorf("empty batch returned %d", got)
	}
	if got := post(`not json`); got != http.StatusBadRequest {
		t.Errorf("malformed body returned %d", got)
	}

	var many []string
	for range MaxBatchSize + 1 {
		many = append(many, "DE89370400440532013000")
	}
	payload, _ := json.Marshal(map[string]any{"ibans": many})
	if got := post(string(payload)); got != http.StatusBadRequest {
		t.Errorf("oversized batch returned %d", got)
	}
}

func TestV2BanksSearch(t *testing.T) {
	h := testServer(t, nil)

	out := decode[struct {
		Count int         `json:"count"`
		Banks []*BankView `json:"banks"`
	}](t, get(t, h, "/v2/banks?country=DE&bic=INGD"))

	if out.Count != 1 || out.Banks[0].Name != "ING-DiBa" {
		t.Errorf("search returned %+v", out)
	}

	if rec := get(t, h, "/v2/banks?limit=abc"); rec.Code != http.StatusBadRequest {
		t.Errorf("a non numeric limit returned %d", rec.Code)
	}
}

func TestV2LogoIsSVG(t *testing.T) {
	h := testServer(t, nil)

	rec := get(t, h, "/v2/banks/DE/50010517/logo.svg")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/svg+xml") {
		t.Errorf("content type = %q", ct)
	}
	if !strings.HasPrefix(rec.Body.String(), "<svg") {
		t.Errorf("body is not an SVG: %s", rec.Body.String())
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("nosniff header is missing on a user influenced document")
	}

	if rec := get(t, h, "/v2/banks/DE/99999999/logo.svg"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown bank returned %d", rec.Code)
	}
}

func TestHealthReportsFreshness(t *testing.T) {
	h := testServer(t, nil)
	got := decode[Health](t, get(t, h, "/healthz"))

	if got.Status != "ok" || got.Stale {
		t.Errorf("health = %+v", got)
	}
	if got.Records == 0 {
		t.Errorf("health reports no data: %+v", got)
	}
}

// TestHealthStaysSmall guards the split between liveness and provenance. The
// health body is polled by orchestrators and must not grow a per source list;
// that belongs to /v2/data.
func TestHealthStaysSmall(t *testing.T) {
	h := testServer(t, nil)
	body := decode[map[string]any](t, get(t, h, "/healthz"))

	for _, forbidden := range []string{"sources", "schemes"} {
		if _, ok := body[forbidden]; ok {
			t.Errorf("health carries %q, which belongs to /v2/data", forbidden)
		}
	}
}

// TestV2DataReportsProvenance covers the endpoint an integrator uses to judge
// an answer: which registry, retrieved when, how many records, how old.
func TestV2DataReportsProvenance(t *testing.T) {
	h := testServer(t, nil)
	rec := get(t, h, "/v2/data")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	got := decode[DataReport](t, rec)

	if got.Records == 0 {
		t.Error("no records reported")
	}
	if len(got.Sources) != 1 {
		t.Fatalf("reported %d sources, want 1", len(got.Sources))
	}
	src := got.Sources[0]
	if src.Country != "DE" || src.Name != "Deutsche Bundesbank" {
		t.Errorf("source = %+v", src)
	}
	if src.Records != 2 || src.AgeDays != 0 || src.Stale {
		t.Errorf("source metadata = %+v", src)
	}
	if src.RetrievedAt == "" {
		t.Error("retrievedAt is empty")
	}
	if len(got.Countries) != 1 || got.Countries[0] != "DE" {
		t.Errorf("countries = %v", got.Countries)
	}
	if got.Stale {
		t.Error("fresh data reported as stale")
	}
	if got.StaleAfter == "" {
		t.Error("staleAfter is empty")
	}

	if !got.Schemes.Loaded || got.Schemes.Institutions == 0 {
		t.Errorf("schemes = %+v", got.Schemes)
	}
	if got.Schemes.Participants["sctInst"] != 1 {
		t.Errorf("sctInst participants = %d, want 1", got.Schemes.Participants["sctInst"])
	}
	if got.Schemes.AsOf != "2026-08-07" {
		t.Errorf("schemes asOf = %q", got.Schemes.AsOf)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}
}

func TestV2DataWithoutSchemes(t *testing.T) {
	h := testServer(t, func(c *Config) { c.Schemes = nil })
	got := decode[DataReport](t, get(t, h, "/v2/data"))
	if got.Schemes.Loaded {
		t.Error("schemes reported as loaded with no register configured")
	}
}

func TestV2DataReportsStaleSource(t *testing.T) {
	store := bankdata.NewMemoryStore()
	old := time.Now().Add(-3 * 365 * 24 * time.Hour)
	_ = store.Replace(context.Background(),
		bankdata.SourceInfo{Country: "DE", Name: "Deutsche Bundesbank",
			URL: "https://example.invalid/blz", RetrievedAt: old},
		[]bankdata.Bank{{Country: "DE", BankCode: "37040044", Name: "Commerzbank", Source: "Deutsche Bundesbank"}})

	h := New(Config{Repo: store, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}).Handler()
	got := decode[DataReport](t, get(t, h, "/v2/data"))

	if !got.Stale || !got.Sources[0].Stale {
		t.Errorf("three year old source not reported stale: %+v", got)
	}
	if got.Sources[0].AgeDays < 1000 {
		t.Errorf("ageDays = %d for a three year old source", got.Sources[0].AgeDays)
	}
	if got.Sources[0].URL != "https://example.invalid/blz" {
		t.Errorf("url = %q, provenance lost", got.Sources[0].URL)
	}
}

// TestHealthReportsStaleData is what the upstream project lacked: data that
// silently froze in 2019 while the service kept answering.
func TestHealthReportsStaleData(t *testing.T) {
	store := bankdata.NewMemoryStore()
	old := time.Now().Add(-3 * 365 * 24 * time.Hour)
	_ = store.Replace(context.Background(),
		bankdata.SourceInfo{Country: "DE", Name: "Deutsche Bundesbank", RetrievedAt: old},
		[]bankdata.Bank{{Country: "DE", BankCode: "37040044", Name: "Commerzbank", Source: "Deutsche Bundesbank"}})

	h := New(Config{Repo: store, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}).Handler()
	got := decode[Health](t, get(t, h, "/healthz"))

	if !got.Stale || got.Status != "stale" {
		t.Errorf("three year old data was not reported as stale: %+v", got)
	}
	// It must still serve: stale data is a warning, not an outage.
	if rec := get(t, h, "/healthz"); rec.Code != http.StatusOK {
		t.Errorf("stale data made health return %d", rec.Code)
	}
}

func TestReadyRequiresData(t *testing.T) {
	h := testServer(t, nil)
	if rec := get(t, h, "/readyz"); rec.Code != http.StatusOK {
		t.Errorf("ready returned %d with data loaded", rec.Code)
	}

	empty := New(Config{
		Repo:   bankdata.NewMemoryStore(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler()
	if rec := get(t, empty, "/readyz"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("ready returned %d with no data", rec.Code)
	}
}

func TestOpenAPIIsServed(t *testing.T) {
	h := testServer(t, nil)
	rec := get(t, h, "/openapi.yaml")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(body, "openapi: 3.1") {
		t.Errorf("body does not start with an OpenAPI version: %.60s", body)
	}
	// Every route the server registers should appear in the description.
	for _, path := range []string{"/v2/iban/{iban}", "/validate/{iban}", "/healthz", "/v2/banks", "/v2/data"} {
		if !strings.Contains(body, path) {
			t.Errorf("the spec does not document %s", path)
		}
	}
}

// TestCORSIsNotOpenByDefault covers the deliberate change from the upstream
// service, which sent a wildcard on every response.
func TestCORSIsNotOpenByDefault(t *testing.T) {
	h := testServer(t, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v2/iban/DE89370400440532013000", nil)
	req.Header.Set("Origin", "https://example.com")
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("cross origin access granted by default: %q", got)
	}
}

func TestCORSAllowsConfiguredOrigins(t *testing.T) {
	h := testServer(t, func(c *Config) {
		c.AllowedOrigins = []string{"https://allowed.example"}
	})

	for _, tc := range []struct{ origin, want string }{
		{"https://allowed.example", "https://allowed.example"},
		{"https://other.example", ""},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.Header.Set("Origin", tc.origin)
		h.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tc.want {
			t.Errorf("origin %s got %q, want %q", tc.origin, got, tc.want)
		}
	}

	// A per origin response must not be cached across origins.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "https://allowed.example")
	h.ServeHTTP(rec, req)
	if !strings.Contains(rec.Header().Get("Vary"), "Origin") {
		t.Error("Vary: Origin is missing on an origin specific response")
	}
}

func TestRateLimit(t *testing.T) {
	h := testServer(t, func(c *Config) { c.RateLimit = 3 })

	var limited bool
	for range 10 {
		if get(t, h, "/healthz").Code == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Error("the rate limit never triggered")
	}
}

func TestLimiterWindowExpires(t *testing.T) {
	l := newLimiter(2)
	now := time.Now()

	if !l.allow("client", now) || !l.allow("client", now) {
		t.Fatal("the first two requests were refused")
	}
	if l.allow("client", now) {
		t.Error("the third request within the window was allowed")
	}
	// A new window must let the client through again, and the old entry must
	// not be kept forever.
	if !l.allow("client", now.Add(2*time.Minute)) {
		t.Error("the window did not reset")
	}
}

func TestUnknownRouteIs404(t *testing.T) {
	h := testServer(t, nil)
	if rec := get(t, h, "/nope"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown route returned %d", rec.Code)
	}
}
