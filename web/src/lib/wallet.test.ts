import { describe, expect, test } from "bun:test";
import { shortenAddress, getChainMeta } from "./wallet";

describe("shortenAddress", () => {
  test("returns empty string for empty input", () => {
    expect(shortenAddress("")).toBe("");
  });

  test("returns short address unchanged", () => {
    expect(shortenAddress("0x123456")).toBe("0x123456");
  });

  test("shortens a normal address", () => {
    expect(
      shortenAddress("0x1234567890abcdef1234567890abcdef12345678")
    ).toBe("0x1234…5678");
  });

  test("supports custom chars argument", () => {
    expect(
      shortenAddress("0x1234567890abcdef1234567890abcdef12345678", 6)
    ).toBe("0x123456…345678");
  });
});

describe("getChainMeta", () => {
  test("returns Base Sepolia metadata", () => {
    expect(getChainMeta(84532)).toEqual({
      id: 84532,
      name: "Base Sepolia",
      rpcUrl: "https://sepolia.base.org",
      explorer: "https://sepolia.basescan.org",
    });
  });

  test("returns Base metadata", () => {
    expect(getChainMeta(8453)).toEqual({
      id: 8453,
      name: "Base",
      rpcUrl: "https://mainnet.base.org",
      explorer: "https://basescan.org",
    });
  });

  test("returns fallback metadata for unknown chain", () => {
    expect(getChainMeta(99999)).toEqual({
      id: 99999,
      name: "Chain 99999",
      rpcUrl: "",
      explorer: "",
    });
  });
});