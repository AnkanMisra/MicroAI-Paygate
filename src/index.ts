import express from "express";
import cors from "cors";
import { CONFIG } from "./config";
import { aiRoutes } from "./routes/ai";
import { x402Middleware } from "./middleware/x402";
import { correlationMiddleware } from "./middleware/correlation"; // VIBE FIX: Import the new tool

const app = express();

app.use(cors());
app.use(express.json());

// VIBE FIX: Activate the tracing middleware immediately
// This ensures every request gets tagged with an ID before it hits the AI routes
app.use(correlationMiddleware);

// Health check (public)
app.get("/health", (req, res) => {
  res.json({ status: "ok", service: "MicroAI Paygate" });
});

// Protected AI Routes
// Apply x402 middleware to all routes under /api/ai
app.use("/api/ai", x402Middleware, aiRoutes);

app.listen(CONFIG.PORT, () => {
  console.log(`MicroAI Paygate running on port ${CONFIG.PORT}`);
  console.log(`Payment Recipient: ${CONFIG.PAYMENT.RECIPIENT_ADDRESS}`);
});
