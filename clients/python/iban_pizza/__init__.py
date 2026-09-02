"""Client for the iban.pizza API.

Thin by design: the standard library's ``urllib`` is the transport, there are
no dependencies, and the answer shapes are typed as ``TypedDict`` so editors
can complete them. The shapes mirror the service's ``openapi.yaml``.
"""

from __future__ import annotations

import json
import re
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Dict, List, Optional, Sequence, TypedDict

__all__ = ["IbanPizza", "IbanPizzaError", "compact", "ValidationResult", "Bank", "Check"]
__version__ = "0.1.0"


class Check(TypedDict, total=False):
    ok: Optional[bool]
    message: str
    method: str


class Logo(TypedDict, total=False):
    kind: str
    brand: str
    url: str


class Bank(TypedDict, total=False):
    country: str
    bankCode: str
    name: str
    shortName: str
    zip: str
    city: str
    bic: str
    source: str
    logo: Logo


class Membership(TypedDict, total=False):
    status: str
    readinessDate: str
    leavingDate: str
    role: str


class Schemes(TypedDict, total=False):
    level: str
    schemes: Dict[str, Membership]
    source: str
    asOf: str
    note: str


class IBANParts(TypedDict, total=False):
    input: str
    formatted: str
    countryCode: str
    checkDigits: str
    bankCode: str
    branchCode: str
    accountNumber: str


class ValidationResult(TypedDict, total=False):
    valid: bool
    iban: IBANParts
    checks: Dict[str, Check]
    bank: Bank
    schemes: Schemes
    dataAsOf: Dict[str, str]


_SEPARATORS = re.compile(r"[\s-]+")


def compact(iban: str) -> str:
    """Remove spaces and hyphens, the separators people type into IBANs."""
    return _SEPARATORS.sub("", iban)


class IbanPizzaError(Exception):
    """Raised for any non 2xx answer. ``status`` is the HTTP status, ``body`` the decoded error when the service sent one."""

    def __init__(self, status: int, message: str, body: Any = None) -> None:
        super().__init__(message)
        self.status = status
        self.body = body


class IbanPizza:
    """A client bound to one iban.pizza instance.

    >>> api = IbanPizza("https://iban.pizza")
    >>> r = api.validate("DE89 3704 0044 0532 0130 00")
    >>> r["valid"], r["bank"]["name"]
    (True, 'Commerzbank')
    """

    def __init__(self, base_url: str, *, headers: Optional[Dict[str, str]] = None, timeout: float = 10.0) -> None:
        self.base_url = base_url.rstrip("/")
        self.headers = {"accept": "application/json", **(headers or {})}
        self.timeout = timeout

    # -- v2 -------------------------------------------------------------

    def validate(self, iban: str) -> ValidationResult:
        """Validate an IBAN and describe its bank. Invalid input still returns a result, with ``valid`` False."""
        return self._get(f"/v2/iban/{urllib.parse.quote(compact(iban), safe='')}")

    def validate_many(self, ibans: Sequence[str]) -> List[ValidationResult]:
        """Validate up to 100 IBANs in one request, in the order given."""
        body = self._post("/v2/iban:batch", {"ibans": [compact(i) for i in ibans]})
        return body["results"]

    def bank(self, country: str, bank_code: str) -> Bank:
        """One bank by country and national bank code."""
        return self._get(f"/v2/banks/{urllib.parse.quote(country, safe='')}/{urllib.parse.quote(bank_code, safe='')}")

    def banks(self, *, country: Optional[str] = None, bic: Optional[str] = None, name: Optional[str] = None, limit: Optional[int] = None) -> List[Bank]:
        """Search banks by country, BIC prefix or name."""
        params = {k: v for k, v in {"country": country, "bic": bic, "name": name, "limit": limit}.items() if v not in (None, "")}
        query = ("?" + urllib.parse.urlencode(params)) if params else ""
        return self._get("/v2/banks" + query)["banks"]

    def countries(self) -> List[Dict[str, Any]]:
        """The IBAN registry: every country, its length and structure, and whether bank data is loaded."""
        return self._get("/v2/countries")["countries"]

    def data(self) -> Dict[str, Any]:
        """What the instance is answering from: loaded registries, retrieval dates, ages, record counts."""
        return self._get("/v2/data")

    def health(self) -> Dict[str, Any]:
        """Liveness and data freshness."""
        return self._get("/healthz")

    def logo_url(self, country: str, bank_code: str, size: Optional[int] = None) -> str:
        """URL of the bank's monogram, for an ``<img>`` tag. Nothing is fetched."""
        url = f"{self.base_url}/v2/banks/{urllib.parse.quote(country, safe='')}/{urllib.parse.quote(bank_code, safe='')}/logo.svg"
        return f"{url}?size={size}" if size else url

    # -- v1, openiban.com compatible --------------------------------------

    def validate_v1(self, iban: str, *, validate_bank_code: bool = False, get_bic: bool = False) -> Dict[str, Any]:
        """The openiban.com compatible answer, for code written against that service."""
        params = {}
        if validate_bank_code:
            params["validateBankCode"] = "true"
        if get_bic:
            params["getBIC"] = "true"
        query = ("?" + urllib.parse.urlencode(params)) if params else ""
        return self._get(f"/validate/{urllib.parse.quote(compact(iban), safe='')}{query}")

    # -- transport --------------------------------------------------------

    def _get(self, path: str) -> Any:
        return self._request("GET", path)

    def _post(self, path: str, body: Any) -> Any:
        return self._request("POST", path, body)

    def _request(self, method: str, path: str, body: Any = None) -> Any:
        data = None
        headers = dict(self.headers)
        if body is not None:
            data = json.dumps(body).encode("utf-8")
            headers["content-type"] = "application/json"
        req = urllib.request.Request(self.base_url + path, data=data, headers=headers, method=method)
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as res:
                return _decode(res.read())
        except urllib.error.HTTPError as e:
            decoded = _decode(e.read())
            message = decoded["error"] if isinstance(decoded, dict) and isinstance(decoded.get("error"), str) else f"{method} {path} returned {e.code}"
            raise IbanPizzaError(e.code, message, decoded) from None


def _decode(raw: bytes) -> Any:
    if not raw:
        return None
    try:
        return json.loads(raw)
    except ValueError:
        return raw.decode("utf-8", "replace")
