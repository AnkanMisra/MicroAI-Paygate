import { describe, expect, it } from "bun:test";
import { existsSync, readdirSync } from "node:fs";
import { join, resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { DOCS_NAV_ITEMS } from "./docs-nav";

const __dirname = dirname(fileURLToPath(import.meta.url));
const DOCS_PAGES_DIR = resolve(__dirname, "../../app/docs");
const PAGE_EXTENSIONS = [".mdx", ".tsx", ".ts", ".jsx", ".js"];

function pageExists(href: string): boolean {
  if (href === "/docs") {
    return PAGE_EXTENSIONS.some((ext) =>
      existsSync(join(DOCS_PAGES_DIR, `page${ext}`)),
    );
  }
  const slug = href.replace("/docs/", "");
  return PAGE_EXTENSIONS.some((ext) =>
    existsSync(join(DOCS_PAGES_DIR, slug, `page${ext}`)),
  );
}

function discoverDocsPageSlugs(): string[] {
  const slugs: string[] = [];
  const entries = readdirSync(DOCS_PAGES_DIR, { withFileTypes: true });
  for (const entry of entries) {
    if (!entry.isDirectory()) continue;
    const dirPath = join(DOCS_PAGES_DIR, entry.name);
    const hasPage = PAGE_EXTENSIONS.some((ext) =>
      existsSync(join(dirPath, `page${ext}`)),
    );
    if (hasPage) slugs.push(`/docs/${entry.name}`);
  }
  return slugs.sort();
}

const NAV_HREFS = DOCS_NAV_ITEMS.map((item) => item.href);

describe("docs navigation", () => {
  it("defines the full platform docs route set in order", () => {
    expect(NAV_HREFS).toEqual([
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
    expect(new Set(NAV_HREFS).size).toBe(NAV_HREFS.length);
    expect(new Set(DOCS_NAV_ITEMS.map((item) => item.label)).size).toBe(DOCS_NAV_ITEMS.length);
  });

  it("every nav link points to an existing docs page file", () => {
    const missing = DOCS_NAV_ITEMS.filter((item) => !pageExists(item.href));
    expect(missing).toHaveLength(0);
  });

  it("every docs page directory has a corresponding nav entry", () => {
    const actualSlugs = discoverDocsPageSlugs();
    const orphaned = actualSlugs.filter((slug) => !NAV_HREFS.includes(slug));
    expect(orphaned).toHaveLength(0);
  });
});
