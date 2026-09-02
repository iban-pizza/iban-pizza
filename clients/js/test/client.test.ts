import { beforeAll, describe, expect, it, vi } from "vitest";
import { IbanPizza, IbanPizzaError, compact } from "../src/index.js";

// A fetch that records the request and answers with a canned body.
function fakeFetch(status: number, body: unknown) {
  const calls: { url: string; init: RequestInit }[] = [];
  const fn = vi.fn(async (url: string | URL | Request, init?: RequestInit) => {
    calls.push({ url: String(url), init: init ?? {} });
    return new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
  }) as unknown as typeof fetch;
  return { fn, calls };
}

describe("IbanPizza", () => {
  it("normalises the base URL and compacts the IBAN into the path", async () => {
    const { fn, calls } = fakeFetch(200, { valid: true, iban: { input: "x" }, checks: {} });
    const c = new IbanPizza({ baseUrl: "https://iban.pizza///", fetch: fn });
    await c.validate("DE89 3704 0044 0532 0130 00");
    expect(calls[0].url).toBe("https://iban.pizza/v2/iban/DE89370400440532013000");
    expect(calls[0].init.method).toBe("GET");
  });

  it("sends a batch as JSON and unwraps the results", async () => {
    const { fn, calls } = fakeFetch(200, { results: [{ valid: true }, { valid: false }] });
    const c = new IbanPizza({ baseUrl: "http://localhost:8080", fetch: fn });
    const r = await c.validateMany(["DE89 3704 0044 0532 0130 00", "nope"]);
    expect(r).toHaveLength(2);
    expect(calls[0].url).toBe("http://localhost:8080/v2/iban:batch");
    expect(JSON.parse(String(calls[0].init.body))).toEqual({ ibans: ["DE89370400440532013000", "nope"] });
    expect((calls[0].init.headers as Record<string, string>)["content-type"]).toBe("application/json");
  });

  it("builds the bank search query and drops empty parameters", async () => {
    const { fn, calls } = fakeFetch(200, { banks: [], count: 0 });
    const c = new IbanPizza({ baseUrl: "http://localhost:8080", fetch: fn });
    await c.banks({ country: "DE", bic: "COBA", name: "", limit: 5 });
    expect(calls[0].url).toBe("http://localhost:8080/v2/banks?country=DE&bic=COBA&limit=5");
  });

  it("throws IbanPizzaError with the service's message on a non 2xx answer", async () => {
    const { fn } = fakeFetch(404, { error: "no such bank" });
    const c = new IbanPizza({ baseUrl: "http://localhost:8080", fetch: fn });
    await expect(c.bank("DE", "00000000")).rejects.toMatchObject({ name: "IbanPizzaError", status: 404, message: "no such bank" });
    await expect(c.bank("DE", "00000000")).rejects.toBeInstanceOf(IbanPizzaError);
  });

  it("builds logo URLs without fetching", () => {
    const { fn, calls } = fakeFetch(200, {});
    const c = new IbanPizza({ baseUrl: "https://iban.pizza", fetch: fn });
    expect(c.logoUrl("DE", "50010517")).toBe("https://iban.pizza/v2/banks/DE/50010517/logo.svg");
    expect(c.logoUrl("DE", "50010517", 64)).toBe("https://iban.pizza/v2/banks/DE/50010517/logo.svg?size=64");
    expect(calls).toHaveLength(0);
  });

  it("passes v1 flags through as the upstream expects them", async () => {
    const { fn, calls } = fakeFetch(200, { valid: true, messages: [], iban: "x", bankData: {}, checkResults: {} });
    const c = new IbanPizza({ baseUrl: "http://localhost:8080", fetch: fn });
    await c.validateV1("DE89370400440532013000", { getBIC: true, validateBankCode: true });
    expect(calls[0].url).toBe("http://localhost:8080/validate/DE89370400440532013000?validateBankCode=true&getBIC=true");
  });

  it("aborts after the timeout", async () => {
    const fn = (async (_url: unknown, init?: RequestInit) =>
      new Promise<Response>((_, reject) => init?.signal?.addEventListener("abort", () => reject(new Error("aborted"))))) as unknown as typeof fetch;
    const c = new IbanPizza({ baseUrl: "http://localhost:8080", fetch: fn, timeoutMs: 20 });
    await expect(c.health()).rejects.toThrow("aborted");
  });
});

describe("compact", () => {
  it("removes spaces and hyphens only", () => {
    expect(compact(" DE89-3704 0044 ")).toBe("DE8937040044");
  });
});

// Integration: only when a real instance is reachable. CI starts the service
// binary and sets IBAN_PIZZA_URL; locally the block is skipped.
const live = process.env.IBAN_PIZZA_URL;
describe.skipIf(!live)("against a running instance", () => {
  // Built in beforeAll, not at collection time: a skipped describe still
  // runs its body to collect tests, and the constructor throws without a URL.
  let c: IbanPizza;
  beforeAll(() => {
    c = new IbanPizza({ baseUrl: live! });
  });

  it("validates a known IBAN and resolves its bank", async () => {
    const r = await c.validate("DE89 3704 0044 0532 0130 00");
    expect(r.valid).toBe(true);
    expect(r.bank?.name).toBe("Commerzbank");
    expect(r.checks.ibanChecksum.ok).toBe(true);
  });

  it("reports the ING account check digit failure", async () => {
    const r = await c.validate("DE49500105179144355668");
    expect(r.valid).toBe(false);
    expect(r.checks.accountNumber.ok).toBe(false);
    expect(r.checks.accountNumber.method).toBe("C1");
  });

  it("batches, searches, reads the registry and the data report", async () => {
    // The Austrian sample has a bank code that is not in the OeNB register,
    // so the service reports it as not valid; the point here is that both
    // results come back in order, each parsed.
    const many = await c.validateMany(["DE89370400440532013000", "AT611904300234573201"]);
    expect(many).toHaveLength(2);
    expect(many[0].valid).toBe(true);
    expect(many[1].iban.countryCode).toBe("AT");
    expect(many[1].checks.ibanChecksum.ok).toBe(true);
    const banks = await c.banks({ bic: "COBA", limit: 3 });
    expect(banks.length).toBeGreaterThan(0);
    const countries = await c.countries();
    expect(countries.find((x) => x.code === "DE")?.hasBankData).toBe(true);
    const data = await c.data();
    expect(data.records).toBeGreaterThan(1000);
    expect((await c.health()).status).toMatch(/ok|stale/);
  });

  it("keeps the v1 surface", async () => {
    const r = await c.validateV1("DE89370400440532013000", { getBIC: true });
    expect(r.bankData?.bic).toBe("COBADEFFXXX");
  });

  it("raises on an unknown bank", async () => {
    await expect(c.bank("DE", "00000000")).rejects.toMatchObject({ status: 404 });
  });
});
