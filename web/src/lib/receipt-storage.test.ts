import { beforeEach, describe, expect, it } from "bun:test";

import {
  clearReceipts,
  listReceipts,
  removeReceipt,
  saveReceipt,
  subscribeReceipts,
} from "./receipt-storage";

import type { SignedReceipt } from "./verify-receipt";

const mockReceipt: SignedReceipt = {
  receipt: {
    id: "rcpt_123",
    version: "1.0",
    timestamp: "2026-06-19T00:00:00Z",
    payment: {
      payer: "0x1111111111111111111111111111111111111111",
      recipient: "0x2222222222222222222222222222222222222222",
      amount: "100",
      token: "USDC",
      chainId: 84532,
      nonce: "nonce-1",
    },
    service: {
      endpoint: "/api/test",
      request_hash: "req-hash",
      response_hash: "res-hash",
    },
  },
  signature:
    "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  server_public_key:
    "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
};

const storage = new Map<string, string>();

type TestGlobals = {
  localStorage?: {
    getItem: (key: string) => string | null;
    setItem: (key: string, value: string) => void;
    removeItem: (key: string) => void;
    clear: () => void;
  };
  window?: {
    localStorage?: unknown;
  };
};

const testGlobals = globalThis as unknown as TestGlobals;

beforeEach(() => {
  storage.clear();

  testGlobals.localStorage = {
  getItem: (key: string) => storage.get(key) ?? null,
  setItem: (key: string, value: string) => {
    storage.set(key, value);
  },
  removeItem: (key: string) => {
    storage.delete(key);
  },
  clear: () => {
    storage.clear();
  },
};

testGlobals.window = {
  localStorage: testGlobals.localStorage,
};
});

describe("receipt-storage", () => {
  it("ignores malformed JSON instead of crashing", () => {
    localStorage.setItem("microai:receipts", "{bad json");

    expect(() => listReceipts()).not.toThrow();
    expect(listReceipts()).toEqual([]);
  });

  it("ignores stale schema entries", () => {
    localStorage.setItem(
      "microai:receipts",
      JSON.stringify([
        {
          savedAt: Date.now(),
          receipt: {
            bad: "data",
          },
        },
      ])
    );

    expect(listReceipts()).toEqual([]);
  });

  it("deduplicates receipts by receipt id", () => {
  saveReceipt(mockReceipt, "first");

  saveReceipt(
    {
      ...mockReceipt,
      receipt: {
        ...mockReceipt.receipt,
        payment: {
          ...mockReceipt.receipt.payment,
          amount: "999",
        },
      },
    },
    "second"
  );

  const receipts = listReceipts();

  expect(receipts).toHaveLength(1);
  expect(receipts[0]?.promptPreview).toBe("second");
  expect(receipts[0]?.receipt.receipt.payment.amount).toBe("999");
});

it("trims promptPreview to 80 characters", () => {
  const longPrompt = "a".repeat(120);

  saveReceipt(mockReceipt, longPrompt);

  const receipts = listReceipts();

  expect(receipts[0]?.promptPreview.length).toBe(80);
  expect(receipts[0]?.promptPreview).toBe("a".repeat(80));
});

it("caps stored history at 20 entries with newest first", () => {
  for (let i = 0; i < 25; i++) {
    saveReceipt(
      {
        ...mockReceipt,
        receipt: {
          ...mockReceipt.receipt,
          id: `receipt-${i}`,
        },
      },
      `prompt-${i}`
    );
  }

  const receipts = listReceipts();

  expect(receipts).toHaveLength(20);
  expect(receipts[0]?.receipt.receipt.id).toBe("receipt-24");
  expect(receipts[19]?.receipt.receipt.id).toBe("receipt-5");
});

it("saveReceipt does not throw when localStorage fails", () => {
  testGlobals.localStorage!.setItem = () => {
    throw new Error("storage failed");
  };

  testGlobals.window!.localStorage =
    testGlobals.localStorage;

  expect(() => {
    saveReceipt(mockReceipt, "test");
  }).not.toThrow();
});

it("removeReceipt does not throw when localStorage fails", () => {
  testGlobals.localStorage!.setItem = () => {
    throw new Error("storage failed");
  };

  testGlobals.window!.localStorage =
    testGlobals.localStorage;

  expect(() => {
    removeReceipt("rcpt_123");
  }).not.toThrow();
});

it("clearReceipts does not throw when localStorage fails", () => {
  testGlobals.localStorage!.removeItem = () => {
    throw new Error("storage failed");
  };

  testGlobals.window!.localStorage =
    testGlobals.localStorage;

  expect(() => {
    clearReceipts();
  }).not.toThrow();
});

it("notifies subscribers after successful mutations", () => {
  let calls = 0;

  const unsubscribe = subscribeReceipts(() => {
    calls++;
  });

  saveReceipt(mockReceipt, "test");
  removeReceipt(mockReceipt.receipt.id);
  clearReceipts();

  unsubscribe();

  saveReceipt(mockReceipt, "test");

  expect(calls).toBe(3);
});

});