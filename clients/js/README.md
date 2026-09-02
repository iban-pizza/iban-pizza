# iban-pizza

Client for the [iban.pizza](https://iban.pizza) API: IBAN validation, bank
lookup and SEPA scheme membership. Works against the public instance or your
own.

No runtime dependencies. Uses the platform `fetch` (Node 18+, browsers,
Deno, Bun). Response types are generated from the service's `openapi.yaml`
at build time, so they match the API by construction.

```sh
npm install iban-pizza
```

```ts
import { IbanPizza } from "iban-pizza";

const api = new IbanPizza({ baseUrl: "https://iban.pizza" });

const r = await api.validate("DE89 3704 0044 0532 0130 00");
r.valid;                       // true
r.bank?.name;                  // "Commerzbank"
r.checks.accountNumber.ok;     // true, false, or null when no method applies
r.schemes?.schemes.sctInst;    // { status: "participant", readinessDate: "..." }
```

## What is there

| Method | Endpoint |
|---|---|
| `validate(iban)` | `GET /v2/iban/{iban}` |
| `validateMany(ibans)` | `POST /v2/iban:batch` (up to 100) |
| `bank(country, bankCode)` | `GET /v2/banks/{country}/{bankCode}` |
| `banks({ country, bic, name, limit })` | `GET /v2/banks` |
| `countries()` | `GET /v2/countries` |
| `data()` | `GET /v2/data`, what is loaded and how old |
| `health()` | `GET /healthz` |
| `logoUrl(country, bankCode, size?)` | URL only, nothing fetched |
| `validateV1(iban, { validateBankCode, getBIC })` | `GET /validate/{iban}`, openiban.com compatible |

Every non 2xx answer throws `IbanPizzaError` with `status` and the decoded
body. A structurally invalid IBAN is not an error: it returns a result with
`valid: false` and the failing check.

## Two things to read before integrating

**`ok` can be `null`.** A check that could not be performed is not a
failure. An account number whose check digit method is not implemented reports
`ok: null`, and `valid` stays true. Treat `null` as "no statement".

**Scheme membership is per institution, not per account.** `unknown` means the
BIC is absent from the EPC register, which is common for banks reachable
through a central institution. It does not mean "not supported".

## Options

```ts
new IbanPizza({
  baseUrl: "http://localhost:8080",
  fetch: customFetch,          // optional
  headers: { "x-api-key": "..." }, // optional, sent with every request
  timeoutMs: 5000,             // default 10000
});
```
