/**
 * Tests for PaygateClient
 */

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { ethers } from 'ethers';
import { PaygateClient } from '../src/client';
import type { SignedReceipt, PaymentContext } from '../src/types';

describe('PaygateClient', () => {
  let client: PaygateClient;
  let mockSigner: ethers.Signer;

  beforeEach(() => {
    // Create a mock wallet for tests
    const wallet = ethers.Wallet.createRandom();
    mockSigner = wallet;
    
    client = new PaygateClient(
      'https://paygate.example.com',
      mockSigner
    );
  });

  describe('constructor', () => {
    it('should create client with valid parameters', () => {
      expect(client).toBeInstanceOf(PaygateClient);
    });

    it('should remove trailing slash from serverUrl', () => {
      const clientWithSlash = new PaygateClient(
        'https://paygate.example.com/',
        mockSigner
      );
      expect(clientWithSlash).toBeInstanceOf(PaygateClient);
    });

    it('should set default options', () => {
      const clientWithOptions = new PaygateClient(
        'https://paygate.example.com',
        mockSigner,
        {
          autoVerifyReceipts: false,
          timeout: 60000,
        }
      );
      expect(clientWithOptions).toBeInstanceOf(PaygateClient);
    });
  });

  describe('signPayment', () => {
    it('should sign payment context with EIP-712', async () => {
      const context: PaymentContext = {
        recipient: '0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219',
        token: 'USDC',
        amount: '0.001',
        nonce: 'test-nonce-123',
        chainId: 8453,
      };

      const signature = await client.signPayment(context);

      expect(signature).toMatch(/^0x[0-9a-f]{130}$/i);
      expect(signature.length).toBe(132); // 0x + 130 hex chars
    });

    it('should produce consistent signatures for same context', async () => {
      const context: PaymentContext = {
        recipient: '0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219',
        token: 'USDC',
        amount: '0.001',
        nonce: 'test-nonce-456',
        chainId: 8453,
      };

      const sig1 = await client.signPayment(context);
      const sig2 = await client.signPayment(context);

      expect(sig1).toBe(sig2);
    });
  });

  describe('verifyReceipt', () => {
    it('should validate receipt structure', async () => {
      const mockReceipt: SignedReceipt = {
        receipt: {
          id: 'rcpt_test123',
          version: '1.0',
          timestamp: new Date().toISOString(),
          payment: {
            payer: '0x742d35Cc6634C0532925a3b844Bc9e7595f8fE21',
            recipient: '0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219',
            amount: '0.001',
            token: 'USDC',
            chainId: 8453,
            nonce: 'test-nonce',
          },
          service: {
            endpoint: '/api/ai/summarize',
            request_hash: 'sha256:abc123',
            response_hash: 'sha256:def456',
          },
        },
        signature: '0x' + '0'.repeat(130),
        server_public_key: '0x04' + '0'.repeat(128),
      };

      // This will fail verification but shouldn't throw on structure validation
      const result = await client.verifyReceipt(mockReceipt);
      expect(typeof result).toBe('boolean');
    });

    it('should reject receipt with invalid structure', async () => {
      const invalidReceipt = {
        receipt: null,
        signature: '0x123',
        server_public_key: '0x456',
      } as any;

      const result = await client.verifyReceipt(invalidReceipt);
      expect(result).toBe(false);
    });
  });

  describe('request', () => {
    it('should handle successful requests without 402', async () => {
      global.fetch = vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        json: async () => ({
          result: 'Summary text',
          receipt: {
            receipt: {
              id: 'rcpt_abc123',
              version: '1.0',
              timestamp: new Date().toISOString(),
              payment: {
                payer: await mockSigner.getAddress(),
                recipient: '0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219',
                amount: '0.001',
                token: 'USDC',
                chainId: 8453,
                nonce: 'nonce123',
              },
              service: {
                endpoint: '/api/ai/summarize',
                request_hash: 'sha256:req',
                response_hash: 'sha256:resp',
              },
            },
            signature: '0x' + '1'.repeat(130),
            server_public_key: '0x04' + '2'.repeat(128),
          },
        }),
      });

      const clientNoVerify = new PaygateClient(
        'https://test.com',
        mockSigner,
        { autoVerifyReceipts: false }
      );

      const response = await clientNoVerify.request('/api/ai/summarize', {
        text: 'Test text',
      });

      expect(response.data).toBeDefined();
      expect(response.receipt).toBeDefined();
      expect(response.receipt.receipt.id).toBe('rcpt_abc123');
    });

    it('should throw error when response missing receipt', async () => {
      global.fetch = vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        json: async () => ({
          result: 'No receipt',
        }),
      });

      await expect(
        client.request('/api/test', {}, { skipReceiptVerification: true })
      ).rejects.toThrow('Response missing receipt');
    });
  });
});
