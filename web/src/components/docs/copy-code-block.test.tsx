import { describe, expect, test } from "bun:test";
import { extractCopyText } from "./copy-code-block";

describe("extractCopyText", () => {
  test("preserves terminal command text from nested code children", () => {
    expect(
      extractCopyText(
        <code>
          cd sdk/typescript{"\n"}
          bun install{"\n"}
          bun run test
        </code>,
      ),
    ).toBe("cd sdk/typescript\nbun install\nbun run test");
  });
});
