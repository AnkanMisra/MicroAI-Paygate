/**
 * Type definitions for MicroAI-Paygate SDK
 */

/**
 * Payment context returned by gateway in 402 response
 */
export interface PaymentContext {
  recipient: string;
  token: string;
  amount: string;
  nonce: string;
  chainId: number;
}

/**
 * Payment details in a receipt
 */
export interface PaymentDetails {
  payer: string;
  recipient: string;
  amount: string;
  token: string;
  chainId: number;
  nonce: string;
}

/**
 * Service details in a receipt
 */
export interface ServiceDetails {
  endpoint: string;
  request_hash: string;
  response_hash: string;
}

/**
 * Receipt structure
 */
export interface Receipt {
  id: string;
  version: string;
  timestamp: string;
  payment: PaymentDetails;
  service: ServiceDetails;
}

/**
 * Signed receipt from gateway
 */
export interface SignedReceipt {
  receipt: Receipt;
  signature: string;
  server_public_key: string;
}

/**
 * Response structure from gateway endpoints
 */
export interface PaygateResponse<T> {
  data: T;
  receipt: SignedReceipt;
}

/**
 * 402 Payment Required response
 */
export interface PaymentRequiredResponse {
  error: string;
  message: string;
  paymentContext: PaymentContext;
}

/**
 * Options for PaygateClient constructor
 */
export interface PaygateClientOptions {
  /** Base URL of the gateway server */
  serverUrl: string;
  
  /** Ethers signer for signing payments */
  signer: any; // ethers.Signer
  
  /** Optional: Verify receipts automatically (default: true) */
  autoVerifyReceipts?: boolean;
  
  /** Optional: Request timeout in ms (default: 30000) */
  timeout?: number;
}

/**
 * Options for individual requests
 */
export interface RequestOptions {
  /** Custom headers */
  headers?: Record<string, string>;
  
  /** Request timeout override */
  timeout?: number;
  
  /** Skip automatic receipt verification */
  skipReceiptVerification?: boolean;
}

/**
 * Error thrown when payment is required
 */
export class PaymentRequiredError extends Error {
  constructor(
    message: string,
    public paymentContext: PaymentContext
  ) {
    super(message);
    this.name = 'PaymentRequiredError';
  }
}

/**
 * Error thrown when receipt verification fails
 */
export class ReceiptVerificationError extends Error {
  constructor(
    message: string,
    public receipt: SignedReceipt
  ) {
    super(message);
    this.name = 'ReceiptVerificationError';
  }
}

/**
 * Error thrown by the gateway
 */
export class GatewayError extends Error {
  constructor(
    message: string,
    public statusCode: number,
    public details?: any
  ) {
    super(message);
    this.name = 'GatewayError';
  }
}
