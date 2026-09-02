// Client for the iban.pizza API.
//
// Thin by design: the response types come from the service's openapi.yaml
// (generated into openapi.d.ts at build time, so they cannot drift), the
// transport is the platform's fetch, and there is no runtime dependency.

import type { components, paths } from "./openapi.js";

type Schemas = components["schemas"];
export type ValidationResult = Schemas["ValidationResult"];
export type Check = Schemas["Check"];
export type Bank = Schemas["Bank"];
export type Membership = Schemas["Membership"];
export type Schemes = Schemas["Schemes"];
export type Country = Schemas["Country"];
export type Health = Schemas["Health"];
export type DataReport = Schemas["DataReport"];
export type V1Result = Schemas["V1Result"];

export type BankQuery = NonNullable<paths["/v2/banks"]["get"]["parameters"]["query"]>;

/** Options for the client. */
export interface Options {
  /** Base URL of an iban.pizza instance, for example "https://iban.pizza" or "http://localhost:8080". */
  baseUrl: string;
  /** A fetch implementation; defaults to the global one. */
  fetch?: typeof fetch;
  /** Extra headers sent with every request. */
  headers?: Record<string, string>;
  /** Request timeout in milliseconds. Defaults to 10000. */
  timeoutMs?: number;
}

/** Thrown for any non 2xx answer. `status` is the HTTP status, `body` the decoded error when the service sent one. */
export class IbanPizzaError extends Error {
  readonly status: number;
  readonly body: unknown;
  constructor(status: number, message: string, body: unknown) {
    super(message);
    this.name = "IbanPizzaError";
    this.status = status;
    this.body = body;
  }
}

export class IbanPizza {
  readonly baseUrl: string;
  private readonly fetchFn: typeof fetch;
  private readonly headers: Record<string, string>;
  private readonly timeoutMs: number;

  constructor(options: Options) {
    this.baseUrl = options.baseUrl.replace(/\/+$/, "");
    this.fetchFn = options.fetch ?? globalThis.fetch;
    this.headers = { accept: "application/json", ...options.headers };
    this.timeoutMs = options.timeoutMs ?? 10_000;
    if (typeof this.fetchFn !== "function") {
      throw new Error("iban-pizza: no fetch implementation available; pass one in options.fetch");
    }
  }

  /** Validate an IBAN and describe its bank. Structurally invalid input still returns a result, with `valid: false`. */
  validate(iban: string): Promise<ValidationResult> {
    return this.get(`/v2/iban/${encodeURIComponent(compact(iban))}`);
  }

  /** Validate up to 100 IBANs in one request. Results come back in the order given. */
  async validateMany(ibans: string[]): Promise<ValidationResult[]> {
    const body = await this.post<{ results: ValidationResult[] }>("/v2/iban:batch", { ibans: ibans.map(compact) });
    return body.results;
  }

  /** One bank by country and national bank code. */
  bank(country: string, bankCode: string): Promise<Bank> {
    return this.get(`/v2/banks/${encodeURIComponent(country)}/${encodeURIComponent(bankCode)}`);
  }

  /** Search banks by country, BIC prefix or name. */
  async banks(query: BankQuery = {}): Promise<Bank[]> {
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(query)) if (v !== undefined && v !== null && v !== "") q.set(k, String(v));
    const s = q.toString();
    const body = await this.get<{ banks: Bank[] }>(`/v2/banks${s ? `?${s}` : ""}`);
    return body.banks;
  }

  /** The IBAN registry: every country, its length and structure, and whether bank data is loaded. */
  async countries(): Promise<Country[]> {
    const body = await this.get<{ countries: Country[] }>("/v2/countries");
    return body.countries;
  }

  /** What the instance is answering from: loaded registries, retrieval dates, ages, record counts. */
  data(): Promise<DataReport> {
    return this.get("/v2/data");
  }

  /** Liveness and data freshness. */
  health(): Promise<Health> {
    return this.get("/healthz");
  }

  /** URL of the bank's monogram, for an <img> tag. Nothing is fetched. */
  logoUrl(country: string, bankCode: string, size?: number): string {
    const u = `${this.baseUrl}/v2/banks/${encodeURIComponent(country)}/${encodeURIComponent(bankCode)}/logo.svg`;
    return size ? `${u}?size=${size}` : u;
  }

  /** The openiban.com compatible v1 answer, for code written against that service. */
  validateV1(iban: string, options: { validateBankCode?: boolean; getBIC?: boolean } = {}): Promise<V1Result> {
    const q = new URLSearchParams();
    if (options.validateBankCode) q.set("validateBankCode", "true");
    if (options.getBIC) q.set("getBIC", "true");
    const s = q.toString();
    return this.get(`/validate/${encodeURIComponent(compact(iban))}${s ? `?${s}` : ""}`);
  }

  private get<T>(path: string): Promise<T> {
    return this.request<T>("GET", path);
  }

  private post<T>(path: string, body: unknown): Promise<T> {
    return this.request<T>("POST", path, body);
  }

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeoutMs);
    try {
      const res = await this.fetchFn(this.baseUrl + path, {
        method,
        headers: body === undefined ? this.headers : { ...this.headers, "content-type": "application/json" },
        body: body === undefined ? undefined : JSON.stringify(body),
        signal: controller.signal,
      });
      const text = await res.text();
      const decoded = text ? safeJson(text) : undefined;
      if (!res.ok) {
        const message = isErrorBody(decoded) ? decoded.error : `${method} ${path} returned ${res.status}`;
        throw new IbanPizzaError(res.status, message, decoded);
      }
      return decoded as T;
    } finally {
      clearTimeout(timer);
    }
  }
}

/** Removes spaces and hyphens, the separators people type into IBANs. */
export function compact(iban: string): string {
  return iban.replace(/[\s-]+/g, "");
}

function safeJson(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

function isErrorBody(v: unknown): v is { error: string } {
  return typeof v === "object" && v !== null && typeof (v as { error?: unknown }).error === "string";
}

export default IbanPizza;
