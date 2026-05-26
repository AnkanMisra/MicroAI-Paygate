import { describe, expect, it } from "bun:test";
import { DOCS_NAV_ITEMS } from "./docs-nav";

describe("docs navigation", () => {
  it("defines the full platform docs route set in order", () => {
    expect(DOCS_NAV_ITEMS.map((item) => item.href)).toEqual([
      "/docs",
      "/docs/quickstart",
      "/docs/sdk",
      "/docs/api",
      "/docs/protocol",
      "/docs/architecture",
      "/docs/operations",
      "/docs/security-limits",
    ]);
  });

  it("uses unique routes and labels", () => {
    expect(new Set(DOCS_NAV_ITEMS.map((item) => item.href)).size).toBe(DOCS_NAV_ITEMS.length);
    expect(new Set(DOCS_NAV_ITEMS.map((item) => item.label)).size).toBe(DOCS_NAV_ITEMS.length);
  });
});
