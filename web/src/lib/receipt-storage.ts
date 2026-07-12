import type { SignedReceipt } from "./verify-receipt";
import type { StoredReceiptEntry } from "./types";

const KEY = "microai:receipts";
const MAX = 20;
const EMPTY: StoredReceiptEntry[] = [];

function isBrowser(): boolean {
  return typeof window !== "undefined" && typeof window.localStorage !== "undefined";
}

function readRaw(): string | null {
  if (!isBrowser()) return null;
  try {
    return window.localStorage.getItem(KEY);
  } catch {
    return null;
  }
}

// Full SignedReceipt shape check — guards every field the UI later reads
// against malformed / stale-schema entries from older builds. A single missing
// field (e.g. payment.payer in a v0 receipt) would otherwise crash
// receipt-card on shortenAddress(undefined).
function isValidEntry(entry: unknown): entry is StoredReceiptEntry {
  if (typeof entry !== "object" || entry === null) return false;
  const e = entry as { receipt?: unknown; savedAt?: unknown };
  if (typeof e.savedAt !== "number" || !Number.isFinite(e.savedAt)) return false;

  if (typeof e.receipt !== "object" || e.receipt === null) return false;
  const sr = e.receipt as {
    receipt?: unknown;
    signature?: unknown;
    server_public_key?: unknown;
  };
  if (typeof sr.signature !== "string" || !sr.signature.startsWith("0x")) return false;
  if (typeof sr.server_public_key !== "string" || !sr.server_public_key.startsWith("0x")) {
    return false;
  }

  if (typeof sr.receipt !== "object" || sr.receipt === null) return false;
  const r = sr.receipt as {
    id?: unknown;
    payment?: unknown;
    service?: unknown;
  };
  if (typeof r.id !== "string") return false;

  if (typeof r.payment !== "object" || r.payment === null) return false;
  const p = r.payment as {
    payer?: unknown;
    recipient?: unknown;
    amount?: unknown;
    token?: unknown;
  };
  if (typeof p.payer !== "string" || typeof p.recipient !== "string") return false;
  if (typeof p.amount !== "string" || typeof p.token !== "string") return false;
  // chainId is a number on every SignedReceipt the gateway produces; without
  // this guard a stale entry would render "Chain undefined" in the ReceiptCard
  // badge via getChainMeta's fallback path.
  if (typeof (p as { chainId?: unknown }).chainId !== "number") return false;

  if (typeof r.service !== "object" || r.service === null) return false;
  const s = r.service as {
    endpoint?: unknown;
    request_hash?: unknown;
    response_hash?: unknown;
  };
  if (typeof s.endpoint !== "string") return false;
  if (typeof s.request_hash !== "string" || typeof s.response_hash !== "string") {
    return false;
  }

  return true;
}

function parse(raw: string | null): StoredReceiptEntry[] {
  if (!raw) return EMPTY;
  try {
    const parsed = JSON.parse(raw) as unknown;
    // Drop any stale entries from previous schema versions — without this,
    // ReceiptHistory dereferences entry.receipt.receipt.id as a React key and
    // would crash the page on undefined, locking users out until they clear
    // localStorage manually.
    return Array.isArray(parsed) ? parsed.filter(isValidEntry) : EMPTY;
  } catch {
    return EMPTY;
  }
}

// Cached snapshot keyed by the raw localStorage string so React.useSyncExternalStore
// can rely on referential stability between unchanged reads.
let cachedRaw: string | null = null;
let cachedSnapshot: StoredReceiptEntry[] = EMPTY;

function refreshSnapshot(): void {
  const raw = readRaw();
  if (raw !== cachedRaw) {
    cachedRaw = raw;
    cachedSnapshot = parse(raw);
  }
}

export function getReceiptsSnapshot(): StoredReceiptEntry[] {
  refreshSnapshot();
  return cachedSnapshot;
}

export function getReceiptsServerSnapshot(): StoredReceiptEntry[] {
  return EMPTY;
}

type Listener = () => void;
const listeners = new Set<Listener>();

function notify(): void {
  for (const l of listeners) l();
}

export function subscribeReceipts(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function listReceipts(): StoredReceiptEntry[] {
  return parse(readRaw());
}

// All writes are best-effort. A localStorage throw (quota exceeded, private
// browsing, Safari ITP) MUST NOT propagate — useX402.submit calls saveReceipt
// on the success path, and a bubble would discard a paid summary the user
// already signed for.
export function saveReceipt(receipt: SignedReceipt, promptPreview: string): void {
  if (!isBrowser()) return;
  const entries = listReceipts();
  const filtered = entries.filter((e) => e.receipt.receipt.id !== receipt.receipt.id);
  const next: StoredReceiptEntry = {
    receipt,
    savedAt: Date.now(),
    promptPreview: promptPreview.slice(0, 80),
  };
  const trimmed = [next, ...filtered].slice(0, MAX);
  try {
    window.localStorage.setItem(KEY, JSON.stringify(trimmed));
    notify();
  } catch (err) {
    console.warn("receipt-storage: failed to save receipt", err);
  }
}

export function removeReceipt(id: string): void {
  if (!isBrowser()) return;
  const entries = listReceipts().filter((e) => e.receipt.receipt.id !== id);
  try {
    window.localStorage.setItem(KEY, JSON.stringify(entries));
    notify();
  } catch (err) {
    console.warn("receipt-storage: failed to remove receipt", err);
  }
}

export function clearReceipts(): void {
  if (!isBrowser()) return;
  try {
    window.localStorage.removeItem(KEY);
    notify();
  } catch (err) {
    console.warn("receipt-storage: failed to clear receipts", err);
  }
}
