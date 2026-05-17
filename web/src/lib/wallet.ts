import { ethers, type Eip1193Provider } from "ethers";

declare global {
  interface Window {
    ethereum?: Eip1193Provider & {
      on?: (event: string, handler: (...args: unknown[]) => void) => void;
      removeListener?: (event: string, handler: (...args: unknown[]) => void) => void;
    };
  }
}

export type ChainMeta = {
  id: number;
  name: string;
  rpcUrl: string;
  explorer: string;
};

const CHAINS: Record<number, ChainMeta> = {
  84532: {
    id: 84532,
    name: "Base Sepolia",
    rpcUrl: "https://sepolia.base.org",
    explorer: "https://sepolia.basescan.org",
  },
  8453: {
    id: 8453,
    name: "Base",
    rpcUrl: "https://mainnet.base.org",
    explorer: "https://basescan.org",
  },
};

export function getChainMeta(chainId: number): ChainMeta {
  return (
    CHAINS[chainId] ?? {
      id: chainId,
      name: `Chain ${chainId}`,
      rpcUrl: "",
      explorer: "",
    }
  );
}

export function hasWallet(): boolean {
  return typeof window !== "undefined" && !!window.ethereum;
}

export function getProvider(): ethers.BrowserProvider | null {
  if (!hasWallet()) return null;
  return new ethers.BrowserProvider(window.ethereum!);
}

export async function getCurrentAccount(): Promise<string | null> {
  const provider = getProvider();
  if (!provider) return null;
  const accounts: string[] = await provider.send("eth_accounts", []);
  return accounts[0] ?? null;
}

export async function getCurrentChainId(): Promise<number | null> {
  const provider = getProvider();
  if (!provider) return null;
  const net = await provider.getNetwork();
  return Number(net.chainId);
}

export async function connectWallet(): Promise<string> {
  const provider = getProvider();
  if (!provider) throw new Error("No wallet found");
  const accounts: string[] = await provider.send("eth_requestAccounts", []);
  if (!accounts[0]) throw new Error("Wallet returned no accounts");
  return accounts[0];
}

export async function switchOrAddChain(chainId: number): Promise<void> {
  if (!hasWallet()) throw new Error("No wallet found");
  const eth = window.ethereum!;
  const hex = "0x" + chainId.toString(16);

  try {
    await eth.request({ method: "wallet_switchEthereumChain", params: [{ chainId: hex }] });
    return;
  } catch (err: unknown) {
    // EIP-3085 says 4902 means "chain not added". MetaMask v11+ sometimes
    // returns a text-based error before settling on 4902 — match both so the
    // wallet_addEthereumChain fallback fires for first-time chain setup.
    const e = err as { code?: number; message?: string };
    const isUnknownChain =
      e.code === 4902 ||
      String(e.message ?? "")
        .toLowerCase()
        .includes("chain not supported");
    if (!isUnknownChain) throw err;
  }

  const meta = getChainMeta(chainId);
  if (!meta.rpcUrl) throw new Error(`Unknown chain ${chainId} — cannot add to wallet`);

  await eth.request({
    method: "wallet_addEthereumChain",
    params: [
      {
        chainId: hex,
        chainName: meta.name,
        nativeCurrency: { name: "Ether", symbol: "ETH", decimals: 18 },
        rpcUrls: [meta.rpcUrl],
        blockExplorerUrls: [meta.explorer],
      },
    ],
  });
}

export function shortenAddress(addr: string, chars = 4): string {
  if (!addr) return "";
  if (addr.length <= chars * 2 + 2) return addr;
  return `${addr.slice(0, chars + 2)}…${addr.slice(-chars)}`;
}

export function subscribeAccountsChanged(handler: (accounts: string[]) => void): () => void {
  if (!hasWallet() || !window.ethereum?.on) return () => {};
  const eth = window.ethereum;
  const fn = (...args: unknown[]) => handler(args[0] as string[]);
  eth.on!("accountsChanged", fn);
  return () => eth.removeListener?.("accountsChanged", fn);
}

export function subscribeChainChanged(handler: (chainIdHex: string) => void): () => void {
  if (!hasWallet() || !window.ethereum?.on) return () => {};
  const eth = window.ethereum;
  const fn = (...args: unknown[]) => handler(args[0] as string);
  eth.on!("chainChanged", fn);
  return () => eth.removeListener?.("chainChanged", fn);
}
