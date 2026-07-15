import { describe, expect, it } from "bun:test";
import { decodeBase64ToUtf8, encodeUtf8ToBase64 } from "../base64";

const originalAtob = Object.getOwnPropertyDescriptor(globalThis, "atob");
const originalBtoa = Object.getOwnPropertyDescriptor(globalThis, "btoa");

function withoutGlobal(name: "atob" | "btoa", callback: () => void): void {
  const original = Object.getOwnPropertyDescriptor(globalThis, name);
  try {
    Object.defineProperty(globalThis, name, {
      configurable: true,
      writable: true,
      value: undefined,
    });
    callback();
  } finally {
    if (original) {
      Object.defineProperty(globalThis, name, original);
    } else {
      delete (globalThis as Record<string, unknown>)[name];
    }
  }
}

describe("UTF-8 base64 helpers", () => {
  it.each([
    ["ASCII", "Hello, world!"],
    ["accented text", "Café déjà vu"],
    ["non-Latin text", "नमस्ते мир"],
    ["emoji", "Paygate 🚀✅"],
  ])("round-trips %s", (_label, value) => {
    expect(decodeBase64ToUtf8(encodeUtf8ToBase64(value))).toBe(value);
  });

  it("encodes empty text as an empty string", () => {
    expect(encodeUtf8ToBase64("")).toBe("");
  });

  it("matches a known base64 fixture", () => {
    expect(encodeUtf8ToBase64("Hello, world!")).toBe("SGVsbG8sIHdvcmxkIQ==");
    expect(decodeBase64ToUtf8("SGVsbG8sIHdvcmxkIQ==")).toBe("Hello, world!");
  });

  it.each(["", " SGVsbG8=", "SGVsbG8= ", "SGVsbG8", "SGVsbG8$", "===="]) (
    "rejects invalid base64 input %j",
    (value) => {
      expect(() => decodeBase64ToUtf8(value)).toThrow("invalid base64");
    },
  );

  it("reports a missing atob capability and restores it after the assertion", () => {
    withoutGlobal("atob", () => {
      expect(() => decodeBase64ToUtf8("SGVsbG8=")).toThrow(
        "base64 decoding is unavailable in this runtime",
      );
    });

    expect(Object.getOwnPropertyDescriptor(globalThis, "atob")).toEqual(originalAtob);
  });

  it("reports a missing btoa capability and restores it after the assertion", () => {
    withoutGlobal("btoa", () => {
      expect(() => encodeUtf8ToBase64("hello")).toThrow(
        "base64 encoding is unavailable in this runtime",
      );
    });

    expect(Object.getOwnPropertyDescriptor(globalThis, "btoa")).toEqual(originalBtoa);
  });
});
