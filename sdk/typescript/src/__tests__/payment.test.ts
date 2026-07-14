import { describe, expect, it } from "bun:test";
import { ethers } from "ethers";
import authorizationV2Fixture from "../../../../tests/fixtures/payment-authorization-v2.json";
import {
  buildPaymentTypedData,
  buildSignedHeaders,
  signPaymentContext,
  type PaymentContext,
  type PaymentContextV2,
} from "../index";

const wallet = new ethers.Wallet(
  "0x0123456789012345678901234567890123456789012345678901234567890123",
);

const paymentContext: PaymentContext = {
  recipient: "0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219",
  token: "USDC",
  amount: "0.001",
  nonce: "sdk-test-nonce",
  chainId: 84532,
  timestamp: 1766611200,
};

const expectedDomain = {
  name: "MicroAI Paygate",
  version: "1",
  chainId: paymentContext.chainId,
  verifyingContract: ethers.ZeroAddress,
};

const expectedTypes = {
  Payment: [
    { name: "recipient", type: "address" },
    { name: "token", type: "string" },
    { name: "amount", type: "string" },
    { name: "nonce", type: "string" },
    { name: "timestamp", type: "uint256" },
  ],
};

describe("payment helpers", () => {
  it("signPaymentContext produces a signature ethers verifies for the expected wallet", async () => {
    const signature = await signPaymentContext(wallet, paymentContext);

    const recovered = ethers.verifyTypedData(
      expectedDomain,
      expectedTypes,
      {
        recipient: paymentContext.recipient,
        token: paymentContext.token,
        amount: paymentContext.amount,
        nonce: paymentContext.nonce,
        timestamp: paymentContext.timestamp,
      },
      signature,
    );

    expect(recovered).toBe(wallet.address);
  });

  it("matches the shared request-bound v2 typed-data fixture", async () => {
    const context = authorizationV2Fixture.context as PaymentContextV2;
    const typedData = buildPaymentTypedData(context, authorizationV2Fixture.payer);

    expect(ethers.TypedDataEncoder.hash(typedData.domain, typedData.types, typedData.value)).toBe(
      authorizationV2Fixture.expectedTypedDataDigest,
    );
    expect(
      ethers.verifyTypedData(
        typedData.domain,
        typedData.types,
        typedData.value,
        authorizationV2Fixture.expectedSignature,
      ),
    ).toBe(authorizationV2Fixture.expectedSigner);
    expect(await signPaymentContext(wallet, context, authorizationV2Fixture.payer)).toBe(
      authorizationV2Fixture.expectedSignature,
    );
    expect(
      buildSignedHeaders(
        context,
        authorizationV2Fixture.expectedSignature,
        authorizationV2Fixture.payer,
      ),
    ).toMatchObject({ "X-402-Payer": authorizationV2Fixture.expectedSigner });
  });

  it("buildSignedHeaders emits exactly the current MicroAI X-402 retry headers", () => {
    const headers = buildSignedHeaders(paymentContext, "0xsigned");

    expect(headers).toEqual({
      "X-402-Signature": "0xsigned",
      "X-402-Nonce": "sdk-test-nonce",
      "X-402-Timestamp": "1766611200",
    });
    expect(Object.keys(headers).sort()).toEqual([
      "X-402-Nonce",
      "X-402-Signature",
      "X-402-Timestamp",
    ]);
  });
});
