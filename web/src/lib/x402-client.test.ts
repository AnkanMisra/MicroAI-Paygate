import { describe, expect, it } from "bun:test";
import { readSummarizeStream } from "./x402-client";

function streamResponse(chunks: string[]): Response {
  const encoder = new TextEncoder();
  return new Response(
    new ReadableStream({
      start(controller) {
        for (const chunk of chunks) {
          controller.enqueue(encoder.encode(chunk));
        }
        controller.close();
      },
    }),
    { headers: { "Content-Type": "text/event-stream" } },
  );
}

describe("readSummarizeStream", () => {
  it("streams chunk events and resolves the final done payload", async () => {
    const deltas: string[] = [];
    const res = streamResponse([
      'event: chunk\ndata: {"delta":"MicroAI "}\n\n',
      'event: chunk\ndata: {"delta":"streams."}\n\n',
      'event: done\ndata: {"result":"MicroAI streams.","receipt":null}\n\n',
    ]);

    const result = await readSummarizeStream(res, (delta) => deltas.push(delta));

    expect(deltas).toEqual(["MicroAI ", "streams."]);
    expect(result.summary).toBe("MicroAI streams.");
    expect(result.receipt).toBeNull();
  });

  it("handles CRLF framed server-sent events", async () => {
    const deltas: string[] = [];
    const res = streamResponse([
      'event: chunk\r\ndata: {"delta":"CRLF"}\r\n\r\n',
      'event: done\r\ndata: {"result":"CRLF","receipt":null}\r\n\r\n',
    ]);

    const result = await readSummarizeStream(res, (delta) => deltas.push(delta));

    expect(deltas).toEqual(["CRLF"]);
    expect(result.summary).toBe("CRLF");
  });

  it("throws the gateway stream error message", async () => {
    const res = streamResponse([
      'event: error\ndata: {"error":"upstream_timeout","message":"provider timed out"}\n\n',
    ]);

    await expect(readSummarizeStream(res, () => {})).rejects.toThrow("provider timed out");
  });

  it("throws when the stream closes before a done event", async () => {
    const res = streamResponse(['event: chunk\ndata: {"delta":"partial"}\n\n']);

    await expect(readSummarizeStream(res, () => {})).rejects.toThrow(
      "Streaming response ended before the done event",
    );
  });
});
