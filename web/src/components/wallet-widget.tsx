"use client";

import { useEffect, useState } from "react";
import {
  connectWallet,
  getChainMeta,
  getCurrentAccount,
  getCurrentChainId,
  hasWallet,
  shortenAddress,
  subscribeAccountsChanged,
  subscribeChainChanged,
  switchOrAddChain,
} from "@/lib/wallet";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";

// Honors NEXT_PUBLIC_EXPECTED_CHAIN_ID so deployments using gateway CHAIN_ID
// other than Base Sepolia (e.g. mainnet Base 8453) don't see the widget fight
// every payment-context the gateway issues. Defaults to Base Sepolia (84532).
const EXPECTED_CHAIN = Number(process.env.NEXT_PUBLIC_EXPECTED_CHAIN_ID ?? "84532");
const EXPECTED_CHAIN_NAME =
  process.env.NEXT_PUBLIC_EXPECTED_CHAIN_NAME ?? "Base Sepolia";
const ACTION_ERROR_TTL_MS = 6000;

type State =
  | { kind: "loading" }
  | { kind: "missing" }
  | { kind: "disconnected" }
  | { kind: "connected"; address: string; chainId: number };

export function WalletWidget() {
  const [state, setState] = useState<State>({ kind: "loading" });
  const [switching, setSwitching] = useState(false);
  // Visible inline error chip for non-rejection provider failures (provider
  // crash, network error, silent no-op switch). Auto-clears after a few
  // seconds so it doesn't linger forever.
  const [actionError, setActionError] = useState<string | null>(null);

  useEffect(() => {
    if (!actionError) return;
    const id = window.setTimeout(() => setActionError(null), ACTION_ERROR_TTL_MS);
    return () => window.clearTimeout(id);
  }, [actionError]);

  useEffect(() => {
    let mounted = true;

    if (!hasWallet()) {
      setState({ kind: "missing" });
      return;
    }

    async function load() {
      try {
        const [addr, chain] = await Promise.all([getCurrentAccount(), getCurrentChainId()]);
        if (!mounted) return;
        if (addr && chain != null) {
          setState({ kind: "connected", address: addr, chainId: chain });
        } else {
          setState({ kind: "disconnected" });
        }
      } catch (err) {
        if (!mounted) return;
        // Provider hiccup during hydration must never leave the widget stuck
        // showing "Checking wallet…" forever.
        console.warn("wallet-widget: initial load failed", err);
        setState({ kind: "disconnected" });
      }
    }
    void load();

    const unsubAcc = subscribeAccountsChanged(async (accounts) => {
      if (!accounts[0]) {
        setState({ kind: "disconnected" });
        return;
      }
      // Read the live chainId — otherwise an external connect lands us in
      // `chainId: 0`, which is never a real EVM chain and triggers the
      // wrong-chain CTA even when the wallet is already correct.
      const chain = await getCurrentChainId();
      setState((prev) =>
        prev.kind === "connected"
          ? { ...prev, address: accounts[0] }
          : { kind: "connected", address: accounts[0], chainId: chain ?? 0 },
      );
    });

    const unsubChain = subscribeChainChanged((hex) => {
      const chainId = parseInt(hex, 16);
      setState((prev) =>
        prev.kind === "connected" ? { ...prev, chainId } : prev,
      );
    });

    return () => {
      mounted = false;
      unsubAcc();
      unsubChain();
    };
  }, []);

  if (state.kind === "loading") {
    return <Badge tone="muted">Checking wallet…</Badge>;
  }

  if (state.kind === "missing") {
    return (
      <a
        href="https://metamask.io/download"
        target="_blank"
        rel="noreferrer"
        className="inline-flex items-center"
      >
        <Badge tone="alert">No wallet · install MetaMask</Badge>
      </a>
    );
  }

  if (state.kind === "disconnected") {
    return (
      <div className="flex items-center gap-2">
        <Button
          size="sm"
          variant="secondary"
          onClick={async () => {
            setActionError(null);
            try {
              const addr = await connectWallet();
              const chain = await getCurrentChainId();
              setState({ kind: "connected", address: addr, chainId: chain ?? 0 });
            } catch (err) {
              if (!isUserRejection(err)) {
                console.error("wallet-widget: connect failed", err);
                setActionError("Couldn't connect to wallet — try again or refresh.");
              }
            }
          }}
        >
          Connect wallet
        </Button>
        {actionError && <ActionErrorChip msg={actionError} />}
      </div>
    );
  }

  const onCorrect = state.chainId === EXPECTED_CHAIN;
  const meta = getChainMeta(state.chainId);

  return (
    <div className="flex items-center gap-2">
      <Badge tone={onCorrect ? "ok" : "alert"}>
        {onCorrect ? "✓ " : "✗ "}
        {meta.name}
      </Badge>
      <span className="hidden font-mono text-xs tracking-tight tnum text-ink-soft sm:inline">
        {shortenAddress(state.address)}
      </span>
      {!onCorrect && (
        <Button
          size="sm"
          variant="danger"
          disabled={switching}
          onClick={async () => {
            setSwitching(true);
            setActionError(null);
            try {
              await switchOrAddChain(EXPECTED_CHAIN);
              // EIP-3085 (wallet_addEthereumChain) only ADDS — Brave and some
              // injected providers don't auto-switch after adding. Verify so
              // the user gets explicit feedback instead of a silently-no-op
              // button click.
              const postSwitch = await getCurrentChainId();
              if (postSwitch !== EXPECTED_CHAIN) {
                setActionError(
                  `Switch to ${EXPECTED_CHAIN_NAME} didn't take. Open your wallet and switch manually.`,
                );
              } else {
                // Close the chainChanged race window — update state immediately
                // so the badge flips to ok instead of waiting on the event.
                setState((prev) =>
                  prev.kind === "connected"
                    ? { ...prev, chainId: EXPECTED_CHAIN }
                    : prev,
                );
              }
            } catch (err) {
              if (!isUserRejection(err)) {
                console.error("wallet-widget: chain switch failed", err);
                setActionError(`Couldn't switch to ${EXPECTED_CHAIN_NAME} — check your wallet.`);
              }
            } finally {
              setSwitching(false);
            }
          }}
        >
          {switching ? "Switching…" : `Switch to ${EXPECTED_CHAIN_NAME}`}
        </Button>
      )}
      {actionError && <ActionErrorChip msg={actionError} />}
    </div>
  );
}

function ActionErrorChip({ msg }: { msg: string }) {
  return (
    <span
      role="alert"
      title={msg}
      className="inline-block max-w-[140px] truncate border border-alert bg-alert-soft px-2 py-1 font-mono text-[10px] uppercase tracking-[0.12em] text-alert sm:max-w-[240px]"
    >
      ! {msg}
    </span>
  );
}

function isUserRejection(err: unknown): boolean {
  if (typeof err !== "object" || err === null) return false;
  const e = err as { code?: number | string; message?: string };
  if (e.code === 4001 || e.code === "ACTION_REJECTED") return true;
  const m = (e.message ?? "").toLowerCase();
  return (
    m.includes("user rejected") ||
    m.includes("user denied") ||
    m.includes("rejected the request") ||
    m.includes("action_rejected")
  );
}
