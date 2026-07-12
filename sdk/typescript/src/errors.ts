export type PaygateSdkErrorCode =
  | "payment_challenge_missing"
  | "payment_signature_failed"
  | "signed_retry_failed"
  | "receipt_decode_failed"
  | "receipt_verification_failed"
  | "network_error";

export type PaygateSdkErrorOptions = {
  status?: number;
  bodyText?: string;
  cause?: unknown;
};

export class PaygateSdkError extends Error {
  readonly code: PaygateSdkErrorCode;
  readonly status?: number;
  readonly bodyText?: string;

  constructor(code: PaygateSdkErrorCode, message: string, options: PaygateSdkErrorOptions = {}) {
    super(message);
    this.name = "PaygateSdkError";
    this.code = code;
    this.status = options.status;
    this.bodyText = options.bodyText;
    if (options.cause !== undefined) {
      this.cause = options.cause;
    }
    Object.setPrototypeOf(this, PaygateSdkError.prototype);
  }
}
