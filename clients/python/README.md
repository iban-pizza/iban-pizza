# iban-pizza

Client for the [iban.pizza](https://iban.pizza) API: IBAN validation, bank
lookup and SEPA scheme membership. Works against the public instance or your
own. No dependencies; Python 3.9 or newer.

```sh
pip install iban-pizza
```

```python
from iban_pizza import IbanPizza

api = IbanPizza("https://iban.pizza")

r = api.validate("DE89 3704 0044 0532 0130 00")
r["valid"]                          # True
r["bank"]["name"]                   # "Commerzbank"
r["checks"]["accountNumber"]["ok"]  # True, False, or None when no method applies
r["schemes"]["schemes"]["sctInst"]  # {"status": "participant", ...}
```

## What is there

| Method | Endpoint |
|---|---|
| `validate(iban)` | `GET /v2/iban/{iban}` |
| `validate_many(ibans)` | `POST /v2/iban:batch` (up to 100) |
| `bank(country, bank_code)` | `GET /v2/banks/{country}/{bankCode}` |
| `banks(country=, bic=, name=, limit=)` | `GET /v2/banks` |
| `countries()` | `GET /v2/countries` |
| `data()` | `GET /v2/data`, what is loaded and how old |
| `health()` | `GET /healthz` |
| `logo_url(country, bank_code, size=None)` | URL only, nothing fetched |
| `validate_v1(iban, validate_bank_code=, get_bic=)` | `GET /validate/{iban}`, openiban.com compatible |

Every non 2xx answer raises `IbanPizzaError` with `.status` and the decoded
`.body`. A structurally invalid IBAN is not an error: it returns a result with
`valid` False and the failing check.

## Two things to read before integrating

**`ok` can be `None`.** A check that could not be performed is not a
failure. An account number whose check digit method is not implemented reports
`ok: None`, and `valid` stays True. Treat `None` as "no statement".

**Scheme membership is per institution, not per account.** `unknown` means the
BIC is absent from the EPC register, which is common for banks reachable
through a central institution. It does not mean "not supported".

## Options

```python
IbanPizza("http://localhost:8080", headers={"x-api-key": "..."}, timeout=5.0)
```
