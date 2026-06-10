"use client";

import { useCallback, useRef, useState } from "react";
import { ethers } from "ethers";
import { browserAnalytics } from "@/lib/browser-analytics";
import { AnalyticsEvent, type AnalyticsEventName } from "@/lib/analytics-events";
import { createFlowContext, type AnalyticsProperties } from "@/lib/analytics";
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

/**
 * Manages the X402 summarization flow (request → optional payment challenge → wallet connect/sign → verify → receipt) and exposes the current flow state and control actions.
 *
 * Handles summary requests, payment challenges, wallet/chain handling, signing, verify retries, receipt persistence, error classification, and analytics emission.
 *
 * @returns An object containing the current hook state plus `submit(text)` to start a summarization run and `reset()` to cancel and reset the flow
 */
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
      | "chain-lookup"
      | "chain-switch"
      | "signer"
      | "sign"
      | "verify"
      | "done" = "request";

    const update = (patch: Partial<UseX402State>) => {
      if (runId.current !== myRun) return;
      setState((prev) => ({ ...prev, ...patch }));
    };

    // Guard analytics the same way as state: if a newer submit() has superseded
    // this run (e.g. a rapid double-click), a stale run must stop emitting funnel
    // events — otherwise it corrupts conversion metrics or re-identifies a wallet
    // for a run the user already abandoned.
    const isCurrent = () => runId.current === myRun;
    const analytics = browserAnalytics;
    const track = (event: AnalyticsEventName, props?: AnalyticsProperties) => {
      if (!isCurrent()) return;
      analytics.capture(event, props);
    };
    const identify = (walletAddress: string, props?: AnalyticsProperties) => {
      if (!isCurrent()) return;
      analytics.identifyWallet(walletAddress, props);
    };

    const flowProps = {
      flow_run_id: flow.flowRunId,
      correlation_id: flow.correlationId,
      input_word_count: flow.inputWordCount,
      input_char_count: flow.inputCharCount,
    };

    update({ step: "request", summary: null, receipt: null, error: null, isRunning: true });
    track(AnalyticsEvent.SummaryRequested, {
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
        track(AnalyticsEvent.SummaryCompleted, {
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
        track(AnalyticsEvent.SummaryFailed, {
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
      track(AnalyticsEvent.PaymentChallengeReceived, {
        ...flowProps,
        status_code: first.status,
        chain_id: context.chainId,
        payment_amount: context.amount,
        payment_token: context.token,
      });

      if (!hasWallet() || !getProvider()) {
        track(AnalyticsEvent.WalletConnectFailed, {
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
        track(AnalyticsEvent.WalletConnectRequested, flowProps);
        account = await connectWallet();
        track(AnalyticsEvent.WalletConnectSucceeded, {
          ...flowProps,
          wallet_connected: true,
        });
      }

      // Account is in hand; a failure in the chain lookup below is NOT a
      // wallet-connect failure, so move off the "wallet-connect" stage before
      // awaiting it to avoid corrupting the connect-conversion metric.
      stage = "chain-lookup";
      const currentChain = await getCurrentChainId();
      if (currentChain !== context.chainId) {
        stage = "chain-switch";
        track(AnalyticsEvent.ChainSwitchRequested, {
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
        track(AnalyticsEvent.ChainSwitchSucceeded, {
          ...flowProps,
          chain_id: context.chainId,
        });
      }

      stage = "signer";
      const refreshedProvider = new ethers.BrowserProvider(window.ethereum!);
      const signer = await refreshedProvider.getSigner(account);

      update({ step: "sign" });
      stage = "sign";
      track(AnalyticsEvent.PaymentSignatureStarted, {
        ...flowProps,
        chain_id: context.chainId,
      });
      const signature = await signPaymentContext(signer, context);
      // The signature prompt is modal and can take arbitrarily long. If the
      // user switched or disconnected accounts while it was open, the wallet's
      // accountsChanged handler has already reset analytics identity — so only
      // identify when the live account still matches the one we signed with,
      // otherwise we'd re-identify a stale wallet.
      const liveAccount = await getCurrentAccount();
      if (liveAccount && liveAccount.toLowerCase() === account.toLowerCase()) {
        identify(account, {
          wallet_connected: true,
          chain_id: context.chainId,
        });
      }
      track(AnalyticsEvent.PaymentSignatureSucceeded, {
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
        track(AnalyticsEvent.SignedRetrySent, {
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
        track(AnalyticsEvent.SummaryFailed, {
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
      track(AnalyticsEvent.SummaryCompleted, {
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
        track(AnalyticsEvent.WalletConnectFailed, {
          ...flowProps,
          stage,
          error_kind: classified.kind,
        });
      } else if (stage === "chain-switch") {
        track(AnalyticsEvent.ChainSwitchFailed, {
          ...flowProps,
          stage,
          error_kind: classified.kind,
        });
      } else if (stage === "sign") {
        track(AnalyticsEvent.PaymentSignatureFailed, {
          ...flowProps,
          stage,
          error_kind: classified.kind,
        });
      } else if (stage === "signer") {
        track(AnalyticsEvent.PaymentSignatureFailed, {
          ...flowProps,
          stage,
          error_kind: classified.kind,
        });
      } else if (stage === "verify") {
        track(AnalyticsEvent.SummaryFailed, {
          ...flowProps,
          stage,
          error_kind: classified.kind,
        });
      } else {
        track(AnalyticsEvent.SummaryFailed, {
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
