import json
import os
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer

import pytest

from iban_pizza import IbanPizza, IbanPizzaError, compact


class Recorder(BaseHTTPRequestHandler):
    """A tiny server that records the request and answers with a canned body."""

    status = 200
    body = {}
    calls = []

    def _answer(self):
        length = int(self.headers.get("content-length") or 0)
        raw = self.rfile.read(length) if length else b""
        # HTTPMessage lookups are case insensitive; a plain dict of it is not.
        Recorder.calls.append({"method": self.command, "path": self.path, "body": raw.decode(), "content_type": self.headers.get("content-type")})
        payload = json.dumps(Recorder.body).encode()
        self.send_response(Recorder.status)
        self.send_header("content-type", "application/json")
        self.send_header("content-length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    do_GET = _answer
    do_POST = _answer

    def log_message(self, *a):
        pass


@pytest.fixture
def server():
    srv = HTTPServer(("127.0.0.1", 0), Recorder)
    t = threading.Thread(target=srv.serve_forever, daemon=True)
    t.start()
    Recorder.calls = []
    Recorder.status = 200
    Recorder.body = {}
    yield f"http://127.0.0.1:{srv.server_port}"
    srv.shutdown()


def test_compact():
    assert compact(" DE89-3704 0044 ") == "DE8937040044"


def test_validate_compacts_iban_into_path(server):
    Recorder.body = {"valid": True, "iban": {"input": "x"}, "checks": {}}
    r = IbanPizza(server + "///").validate("DE89 3704 0044 0532 0130 00")
    assert r["valid"] is True
    assert Recorder.calls[0]["path"] == "/v2/iban/DE89370400440532013000"
    assert Recorder.calls[0]["method"] == "GET"


def test_batch_sends_json_and_unwraps(server):
    Recorder.body = {"results": [{"valid": True}, {"valid": False}]}
    r = IbanPizza(server).validate_many(["DE89 3704 0044 0532 0130 00", "nope"])
    assert [x["valid"] for x in r] == [True, False]
    call = Recorder.calls[0]
    assert call["path"] == "/v2/iban:batch"
    assert json.loads(call["body"]) == {"ibans": ["DE89370400440532013000", "nope"]}
    assert call["content_type"] == "application/json"


def test_banks_query_drops_empty_parameters(server):
    Recorder.body = {"banks": [], "count": 0}
    IbanPizza(server).banks(country="DE", bic="COBA", name="", limit=5)
    assert Recorder.calls[0]["path"] == "/v2/banks?country=DE&bic=COBA&limit=5"


def test_error_carries_status_and_message(server):
    Recorder.status = 404
    Recorder.body = {"error": "no such bank"}
    with pytest.raises(IbanPizzaError) as e:
        IbanPizza(server).bank("DE", "00000000")
    assert e.value.status == 404
    assert str(e.value) == "no such bank"
    assert e.value.body == {"error": "no such bank"}


def test_logo_url_fetches_nothing(server):
    api = IbanPizza(server)
    assert api.logo_url("DE", "50010517") == f"{server}/v2/banks/DE/50010517/logo.svg"
    assert api.logo_url("DE", "50010517", 64).endswith("/logo.svg?size=64")
    assert Recorder.calls == []


def test_v1_flags(server):
    Recorder.body = {"valid": True, "messages": [], "iban": "x", "bankData": {}, "checkResults": {}}
    IbanPizza(server).validate_v1("DE89370400440532013000", validate_bank_code=True, get_bic=True)
    assert Recorder.calls[0]["path"] == "/validate/DE89370400440532013000?validateBankCode=true&getBIC=true"


# Integration: only when a real instance is reachable. CI starts the service
# binary and sets IBAN_PIZZA_URL; locally these are skipped.
LIVE = os.environ.get("IBAN_PIZZA_URL")
live = pytest.mark.skipif(not LIVE, reason="IBAN_PIZZA_URL not set")


@live
def test_live_validate_resolves_bank():
    r = IbanPizza(LIVE).validate("DE89 3704 0044 0532 0130 00")
    assert r["valid"] is True
    assert r["bank"]["name"] == "Commerzbank"
    assert r["checks"]["ibanChecksum"]["ok"] is True


@live
def test_live_account_check_digit():
    r = IbanPizza(LIVE).validate("DE49500105179144355668")
    assert r["valid"] is False
    assert r["checks"]["accountNumber"]["ok"] is False
    assert r["checks"]["accountNumber"]["method"] == "C1"


@live
def test_live_batch_search_registry_data():
    api = IbanPizza(LIVE)
    # The Austrian sample's bank code is not in the OeNB register, so the
    # service reports it as not valid; both results must still come back,
    # in order, each parsed.
    many = api.validate_many(["DE89370400440532013000", "AT611904300234573201"])
    assert len(many) == 2 and many[0]["valid"] is True
    assert many[1]["iban"]["countryCode"] == "AT" and many[1]["checks"]["ibanChecksum"]["ok"] is True
    assert api.banks(bic="COBA", limit=3)
    assert next(c for c in api.countries() if c["code"] == "DE")["hasBankData"] is True
    assert api.data()["records"] > 1000
    assert api.health()["status"] in ("ok", "stale")
    assert api.validate_v1("DE89370400440532013000", get_bic=True)["bankData"]["bic"] == "COBADEFFXXX"
    with pytest.raises(IbanPizzaError) as e:
        api.bank("DE", "00000000")
    assert e.value.status == 404
