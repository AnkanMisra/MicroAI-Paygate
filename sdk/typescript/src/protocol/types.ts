import type { TypedDataDomain, TypedDataField } from "ethers";

export type PaymentContextV1 = {
  authorizationVersion?: never;
  recipient: string;
  token: string;
  amount: string;
  nonce: string;
  chainId: number;
  timestamp: number;
};

export type PaymentContextV2 = {
  authorizationVersion: 2;
  recipient: string;
  token: string;
  amount: string;
  nonce: string;
  chainId: number;
  timestamp: number;
  audience: string;
  method: string;
  resource: string;
  contentType: string;
  requestHash: string;
};

export type PaymentContext = PaymentContextV1 | PaymentContextV2;

export type PaymentRequestBinding = {
  url: string;
  method: string;
  contentType: string;
  bodyText?: string;
};

export type PaymentSigner = {
  getAddress?(): Promise<string>;
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
  authorization_version?: number;
  audience?: string;
  method?: string;
  resource?: string;
  content_type?: string;
  authorization_request_hash?: string;
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
  validatePaymentContext(ctx: PaymentContext, request: PaymentRequestBinding): void;
  getPayer?(signer: PaymentSigner, ctx: PaymentContext): Promise<string | undefined>;
  signPaymentContext(
    signer: PaymentSigner,
    ctx: PaymentContext,
    payer?: string,
  ): Promise<string>;
  buildSignedHeaders(
    ctx: PaymentContext,
    signature: string,
    payer?: string,
  ): Record<string, string>;
  readReceipt(response: Response): SignedReceipt | null;
};
