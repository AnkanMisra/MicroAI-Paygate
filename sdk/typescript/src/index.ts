/**
 * @microai/paygate-sdk
 * 
 * TypeScript SDK for MicroAI-Paygate with EIP-712 signing and receipt verification
 */

export { PaygateClient } from './client.js';
export { signPaymentContext, recoverSigner, getEIP712Domain, PaymentContextTypes } from './eip712.js';
export { verifyReceipt, validateReceiptFormat } from './verify.js';
export type {
  PaymentContext,
  PaymentDetails,
  ServiceDetails,
  Receipt,
  SignedReceipt,
  PaygateResponse,
  PaymentRequiredResponse,
  PaygateClientOptions,
  RequestOptions,
} from './types.js';
export {
  PaymentRequiredError,
  ReceiptVerificationError,
  GatewayError,
} from './types.js';
