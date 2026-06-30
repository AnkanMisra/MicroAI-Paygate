import "../test/setup-dom";

import { afterEach, beforeEach, expect, mock, test } from "bun:test";
import { cleanup, render, waitFor } from "@testing-library/react";

const originalFetch = globalThis.fetch;
const originalGateway = process.env.NEXT_PUBLIC_GATEWAY_URL;
const originalVerifier = process.env.NEXT_PUBLIC_VERIFIER_URL;

beforeEach(() => {
  process.env.NEXT_PUBLIC_GATEWAY_URL = "https://gateway.example";
  delete process.env.NEXT_PUBLIC_VERIFIER_URL;
});

afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;

  if (originalGateway === undefined) {
    delete process.env.NEXT_PUBLIC_GATEWAY_URL;
  } else {
    process.env.NEXT_PUBLIC_GATEWAY_URL = originalGateway;
  }

  if (originalVerifier === undefined) {
    delete process.env.NEXT_PUBLIC_VERIFIER_URL;
  } else {
    process.env.NEXT_PUBLIC_VERIFIER_URL = originalVerifier;
  }
});

test("renders nothing when NEXT_PUBLIC_GATEWAY_URL is unset", async () => {
  delete process.env.NEXT_PUBLIC_GATEWAY_URL;

  const { ColdStartWarmup } = await import("./cold-start-warmup");

  const { container } = render(<ColdStartWarmup />);

  expect(container.firstChild).toBeNull();
});

test("banner shows while fetch is pending", async () => {
  const pendingFetch = mock(() => new Promise<Response>(() => {}));

  globalThis.fetch = pendingFetch as unknown as typeof fetch;

  const { ColdStartWarmup } = await import("./cold-start-warmup");

  const { getByText } = render(<ColdStartWarmup />);

  await waitFor(() => {
    expect(getByText(/Free tier wake-up/i)).toBeTruthy();
  });
});

test("banner disappears after fetch settles", async () => {
  let resolveFetch: (value: Response) => void;

  const pendingFetch = mock(
    () =>
      new Promise<Response>((resolve) => {
        resolveFetch = resolve;
      })
  );

  globalThis.fetch = pendingFetch as unknown as typeof fetch;

  const { ColdStartWarmup } = await import("./cold-start-warmup");

  const { getByText, queryByText } = render(<ColdStartWarmup />);

  expect(getByText(/Free tier wake-up/i)).toBeTruthy();

  resolveFetch!(
    new Response("OK", { status: 200, statusText: "OK" })
  );

  await waitFor(() => {
    expect(queryByText(/Free tier wake-up/i)).toBeNull();
  });
});

test("verifier probe is added when NEXT_PUBLIC_VERIFIER_URL is configured", async () => {
  process.env.NEXT_PUBLIC_VERIFIER_URL = "https://verifier.example";

  const fetchCalls: string[] = [];

  const trackedFetch = mock((url: string) => {
    fetchCalls.push(url);
    return Promise.resolve(new Response("OK", { status: 200 }));
  });

  globalThis.fetch = trackedFetch as unknown as typeof fetch;

  const { ColdStartWarmup } = await import("./cold-start-warmup");

  render(<ColdStartWarmup />);

  await waitFor(() => {
    expect(fetchCalls).toContain("https://gateway.example/healthz");
    expect(fetchCalls).toContain("https://verifier.example/health");
  });
});

test("fetch failures still warm the component", async () => {
  const failingFetch = mock(() =>
    Promise.reject(new Error("Network error"))
  );

  globalThis.fetch = failingFetch as unknown as typeof fetch;

  const { ColdStartWarmup } = await import("./cold-start-warmup");

  const { getByText, queryByText } = render(<ColdStartWarmup />);

  await waitFor(() => {
    expect(getByText(/Free tier wake-up/i)).toBeTruthy();
  });

  await waitFor(() => {
    expect(queryByText(/Free tier wake-up/i)).toBeNull();
  });
});