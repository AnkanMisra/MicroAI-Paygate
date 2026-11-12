package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gateway/nanopay"

	"github.com/stretchr/testify/require"
)

const nanoTestBlock = "A1B2C3D4E5F60718293A4B5C6D7E8F90A1B2C3D4E5F60718293A4B5C6D7E8F90"

// enableNanoForTest points the gateway at a fake node that holds one send
// block, built by the test from the 402 offer.
func enableNanoForTest(t *testing.T) (payTo, payer string, blocks map[string]map[string]any) {
	t.Helper()
	payTo, _ = nanopay.EncodeAccount(bytes.Repeat([]byte{0x22}, 32))
	payer, _ = nanopay.EncodeAccount(bytes.Repeat([]byte{0x11}, 32))
	blocks = map[string]map[string]any{}
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		_ = json.NewDecoder(r.Body).Decode(&req)
		if b, ok := blocks[req["hash"]]; ok {
			_ = json.NewEncoder(w).Encode(b)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Block not found"})
	}))
	t.Cleanup(node.Close)

	t.Setenv("NANO_PAY_TO", payTo)
	t.Setenv("NANO_PRICE_XNO", "0.001")
	t.Setenv("NANO_RPC_URL", node.URL)
	t.Setenv("NANO_CLAIMS_FILE", filepath.Join(t.TempDir(), "nano-claims"))
	require.NoError(t, initNanoPayment())
	t.Cleanup(func() { nanoVerifier = nil })
	return payTo, payer, blocks
}

func TestNanoOffIsUnchanged(t *testing.T) {
	nanoVerifier = nil
	t.Setenv("NANO_PAY_TO", "")
	require.NoError(t, initNanoPayment())
	require.Nil(t, nanoVerifier)
	require.Empty(t, nanoCORSHeaders())

	req := httptest.NewRequest(http.MethodPost, "/api/ai/summarize", strings.NewReader(`{"text":"hello"}`))
	req.Header.Set("X-402-Nano-Block", nanoTestBlock)
	req.Header.Set("X-402-Nano-Order", strings.Repeat("a", 32))
	c, recorder := newPaymentFlowTestContext(req)
	require.False(t, hasPaymentHeaders(c))

	_, ok := verifyPaidRequest(c)
	require.False(t, ok)
	require.Equal(t, http.StatusPaymentRequired, recorder.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.NotContains(t, body, "nano")
}

func TestNanoConfigRequiresEveryVariable(t *testing.T) {
	nanoVerifier = nil
	payTo, _ := nanopay.EncodeAccount(bytes.Repeat([]byte{0x22}, 32))
	t.Setenv("NANO_PAY_TO", payTo)
	t.Setenv("NANO_PRICE_XNO", "")
	t.Setenv("NANO_RPC_URL", "https://node.example")
	t.Setenv("NANO_CLAIMS_FILE", filepath.Join(t.TempDir(), "c"))
	require.Error(t, initNanoPayment())
	require.Nil(t, nanoVerifier)
}

func TestNanoChallengeAndPaidRequest(t *testing.T) {
	payTo, payer, blocks := enableNanoForTest(t)
	require.ElementsMatch(t, []string{"X-402-Nano-Block", "X-402-Nano-Order"}, nanoCORSHeaders())

	// 1. Unpaid request: the 402 carries the EIP-712 context and a Nano order.
	req := httptest.NewRequest(http.MethodPost, "/api/ai/summarize", strings.NewReader(`{"text":"hello"}`))
	c, recorder := newPaymentFlowTestContext(req)
	_, ok := verifyPaidRequest(c)
	require.False(t, ok)
	require.Equal(t, http.StatusPaymentRequired, recorder.Code)
	var challenge struct {
		PaymentContext PaymentContext `json:"paymentContext"`
		Nano           nanopay.Offer  `json:"nano"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &challenge))
	require.NotEmpty(t, challenge.PaymentContext.Nonce)
	offer := challenge.Nano
	require.Equal(t, "nano:mainnet", offer.Network)
	require.Equal(t, "exact", offer.Scheme)
	require.Equal(t, payTo, offer.PayTo)
	require.Equal(t, "1000000000000000000000000000", offer.Amount)

	// 2. The payer sends exactly that amount with the order commitment as
	// representative; the node reports it confirmed.
	blocks[nanoTestBlock] = map[string]any{
		"block_account":   payer,
		"amount":          offer.Amount,
		"confirmed":       "true",
		"subtype":         "send",
		"local_timestamp": strconv.FormatInt(time.Now().Unix(), 10),
		"contents": map[string]string{
			"type":            "state",
			"representative":  offer.Representative,
			"link_as_account": payTo,
		},
	}
	paid := func() (*verifiedPayment, bool, *httptest.ResponseRecorder) {
		req := httptest.NewRequest(http.MethodPost, "/api/ai/summarize", strings.NewReader(`{"text":"hello"}`))
		req.Header.Set("X-402-Nano-Block", nanoTestBlock)
		req.Header.Set("X-402-Nano-Order", offer.OrderID)
		c, recorder := newPaymentFlowTestContext(req)
		require.True(t, hasPaymentHeaders(c))
		v, ok := verifyPaidRequest(c)
		return v, ok, recorder
	}

	verified, ok, _ := paid()
	require.True(t, ok)
	require.Equal(t, payer, verified.RecoveredAddress)
	require.Equal(t, "XNO", verified.PaymentContext.Token)
	require.Equal(t, offer.Amount, verified.PaymentContext.Amount)
	require.Equal(t, payTo, verified.PaymentContext.Recipient)
	require.Equal(t, nanoTestBlock, verified.PaymentContext.Nonce)

	// 3. The same block and order again: refused as a replay.
	_, ok, recorder = paid()
	require.False(t, ok)
	require.Equal(t, http.StatusConflict, recorder.Code)
}
