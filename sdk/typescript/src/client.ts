import { ethers } from "ethers";
import { PaygateSdkError } from "./errors";
import { MicroAIPaygateProtocol, readSseSuccessBody } from "./protocol/microai";
import type {
  FetchLike,
  PaygateProtocolAdapter,
  PaygateRequest,
  PaygateResponse,
  PaymentSigner,
  SignedReceipt,
} from "./protocol/types";
import { verifyReceipt } from "./receipts";

export type PaygateClientOptions = {
  gatewayUrl: string;
  signer: PaymentSigner;
  fetch?: FetchLike;
  protocol?: PaygateProtocolAdapter;
  trustedServerPublicKey?: string;
};

export class PaygateClient {
  private readonly gatewayUrl: string;
  private readonly signer: PaymentSigner;
  private readonly fetcher: FetchLike;
  private readonly protocol: PaygateProtocolAdapter;
  private readonly trustedServerPublicKey?: string;

  constructor(options: PaygateClientOptions) {
    this.gatewayUrl = options.gatewayUrl.replace(/\/+$/, "");
    this.signer = options.signer;
    this.fetcher = options.fetch ?? globalThis.fetch.bind(globalThis);
    this.protocol = options.protocol ?? new MicroAIPaygateProtocol();
    this.trustedServerPublicKey = options.trustedServerPublicKey;
  }

  summarize(text: string): Promise<PaygateResponse<{ result: string }>> {
    return this.request<{ text: string }, { result: string }>({
      method: "POST",
      path: "/api/ai/summarize",
      body: { text },
    });
  }

  async request<TBody, TData>(request: PaygateRequest<TBody>): Promise<PaygateResponse<TData>> {
    const url = this.buildUrl(request.path);
    const requestBodyText = this.serializeRequestBody(request.body);
    const successContext = {
      endpoint: this.buildReceiptEndpoint(request.path),
      requestBodyText,
    };
    const firstInit = this.buildRequestInit(request, {}, requestBodyText);
    const firstResponse = await this.fetchOrThrow(url, firstInit);

    if (firstResponse.status !== 402) {
      if (!firstResponse.ok) {
        throw new PaygateSdkError("network_error", "Gateway request failed", {
          status: firstResponse.status,
          bodyText: await firstResponse.text(),
        });
      }
      return this.readSuccess<TData>(firstResponse, successContext);
    }

    const paymentContext = await this.protocol.readPaymentContext(firstResponse);
    let signature: string;
    try {
      signature = await this.protocol.signPaymentContext(this.signer, paymentContext);
    } catch (error) {
      throw new PaygateSdkError("payment_signature_failed", "Failed to sign payment context", {
        cause: error,
      });
    }

    const signedHeaders = this.protocol.buildSignedHeaders(paymentContext, signature);
    const retryResponse = await this.fetchOrThrow(
      url,
      this.buildRequestInit(request, signedHeaders, requestBodyText),
    );

    if (!retryResponse.ok) {
      throw new PaygateSdkError("signed_retry_failed", "Signed retry failed", {
        status: retryResponse.status,
        bodyText: await retryResponse.text(),
      });
    }

    return this.readSuccess<TData>(retryResponse, successContext);
  }

  private buildUrl(path: string): string {
    if (/^[a-z][a-z\d+\-.]*:\/\//i.test(path)) {
      throw new PaygateSdkError(
        "network_error",
        "Request path must be relative to the configured gatewayUrl",
      );
    }
    return `${this.gatewayUrl}${path.startsWith("/") ? path : `/${path}`}`;
  }

  private serializeRequestBody<TBody>(body: TBody | undefined): string | undefined {
    if (body === undefined) return undefined;
    try {
      return JSON.stringify(body);
    } catch (error) {
      throw new PaygateSdkError("network_error", "Failed to serialize request body as JSON", {
        cause: error,
      });
    }
  }

  private buildReceiptEndpoint(path: string): string {
    const pathWithoutQuery = path.split(/[?#]/, 1)[0] ?? "";
    if (pathWithoutQuery === "") return "/";
    return pathWithoutQuery.startsWith("/") ? pathWithoutQuery : `/${pathWithoutQuery}`;
  }

  private buildRequestInit<TBody>(
    request: PaygateRequest<TBody>,
    extraHeaders: Record<string, string> = {},
    requestBodyText = this.serializeRequestBody(request.body),
  ): RequestInit {
    const headers = {
      ...(request.body === undefined ? {} : { "Content-Type": "application/json" }),
      ...(request.headers ?? {}),
      ...extraHeaders,
    };

    return {
      method: request.method,
      headers,
      body: requestBodyText,
    };
  }

  private async fetchOrThrow(input: RequestInfo | URL, init: RequestInit): Promise<Response> {
    try {
      return await this.fetcher(input, init);
    } catch (error) {
      throw new PaygateSdkError("network_error", "Network error while calling gateway", {
        cause: error,
      });
    }
  }

  private hashBody(bodyText: string | undefined): string {
    const hash = ethers.sha256(
      bodyText === undefined ? new Uint8Array() : ethers.toUtf8Bytes(bodyText),
    );
    return `sha256:${hash.slice(2)}`;
  }

  private receiptMatchesPayload(
    receipt: SignedReceipt,
    context: { endpoint: string; requestBodyText?: string },
    canonicalResponseBodyText: string,
  ): boolean {
    return (
      receipt.receipt.service.endpoint === context.endpoint &&
      receipt.receipt.service.request_hash === this.hashBody(context.requestBodyText) &&
      // The gateway hashes JSON.stringify({"result": fullSummary}) — we must
      // hash the same canonical form, not the raw SSE event-stream bytes.
      receipt.receipt.service.response_hash === this.hashBody(canonicalResponseBodyText)
    );
  }

  private async readSuccess<TData>(
    response: Response,
    context: { endpoint: string; requestBodyText?: string },
  ): Promise<PaygateResponse<TData>> {
    // The gateway streams the response as SSE events. Consume the stream,
    // accumulate text chunks, extract the embedded receipt event, and derive
    // the canonical response body string for receipt hash verification.
    const { fullText, receipt, canonicalResponseBody } = await readSseSuccessBody(response);

    // Expose the result in the same shape the summarize() caller expects:
    // { result: string }
    const data = { result: fullText } as unknown as TData;

    if (receipt === null) {
      return {
        data,
        receipt: null,
        receiptVerified: null,
        status: response.status,
      };
    }

    if (!this.receiptMatchesPayload(receipt, context, canonicalResponseBody)) {
      throw new PaygateSdkError(
        "receipt_verification_failed",
        "Gateway receipt does not match the request and response payload",
        { status: response.status },
      );
    }

    const receiptVerified =
      this.trustedServerPublicKey === undefined
        ? false
        : await verifyReceipt(receipt, { expectedServerPublicKey: this.trustedServerPublicKey });
    if (!receiptVerified) {
      if (this.trustedServerPublicKey === undefined) {
        return {
          data,
          receipt: receipt as SignedReceipt,
          receiptVerified: false,
          status: response.status,
        };
      }
      throw new PaygateSdkError(
        "receipt_verification_failed",
        "Gateway receipt signature did not verify",
        { status: response.status },
      );
    }

    return {
      data,
      receipt: receipt as SignedReceipt,
      receiptVerified,
      status: response.status,
    };
  }
}
