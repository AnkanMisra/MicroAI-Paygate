import type { TypedDataDomain, TypedDataField } from "ethers";

export type PaymentContext = {
  recipient: string;
  token: string;
  amount: string;
  nonce: string;
  chainId: number;
  timestamp: number;
};

export type PaymentSigner = {
  signTypedData(
    domain: TypedDataDomain,
    types: Record<string, Array<TypedDataField>>,
    value: Record<string, unknown>,
  ): Promise<string>;
};

export type PaymentDetails = {
  payer: string;
  recipient: string;
  amount: string;
  token: string;
  chainId: number;
  nonce: string;
};

export type ServiceDetails = {
  endpoint: string;
  request_hash: string;
  response_hash: string;
};

export type Receipt = {
  id: string;
  version: string;
  timestamp: string;
  payment: PaymentDetails;
  service: ServiceDetails;
};

export type SignedReceipt = {
  receipt: Receipt;
  signature: string;
  server_public_key: string;
};

export type PaygateResponse<T> = {
  data: T;
  receipt: SignedReceipt | null;
  receiptVerified: boolean | null;
  status: number;
};

export type PaygateRequest<TBody> = {
  method: string;
  path: string;
  body?: TBody;
  headers?: Record<string, string>;
};

export type FetchLike = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

export type PaygateProtocolAdapter = {
  readPaymentContext(response: Response): Promise<PaymentContext>;
  signPaymentContext(signer: PaymentSigner, ctx: PaymentContext): Promise<string>;
  buildSignedHeaders(ctx: PaymentContext, signature: string): Record<string, string>;
  readReceipt(response: Response): SignedReceipt | null;
};
