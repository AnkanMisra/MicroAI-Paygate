/**
 * PaygateClient - Main SDK class for interacting with MicroAI-Paygate
 */

import { ethers } from 'ethers';
import { signPaymentContext } from './eip712.js';
import { verifyReceipt } from './verify.js';
import type {
  PaymentContext,
  PaygateResponse,
  PaymentRequiredResponse,
  RequestOptions,
  SignedReceipt,
} from './types.js';

export class PaygateClient {
  private serverUrl: string;
  private signer: ethers.Signer;
  private autoVerifyReceipts: boolean;
  private defaultTimeout: number;

  /**
   * Create a new PaygateClient
   * 
   * @param serverUrl - Base URL of the gateway server
   * @param signer - Ethers signer for signing payments
   * @param options - Optional configuration
   * 
   * @example
   * ```typescript
   * import { PaygateClient } from '@microai/paygate-sdk';
   * import { ethers } from 'ethers';
   * 
   * const provider = new ethers.BrowserProvider(window.ethereum);
   * const signer = await provider.getSigner();
   * 
   * const client = new PaygateClient(
   *   'https://paygate.example.com',
   *   signer
   * );
   * ```
   */
  constructor(
    serverUrl: string,
    signer: ethers.Signer,
    options?: {
      autoVerifyReceipts?: boolean;
      timeout?: number;
    }
  ) {
    this.serverUrl = serverUrl.replace(/\/$/, ''); // Remove trailing slash
    this.signer = signer;
    this.autoVerifyReceipts = options?.autoVerifyReceipts ?? true;
    this.defaultTimeout = options?.timeout ?? 30000;
  }

  /**
   * Sign a payment context using EIP-712
   * 
   * @param context - Payment context from 402 response
   * @returns Hex-encoded signature
   * 
   * @example
   * ```typescript
   * const signature = await client.signPayment({
   *   recipient: '0x...',
   *   token: 'USDC',
   *   amount: '0.001',
   *   nonce: 'abc123',
   *   chainId: 8453
   * });
   * ```
   */
  async signPayment(context: PaymentContext): Promise<string> {
    return signPaymentContext(this.signer, context);
  }

  /**
   * Verify a receipt signature
   * 
   * @param receipt - Signed receipt from gateway
   * @returns true if valid
   * 
   * @example
   * ```typescript
   * const isValid = await client.verifyReceipt(response.receipt);
   * ```
   */
  async verifyReceipt(receipt: SignedReceipt): Promise<boolean> {
    return verifyReceipt(receipt);
  }

  /**
   * Make a request to the gateway with automatic 402 payment handling
   * 
   * @param endpoint - API endpoint (e.g., '/api/ai/summarize')
   * @param body - Request body
   * @param options - Request options
   * @returns Response data and receipt
   * 
   * @example
   * ```typescript
   * const { data, receipt } = await client.request('/api/ai/summarize', {
   *   text: 'Long article to summarize...'
   * });
   * 
   * console.log('Summary:', data.result);
   * console.log('Receipt ID:', receipt.receipt.id);
   * ```
   */
  async request<T = any>(
    endpoint: string,
    body: any,
    options?: RequestOptions
  ): Promise<PaygateResponse<T>> {
    const url = `${this.serverUrl}${endpoint}`;
    const timeout = options?.timeout ?? this.defaultTimeout;

    // Step 1: Initial request (no signature)
    let response = await this.fetchWithTimeout(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...options?.headers,
      },
      body: JSON.stringify(body),
    }, timeout);

    // Step 2: Handle 402 Payment Required
    if (response.status === 402) {
      const paymentRequired = await response.json() as PaymentRequiredResponse;
      
      if (!paymentRequired.paymentContext) {
        throw new Error('402 response missing payment context');
      }

      // Step 3: Sign the payment context
      const signature = await this.signPayment(paymentRequired.paymentContext);

      // Step 4: Retry request with signature
      response = await this.fetchWithTimeout(url, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-402-Signature': signature,
          'X-402-Nonce': paymentRequired.paymentContext.nonce,
          ...options?.headers,
        },
        body: JSON.stringify(body),
      }, timeout);
    }

    // Handle errors
    if (!response.ok) {
      const errorData = await response.json().catch(() => ({})) as any;
      throw new Error(`Gateway error ${response.status}: ${errorData.error || response.statusText}`);
    }

    // Step 5: Parse response with receipt
    const responseData = await response.json() as any;

    // Extract receipt from response
    const receipt: SignedReceipt = responseData.receipt;
    if (!receipt) {
      throw new Error('Response missing receipt');
    }

    // Step 6: Verify receipt (if enabled)
    if (this.autoVerifyReceipts && !options?.skipReceiptVerification) {
      const isValid = await this.verifyReceipt(receipt);
      if (!isValid) {
        throw new Error('Receipt verification failed');
      }
    }

    return {
      data: responseData,
      receipt,
    };
  }

  /**
   * Fetch with timeout
   */
  private async fetchWithTimeout(
    url: string,
    init: RequestInit,
    timeout: number
  ): Promise<Response> {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), timeout);

    try {
      const response = await fetch(url, {
        ...init,
        signal: controller.signal,
      });
      return response;
    } finally {
      clearTimeout(timeoutId);
    }
  }
}
