"use client";

import { useCallback, useRef, useState } from "react";
import { ethers } from "ethers";
import {
  buildSignedHeaders,
  postSummarize,
  readPaymentChallenge,
  readSummarizeSuccess,
  signPaymentContext,
} from "@/lib/x402-client";
import {
  connectWallet,
  getCurrentAccount,
  getCurrentChainId,
  getProvider,
  hasWallet,
  switchOrAddChain,
} from "@/lib/wallet";
import { saveReceipt } from "@/lib/receipt-storage";
import { classifyError, type ClassifiedError } from "@/lib/errors";
import type { SignedReceipt } from "@/lib/verify-receipt";
import type { X402Step } from "@/lib/types";

type UseX402State = {
  step: X402Step;
  summary: string | null;
  receipt: SignedReceipt | null;
  error: ClassifiedError | null;
  isRunning: boolean;
};

const INITIAL_STATE: UseX402State = {
  step: "idle",
  summary: null,
  receipt: null,
  error: null,
  isRunning: false,
};

export function useX402() {
  const [state, setState] = useState<UseX402State>(INITIAL_STATE);
  const runId = useRef(0);

  const reset = useCallback(() => {
    runId.current += 1;
    setState(INITIAL_STATE);
  }, []);

  const submit = useCallback(async (text: string) => {
    if (!text.trim()) return;
    const myRun = ++runId.current;

    const update = (patch: Partial<UseX402State>) => {
      if (runId.current !== myRun) return;
      setState((prev) => ({ ...prev, ...patch }));
    };

    update({ step: "request", summary: null, receipt: null, error: null, isRunning: true });

    try {
      const first = await postSummarize(text);

      if (first.status === 200) {
        update({ step: "receipt" });
        const { summary, receipt } = await readSummarizeSuccess(first);
        if (receipt) saveReceipt(receipt, text);
        update({ step: "done", summary, receipt, isRunning: false });
        return;
      }

      if (first.status !== 402) {
        const bodyText = await safeText(first);
        update({
          error: classifyError(null, { status: first.status, bodyText }),
          isRunning: false,
        });
        return;
      }

      update({ step: "challenge" });
      const context = await readPaymentChallenge(first);

      if (!hasWallet() || !getProvider()) {
        update({
          error: classifyError(new Error("No crypto wallet found")),
          isRunning: false,
        });
        return;
      }
      const account = (await getCurrentAccount()) ?? (await connectWallet());

      const currentChain = await getCurrentChainId();
      if (currentChain !== context.chainId) {
        await switchOrAddChain(context.chainId);
        // EIP-3085 (wallet_addEthereumChain) only ADDS a chain; some wallets
        // (e.g. Brave) won't auto-switch after adding. Re-check before signing
        // so we never embed the wrong chainId in EIP-712 typed data.
        const postSwitch = await getCurrentChainId();
        if (postSwitch !== context.chainId) {
          throw new Error(
            `Wallet did not switch to chain ${context.chainId} (still on ${postSwitch}). Switch manually and retry.`,
          );
        }
      }

      const refreshedProvider = new ethers.BrowserProvider(window.ethereum!);
      const signer = await refreshedProvider.getSigner(account);

      update({ step: "sign" });
      const signature = await signPaymentContext(signer, context);

      update({ step: "verify" });

      // Start the verify -> ai bump BEFORE awaiting the retry so the timer can
      // actually fire mid-flight. ~700ms is a reasonable verifier round-trip
      // ceiling. Wrapped in try/finally so a thrown fetch (network drop,
      // offline) doesn't leave the timer running — it would otherwise fire
      // after the outer catch already set state to "error" and incorrectly
      // bump the strip to "ai" on a dead run.
      const aiStepTimer = setTimeout(() => update({ step: "ai" }), 700);
      let retry: Response;
      try {
        retry = await postSummarize(text, buildSignedHeaders(context, signature));
      } finally {
        clearTimeout(aiStepTimer);
      }

      if (!retry.ok) {
        const bodyText = await safeText(retry);
        const classified = classifyError(null, { status: retry.status, bodyText });
        // If the gateway returned an AI-side failure (upstream timeout /
        // unavailable), the signature was accepted by the verifier — show the
        // strip at the AI step so the failure UI doesn't misattribute the
        // problem to verification. Verifier-side failures (verifier-timeout /
        // verifier-unavailable) DO mean signing failed, so leave the strip
        // at "verify" where the failure actually occurred.
        if (
          classified.kind === "ai-timeout" ||
          classified.kind === "ai-unavailable"
        ) {
          update({ step: "ai", error: classified, isRunning: false });
        } else {
          update({ error: classified, isRunning: false });
        }
        return;
      }

      update({ step: "receipt" });
      const { summary, receipt } = await readSummarizeSuccess(retry);
      if (receipt) saveReceipt(receipt, text);
      update({ step: "done", summary, receipt, isRunning: false });
    } catch (err) {
      if (runId.current !== myRun) return;
      update({ error: classifyError(err), isRunning: false });
    }
  }, []);

  return { ...state, submit, reset };
}

async function safeText(res: Response): Promise<string> {
  try {
    return await res.text();
  } catch {
    return "";
  }
}
