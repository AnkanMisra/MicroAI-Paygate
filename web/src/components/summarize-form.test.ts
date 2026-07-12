import { describe, expect, test } from "bun:test";
import { getReceiptAnnouncement } from "./summarize-form";

describe("getReceiptAnnouncement", () => {
  test("returns no announcement before a summary exists", () => {
    expect(getReceiptAnnouncement(null, "missing")).toBe("");
  });

  test("announces each verification state with meaningful text", () => {
    expect(getReceiptAnnouncement("summary", "missing")).toBe("Receipt not returned.");
    expect(getReceiptAnnouncement("summary", "verifying")).toBe("Verifying receipt…");
    expect(getReceiptAnnouncement("summary", "valid")).toBe("Receipt verified.");
    expect(getReceiptAnnouncement("summary", "invalid")).toBe("Receipt not verified.");
  });
});
