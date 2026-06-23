import { describe, expect, mock, test } from "bun:test";
import { createWarmupProbes } from "./cold-start-warmup";

describe("createWarmupProbes", () => {

    test("adds verifier probe when verifier url is configured", () => {
      const originalFetch = globalThis.fetch;

      const fetchMock = mock(() =>
        Promise.resolve(new Response(null, { status: 200 })),
      );

      globalThis.fetch = fetchMock as unknown as typeof fetch;

      createWarmupProbes(
        "https://gateway.example",
        "https://verifier.example",
      );

      expect(fetchMock).toHaveBeenCalledTimes(2);

      globalThis.fetch = originalFetch;
    });
  test("creates only gateway probe when verifier url is not configured", () => {
    const originalFetch = globalThis.fetch;

    const fetchMock = mock(() =>
      Promise.resolve(new Response(null, { status: 200 })),
    );

    globalThis.fetch = fetchMock as unknown as typeof fetch;

    createWarmupProbes("https://gateway.example");

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith(
      "https://gateway.example/healthz",
      expect.any(Object),
    );

    globalThis.fetch = originalFetch;
  });
});