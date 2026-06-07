"use client";

import { useCallback, useRef, useState } from "react";
import { ethers } from "ethers";
import { browserAnalytics } from "@/lib/browser-analytics";
import { AnalyticsEvent } from "@/lib/analytics-events";
import { createFlowContext } from "@/lib/analytics";
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
    const flow = createFlowContext(text);
    let stage:
      | "request"
      | "wallet-connect"
      | "chain-switch"
      | "sign"
      | "verify"
      | "done" = "request";

    const update = (patch: Partial<UseX402State>) => {
      if (runId.current !== myRun) return;
      setState((prev) => ({ ...prev, ...patch }));
    };

    const flowProps = {
      flow_run_id: flow.flowRunId,
      correlation_id: flow.correlationId,
      input_word_count: flow.inputWordCount,
      input_char_count: flow.inputCharCount,
    };

    update({ step: "request", summary: null, receipt: null, error: null, isRunning: true });
    browserAnalytics.capture(AnalyticsEvent.SummaryRequested, {
      ...flowProps,
      wallet_available: hasWallet(),
    });

    try {
      const first = await postSummarize(text, {
        "X-Correlation-ID": flow.correlationId,
      });

      if (first.status === 200) {
        update({ step: "receipt" });
        const { summary, receipt } = await readSummarizeSuccess(first);
        if (receipt) saveReceipt(receipt, text);
        browserAnalytics.capture(AnalyticsEvent.SummaryCompleted, {
          ...flowProps,
          status_code: first.status,
          has_receipt: !!receipt,
          summary_char_count: summary.length,
        });
        stage = "done";
        update({ step: "done", summary, receipt, isRunning: false });
        return;
      }

      if (first.status !== 402) {
        const bodyText = await safeText(first);
        const classified = classifyError(null, { status: first.status, bodyText });
        browserAnalytics.capture(AnalyticsEvent.SummaryFailed, {
          ...flowProps,
          stage,
          status_code: first.status,
          error_kind: classified.kind,
        });
        update({
          error: classified,
          isRunning: false,
        });
        return;
      }

      update({ step: "challenge" });
      const context = await readPaymentChallenge(first);
      browserAnalytics.capture(AnalyticsEvent.PaymentChallengeReceived, {
        ...flowProps,
        status_code: first.status,
        chain_id: context.chainId,
        payment_amount: context.amount,
        payment_token: context.token,
      });

      if (!hasWallet() || !getProvider()) {
        browserAnalytics.capture(AnalyticsEvent.WalletConnectFailed, {
          ...flowProps,
          stage: "wallet-connect",
          error_kind: "no-wallet",
        });
        update({
          error: classifyError(new Error("No crypto wallet found")),
          isRunning: false,
        });
        return;
      }
      stage = "wallet-connect";
      let account = await getCurrentAccount();
      if (!account) {
        browserAnalytics.capture(AnalyticsEvent.WalletConnectRequested, flowProps);
        account = await connectWallet();
        browserAnalytics.capture(AnalyticsEvent.WalletConnectSucceeded, {
          ...flowProps,
          wallet_connected: true,
        });
      }

      const currentChain = await getCurrentChainId();
      if (currentChain !== context.chainId) {
        stage = "chain-switch";
        browserAnalytics.capture(AnalyticsEvent.ChainSwitchRequested, {
          ...flowProps,
          chain_id: context.chainId,
          current_chain_id: currentChain,
        });
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
        browserAnalytics.capture(AnalyticsEvent.ChainSwitchSucceeded, {
          ...flowProps,
          chain_id: context.chainId,
        });
      }

      const refreshedProvider = new ethers.BrowserProvider(window.ethereum!);
      const signer = await refreshedProvider.getSigner(account);

      update({ step: "sign" });
      stage = "sign";
      browserAnalytics.capture(AnalyticsEvent.PaymentSignatureStarted, {
        ...flowProps,
        chain_id: context.chainId,
      });
      const signature = await signPaymentContext(signer, context);
      browserAnalytics.identifyWallet(account, {
        wallet_connected: true,
        chain_id: context.chainId,
      });
      browserAnalytics.capture(AnalyticsEvent.PaymentSignatureSucceeded, {
        ...flowProps,
        chain_id: context.chainId,
      });

      update({ step: "verify" });
      stage = "verify";

      // Start the verify -> ai bump BEFORE awaiting the retry so the timer can
      // actually fire mid-flight. ~700ms is a reasonable verifier round-trip
      // ceiling. Wrapped in try/finally so a thrown fetch (network drop,
      // offline) doesn't leave the timer running — it would otherwise fire
      // after the outer catch already set state to "error" and incorrectly
      // bump the strip to "ai" on a dead run.
      const aiStepTimer = setTimeout(() => update({ step: "ai" }), 700);
      let retry: Response;
      try {
        browserAnalytics.capture(AnalyticsEvent.SignedRetrySent, {
          ...flowProps,
          chain_id: context.chainId,
        });
        retry = await postSummarize(text, {
          ...buildSignedHeaders(context, signature),
          "X-Correlation-ID": flow.correlationId,
        });
      } finally {
        clearTimeout(aiStepTimer);
      }

      if (!retry.ok) {
        const bodyText = await safeText(retry);
        const classified = classifyError(null, { status: retry.status, bodyText });
        browserAnalytics.capture(AnalyticsEvent.SummaryFailed, {
          ...flowProps,
          stage,
          status_code: retry.status,
          error_kind: classified.kind,
        });
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
      browserAnalytics.capture(AnalyticsEvent.SummaryCompleted, {
        ...flowProps,
        status_code: retry.status,
        has_receipt: !!receipt,
        summary_char_count: summary.length,
      });
      stage = "done";
      update({ step: "done", summary, receipt, isRunning: false });
    } catch (err) {
      if (runId.current !== myRun) return;
      const classified = classifyError(err);
      if (stage === "wallet-connect") {
        browserAnalytics.capture(AnalyticsEvent.WalletConnectFailed, {
          ...flowProps,
          stage,
          error_kind: classified.kind,
        });
      } else if (stage === "chain-switch") {
        browserAnalytics.capture(AnalyticsEvent.ChainSwitchFailed, {
          ...flowProps,
          stage,
          error_kind: classified.kind,
        });
      } else if (stage === "sign") {
        browserAnalytics.capture(AnalyticsEvent.PaymentSignatureFailed, {
          ...flowProps,
          stage,
          error_kind: classified.kind,
        });
      } else {
        browserAnalytics.capture(AnalyticsEvent.SummaryFailed, {
          ...flowProps,
          stage,
          error_kind: classified.kind,
        });
      }
      update({ error: classified, isRunning: false });
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
