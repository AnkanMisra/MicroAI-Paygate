import { afterEach, describe, expect, it } from "bun:test";
import { decodeBase64ToUtf8, encodeUtf8ToBase64 } from "../base64";

type Base64Global = "atob" | "btoa";

const originalAtobDescriptor = Object.getOwnPropertyDescriptor(globalThis, "atob");
const originalBtoaDescriptor = Object.getOwnPropertyDescriptor(globalThis, "btoa");

function setGlobalCapability(name: Base64Global, value: typeof globalThis.atob | undefined) {
  Object.defineProperty(globalThis, name, {
    configurable: true,
    value,
    writable: true,
  });
}

function restoreGlobalCapability(name: Base64Global, descriptor: PropertyDescriptor | undefined) {
  if (descriptor) {
    Object.defineProperty(globalThis, name, descriptor);
    return;
  }

  Reflect.deleteProperty(globalThis, name);
}

afterEach(() => {
  restoreGlobalCapability("atob", originalAtobDescriptor);
  restoreGlobalCapability("btoa", originalBtoaDescriptor);
});

describe("base64 UTF-8 helpers", () => {
  it("encodes known ASCII fixtures without Node Buffer", () => {
    expect(encodeUtf8ToBase64("hello")).toBe("aGVsbG8=");
    expect(encodeUtf8ToBase64("MicroAI Paygate")).toBe("TWljcm9BSSBQYXlnYXRl");
  });

  it("round-trips ASCII, accented, non-Latin, and emoji text", () => {
    const cases = [
      "plain ASCII receipt",
      "Cr\u00e8me br\u00fbl\u00e9e",
      "\u0928\u092e\u0938\u094d\u0924\u0947 \u092d\u0941\u0917\u0924\u093e\u0928",
      "paid receipt \u{1f680}\u2705",
    ];

    for (const value of cases) {
      expect(decodeBase64ToUtf8(encodeUtf8ToBase64(value))).toBe(value);
    }
  });

  it("decodes valid UTF-8 base64 fixtures", () => {
    expect(decodeBase64ToUtf8("SGVsbG8sIHdvcmxkIQ==")).toBe("Hello, world!");
    expect(decodeBase64ToUtf8("4pyF")).toBe("\u2705");
  });

  it("encodes empty text to an empty base64 payload", () => {
    expect(encodeUtf8ToBase64("")).toBe("");
  });

  it("rejects empty, whitespace-padded, invalid-character, and invalid-padding input", () => {
    for (const value of ["", " aGVsbG8=", "aGVsbG8= ", "abc@", "ab=", "abc==", "a==="]) {
      expect(() => decodeBase64ToUtf8(value)).toThrow("invalid base64");
    }
  });

  it("reports unavailable decoding capability and restores globals", () => {
    setGlobalCapability("atob", undefined);

    expect(() => decodeBase64ToUtf8("aGVsbG8=")).toThrow(
      "base64 decoding is unavailable in this runtime",
    );
  });

  it("reports unavailable encoding capability and restores globals", () => {
    setGlobalCapability("btoa", undefined);

    expect(() => encodeUtf8ToBase64("hello")).toThrow(
      "base64 encoding is unavailable in this runtime",
    );
  });
});
