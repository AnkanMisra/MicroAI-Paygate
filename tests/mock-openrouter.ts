const port = Number(Bun.env.MOCK_OPENROUTER_PORT ?? "3100");

Bun.serve({
  hostname: "127.0.0.1",
  port,
  async fetch(request) {
    if (request.method === "GET" && new URL(request.url).pathname === "/health") {
      return Response.json({ status: "ok" });
    }
    if (request.method !== "POST") {
      return new Response("Not found", { status: 404 });
    }

    const body = await request.json() as {
      messages?: Array<{ content?: string }>;
    };
    const text = body.messages?.at(-1)?.content ?? "";
    return Response.json({
      choices: [{ message: { content: `Mock summary: ${text.slice(0, 80)}` } }],
    });
  },
});

console.log(`Mock OpenRouter listening on http://127.0.0.1:${port}`);
