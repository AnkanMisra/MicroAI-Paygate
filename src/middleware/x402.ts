import { Request, Response, NextFunction } from "express";
import { PaymentService, PaymentContext } from "../services/payment";
import { CONFIG } from "../config";

// Extend Express Request to include user wallet address
declare global {
  namespace Express {
    interface Request {
      userWallet?: string;
    }
  }
}

export const x402Middleware = async (req: Request, res: Response, next: NextFunction) => {
  const signature = req.headers["x-402-signature"] as string;
  const nonce = req.headers["x-402-nonce"] as string;
  const timestampHeader = req.headers["x-402-timestamp"] as string | undefined;

  // 1. If no signature/nonce, return 402 Payment Required
  if (!signature || !nonce) {
    const paymentContext = PaymentService.createPaymentContext();
    
    res.status(402).json({
      error: "Payment Required",
      message: "Please sign the payment context and retry with X-402-Signature and X-402-Nonce headers.",
      paymentContext
    });
    return;
  }

  // 2. Verify Payment
  // We need to reconstruct the context to verify. 
  // In a stateless retry, the client should ideally send back the context or we assume the default price.
  // For this MVP, we assume the default price and current config.
  // NOTE: In a real app, you might want to encode the price/params in the nonce or have the client send the full context back.
  // Here we trust the client to send the nonce we gave them, and we verify against our store.
  

  if (!timestampHeader) {
    res.status(400).json({
      error: "Invalid timestamp",
      details: "Missing X-402-Timestamp header",
    });
    return;
  }

  const parsedTimestamp = parseInt(timestampHeader, 10);
  if (!Number.isInteger(parsedTimestamp) || parsedTimestamp <= 0) {
    res.status(400).json({
      error: "Invalid timestamp",
      details: "Invalid X-402-Timestamp header",
    });
    return;
  }

  const context: PaymentContext = {
    recipient: CONFIG.PAYMENT.RECIPIENT_ADDRESS,
    token: CONFIG.PAYMENT.TOKEN_SYMBOL,
    amount: CONFIG.PAYMENT.DEFAULT_PRICE, // Assuming fixed price for now
    chainId: CONFIG.CHAIN_ID,
    nonce: nonce,
    timestamp: parsedTimestamp,
  };

  const verification = await PaymentService.verifyPayment(context, signature);

  if (!verification.isValid) {
    res.status(403).json({
      error: "Payment Verification Failed",
      details: verification.error
    });
    return;
  }

  // 3. Attach signer and proceed
  req.userWallet = verification.signer;
  console.log(`[x402] Payment verified from ${req.userWallet} for nonce ${nonce}`);
  
  next();
};
