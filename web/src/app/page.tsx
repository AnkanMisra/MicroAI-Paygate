"use client";

import { useState } from "react";
import { ethers } from "ethers";
import type { Eip1193Provider } from "ethers";

declare global {
  interface Window {
    ethereum?: Eip1193Provider;
  }
}

type PaymentContext = {
  recipient: string;
  token: string;
  amount: string;
  nonce: string;
  chainId: number;
  timestamp: number;
};

export default function Home() {
  const [input, setInput] = useState("");
  const [output, setOutput] = useState("");
  const [loading, setLoading] = useState(false);
  const [status, setStatus] = useState("");

  const handleSummarize = async () => {
    if (!input) return;
    setLoading(true);
    setStatus("Requesting...");
    setOutput("");

    try {
      // 1. Initial Request
      let response = await fetch(`${process.env.NEXT_PUBLIC_GATEWAY_URL||'http://localhost:3000'}/api/ai/summarize`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ text: input }),
      });

      if (response.status === 402) {
        setStatus("Payment Required (402). Requesting signature...");
        const data: { paymentContext: PaymentContext } = await response.json();
        const { paymentContext } = data;

        // 2. Check for Wallet
        if (!window.ethereum) {
          throw new Error("No crypto wallet found. Please install MetaMask.");
        }

        const provider = new ethers.BrowserProvider(window.ethereum);
        const signer = await provider.getSigner();

        // Switch chain if needed
        const network = await provider.getNetwork();
        if (Number(network.chainId) !== paymentContext.chainId) {
          try {
            await window.ethereum.request({
              method: "wallet_switchEthereumChain",
              params: [{ chainId: "0x" + paymentContext.chainId.toString(16) }],
            });
            // Re-get signer after switch
            const newProvider = new ethers.BrowserProvider(window.ethereum);
            await newProvider.getSigner(); 
          } catch (switchError: unknown) {
            // This error code indicates that the chain has not been added to MetaMask.
            if ((switchError as { code?: number }).code === 4902) {
              await window.ethereum.request({
                method: "wallet_addEthereumChain",
                params: [
                  {
                    chainId: "0x" + paymentContext.chainId.toString(16),
                    chainName: "Base",
                    rpcUrls: ["https://mainnet.base.org"],
                  },
                ],
              });
            } else {
              throw switchError;
            }
          }
        }

        // 3. Sign Payment
        const domain = {
          name: "MicroAI Paygate",
          version: "1",
          chainId: paymentContext.chainId,
          verifyingContract: ethers.ZeroAddress,
        };

        const types = {
          Payment: [
            { name: "recipient", type: "address" },
            { name: "token", type: "string" },
            { name: "amount", type: "string" },
            { name: "nonce", type: "string" },
            { name: "timestamp", type: "uint256" },
          ],
        };

        const value = {
          recipient: paymentContext.recipient,
          token: paymentContext.token,
          amount: paymentContext.amount,
          nonce: paymentContext.nonce,
          timestamp: paymentContext.timestamp,
        };

        setStatus("Please sign the transaction in your wallet...");
        const signature = await signer.signTypedData(domain, types, value);

        // 4. Retry Request
        setStatus("Signature received. Retrying request...");
        response = await fetch(`${process.env.NEXT_PUBLIC_GATEWAY_URL||'http://localhost:3000'}/api/ai/summarize`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "X-402-Signature": signature,
            "X-402-Nonce": paymentContext.nonce,
            "X-402-Timestamp": paymentContext.timestamp.toString(),
          },
          body: JSON.stringify({ text: input }),
        });
      }

      if (response.ok) {
        const result: { result?: string } = await response.json();
        setOutput(result.result ?? "");
        setStatus("Success!");
      } else {
        const errorText = await response.text();
        setStatus(`Error: ${response.status} - ${errorText}`);
      }
    } catch (error: unknown) {
      console.error(error);
      const message = error instanceof Error ? error.message : String(error);
      setStatus(`Error: ${message}`);
    } finally {
      setLoading(false);
    }
  };

  return (
    <main className="flex min-h-screen flex-col items-center justify-center p-24 bg-gray-900 text-white">
      <div className="z-10 max-w-5xl w-full items-center justify-between font-mono text-sm lg:flex flex-col gap-8">
        <h1 className="text-4xl font-bold text-transparent bg-clip-text bg-gradient-to-r from-blue-400 to-purple-600">
          MicroAI Paygate
        </h1>
        <p className="text-gray-400">
          Pay-per-request AI Summarization (0.001 USDC)
        </p>
        <div className="flex flex-wrap items-center gap-3 text-xs">
          <span className="rounded-full bg-gray-800 border border-gray-700 px-3 py-1 text-gray-200">
            CI Health: Ready
          </span>
          <span className="rounded-full bg-blue-900/40 border border-blue-700 px-3 py-1 text-blue-100">
            Go · Rust · Web checks run on every PR
          </span>
        </div>

        <div className="w-full max-w-2xl flex flex-col gap-4">
          <textarea
            className="w-full h-40 p-4 rounded-lg bg-gray-800 border border-gray-700 focus:border-blue-500 focus:outline-none text-white"
            placeholder="Enter text to summarize..."
            value={input}
            onChange={(e) => setInput(e.target.value)}
          />

          <button
            onClick={handleSummarize}
            disabled={loading || !input}
            className="w-full py-3 px-6 rounded-lg bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors font-bold"
          >
            {loading ? "Processing..." : "Summarize (Pay 0.001 USDC)"}
          </button>

          {status && (
            <div className={`p-4 rounded-lg ${status.startsWith("Error") ? "bg-red-900/50 text-red-200" : "bg-gray-800 text-blue-200"}`}>
              {status}
            </div>
          )}

          {output && (
            <div className="mt-8 p-6 rounded-lg bg-gray-800 border border-gray-700">
              <h3 className="text-xl font-bold mb-4 text-blue-400">Summary:</h3>
              <p className="leading-relaxed">{output}</p>
            </div>
          )}
        </div>
      </div>
    </main>
  );
}
