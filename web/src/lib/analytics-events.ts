export const AnalyticsEvent = {
  SamplePromptLoaded: "sample prompt loaded",
  SummaryRequested: "summary requested",
  PaymentChallengeReceived: "payment challenge received",
  WalletConnectRequested: "wallet connect requested",
  WalletConnectSucceeded: "wallet connect succeeded",
  WalletConnectFailed: "wallet connect failed",
  ChainSwitchRequested: "chain switch requested",
  ChainSwitchSucceeded: "chain switch succeeded",
  ChainSwitchFailed: "chain switch failed",
  PaymentSignatureStarted: "payment signature started",
  PaymentSignatureSucceeded: "payment signature succeeded",
  PaymentSignatureFailed: "payment signature failed",
  SignedRetrySent: "signed retry sent",
  SummaryCompleted: "summary completed",
  SummaryFailed: "summary failed",
  ReceiptHistoryViewed: "receipt history viewed",
  SummaryCopied: "summary copied",
  ReceiptIdCopied: "receipt id copied",
} as const;

export type AnalyticsEventName = (typeof AnalyticsEvent)[keyof typeof AnalyticsEvent];
