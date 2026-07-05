import { GlobalRegistrator } from "@happy-dom/global-registrator";
GlobalRegistrator.register();

import { describe, it, expect, beforeEach, mock, afterEach, spyOn } from "bun:test";
import { renderHook, act } from "@testing-library/react";
import { useX402 } from "./use-x402";
import * as wallet from "@/lib/wallet";
import * as x402Client from "@/lib/x402-client";

// Mock ethers
mock.module("ethers", () => {
  return {
    ethers: {
      BrowserProvider: class {
        getSigner() {
          return { signTypedData: () => Promise.resolve("0xmock-signature") };
        }
      },
    },
  };
});

// Polyfill window.ethereum for the provider check
Object.defineProperty(globalThis, "window", { value: { ethereum: {} }, writable: true });

describe("useX402 chain switching logic", () => {
  beforeEach(() => {
    // Reset all mocks
    mock.restore();

    spyOn(wallet, "hasWallet").mockReturnValue(true);
    spyOn(wallet, "getProvider").mockReturnValue(true as unknown as ethers.BrowserProvider);
    spyOn(wallet, "getCurrentAccount").mockResolvedValue("0xmock-account");
    
    // Default fetch mocks for summarize
    spyOn(x402Client, "postSummarize").mockResolvedValue({
      ok: false,
      status: 402,
      json: async () => ({
        paymentContext: {
          recipient: "0x123",
          token: "USDC",
          amount: "0.001",
          nonce: "nonce1",
          chainId: 84532,
          timestamp: 123456789,
        }
      }),
      text: async () => "Payment Required",
    } as unknown as Response);

    spyOn(x402Client, "readPaymentChallenge").mockResolvedValue({
      recipient: "0x123",
      token: "USDC",
      amount: "0.001",
      nonce: "nonce1",
      chainId: 84532,
      timestamp: 123456789,
      supportedChains: [84532, 8453]
    });
  });

  afterEach(() => {
    mock.restore();
  });

  it("skips chain switch if currently on a supported chain", async () => {
    spyOn(wallet, "getCurrentChainId").mockResolvedValue(8453);
    const switchSpy = spyOn(wallet, "switchOrAddChain").mockResolvedValue();
    const signSpy = spyOn(x402Client, "signPaymentContext").mockResolvedValue("0xmock");

    const { result } = renderHook(() => useX402());
    await act(async () => {
      await result.current.submit("test");
    });

    expect(switchSpy).not.toHaveBeenCalled();
    // Should have signed with the chain we were actually on
    const signArgs = signSpy.mock.calls[0][1];
    expect(signArgs.chainId).toBe(8453);
  });

  it("triggers switch to default chain if on unsupported chain", async () => {
    spyOn(wallet, "getCurrentChainId")
      .mockResolvedValueOnce(1) // Initial check
      .mockResolvedValueOnce(84532); // Post-switch check
      
    const switchSpy = spyOn(wallet, "switchOrAddChain").mockResolvedValue();
    const signSpy = spyOn(x402Client, "signPaymentContext").mockResolvedValue("0xmock");

    const { result } = renderHook(() => useX402());
    await act(async () => {
      await result.current.submit("test");
    });

    expect(switchSpy).toHaveBeenCalledWith(84532);
    const signArgs = signSpy.mock.calls[0][1];
    expect(signArgs.chainId).toBe(84532);
  });

  it("surfaces error if wallet fails to switch to a supported chain", async () => {
    spyOn(wallet, "getCurrentChainId")
      .mockResolvedValueOnce(1) // Initial check
      .mockResolvedValueOnce(1); // Post-switch check fails
      
    const switchSpy = spyOn(wallet, "switchOrAddChain").mockResolvedValue();

    const { result } = renderHook(() => useX402());
    await act(async () => {
      await result.current.submit("test");
    });

    expect(switchSpy).toHaveBeenCalledWith(84532);
    expect(result.current.error?.message).toContain("Switch manually to one of: Base Sepolia, Base");
  });
});
