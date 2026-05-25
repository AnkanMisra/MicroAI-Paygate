import { PaygateSdkError } from "./errors";
import { MicroAIPaygateProtocol } from "./protocol/microai";
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
};

export class PaygateClient {
  private readonly gatewayUrl: string;
  private readonly signer: PaymentSigner;
  private readonly fetcher: FetchLike;
  private readonly protocol: PaygateProtocolAdapter;

  constructor(options: PaygateClientOptions) {
    this.gatewayUrl = options.gatewayUrl.replace(/\/+$/, "");
    this.signer = options.signer;
    this.fetcher = options.fetch ?? globalThis.fetch.bind(globalThis);
    this.protocol = options.protocol ?? new MicroAIPaygateProtocol();
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
    const firstInit = this.buildRequestInit(request);
    const firstResponse = await this.fetchOrThrow(url, firstInit);

    if (firstResponse.status !== 402) {
      if (!firstResponse.ok) {
        throw new PaygateSdkError("network_error", "Gateway request failed", {
          status: firstResponse.status,
          bodyText: await firstResponse.text(),
        });
      }
      return this.readSuccess<TData>(firstResponse);
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
      this.buildRequestInit(request, signedHeaders),
    );

    if (!retryResponse.ok) {
      throw new PaygateSdkError("signed_retry_failed", "Signed retry failed", {
        status: retryResponse.status,
        bodyText: await retryResponse.text(),
      });
    }

    return this.readSuccess<TData>(retryResponse);
  }

  private buildUrl(path: string): string {
    if (/^https?:\/\//i.test(path)) return path;
    return `${this.gatewayUrl}${path.startsWith("/") ? path : `/${path}`}`;
  }

  private buildRequestInit<TBody>(
    request: PaygateRequest<TBody>,
    extraHeaders: Record<string, string> = {},
  ): RequestInit {
    const headers = {
      ...(request.body === undefined ? {} : { "Content-Type": "application/json" }),
      ...(request.headers ?? {}),
      ...extraHeaders,
    };

    return {
      method: request.method,
      headers,
      body: request.body === undefined ? undefined : JSON.stringify(request.body),
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

  private async readSuccess<TData>(response: Response): Promise<PaygateResponse<TData>> {
    const data = (await response.json()) as TData;
    const receipt = this.protocol.readReceipt(response);
    if (receipt === null) {
      return {
        data,
        receipt: null,
        receiptVerified: null,
        status: response.status,
      };
    }

    const receiptVerified = await verifyReceipt(receipt);
    if (!receiptVerified) {
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
