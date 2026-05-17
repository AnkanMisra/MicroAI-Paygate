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

function isValidEntry(entry: unknown): entry is StoredReceiptEntry {
  if (typeof entry !== "object" || entry === null) return false;
  const e = entry as { receipt?: { receipt?: { id?: unknown } }; savedAt?: unknown };
  return (
    typeof e.savedAt === "number" &&
    !!e.receipt &&
    !!e.receipt.receipt &&
    typeof e.receipt.receipt.id === "string"
  );
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
