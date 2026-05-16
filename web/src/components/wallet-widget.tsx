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

const EXPECTED_CHAIN = 84532;

type State =
  | { kind: "loading" }
  | { kind: "missing" }
  | { kind: "disconnected" }
  | { kind: "connected"; address: string; chainId: number };

export function WalletWidget() {
  const [state, setState] = useState<State>({ kind: "loading" });
  const [switching, setSwitching] = useState(false);

  useEffect(() => {
    let mounted = true;

    if (!hasWallet()) {
      setState({ kind: "missing" });
      return;
    }

    async function load() {
      const [addr, chain] = await Promise.all([getCurrentAccount(), getCurrentChainId()]);
      if (!mounted) return;
      if (addr && chain != null) {
        setState({ kind: "connected", address: addr, chainId: chain });
      } else {
        setState({ kind: "disconnected" });
      }
    }
    void load();

    const unsubAcc = subscribeAccountsChanged((accounts) => {
      if (!accounts[0]) {
        setState({ kind: "disconnected" });
        return;
      }
      setState((prev) =>
        prev.kind === "connected"
          ? { ...prev, address: accounts[0] }
          : { kind: "connected", address: accounts[0], chainId: 0 },
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
      <Button
        size="sm"
        variant="secondary"
        onClick={async () => {
          try {
            const addr = await connectWallet();
            const chain = await getCurrentChainId();
            setState({ kind: "connected", address: addr, chainId: chain ?? 0 });
          } catch {
            /* user dismissed */
          }
        }}
      >
        Connect wallet
      </Button>
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
            try {
              await switchOrAddChain(EXPECTED_CHAIN);
            } catch {
              /* user dismissed */
            } finally {
              setSwitching(false);
            }
          }}
        >
          {switching ? "Switching…" : "Switch to Base Sepolia"}
        </Button>
      )}
    </div>
  );
}
