package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"gateway/nanopay"

	"github.com/gin-gonic/gin"
)

// Optional Nano (nano:mainnet, scheme exact) payments beside the EIP-712 flow.
// Off unless NANO_PAY_TO is set; with it unset the gateway behaves exactly as
// before. The verification logic lives in the stdlib-only gateway/nanopay
// package; this file only wires it to the gateway's headers and receipts.

const (
	nanoBlockHeader = "X-402-Nano-Block"
	nanoOrderHeader = "X-402-Nano-Order"
)

var nanoVerifier *nanopay.Verifier

// initNanoPayment reads NANO_PAY_TO, NANO_PRICE_XNO, NANO_RPC_URL and
// NANO_CLAIMS_FILE. All four are required once NANO_PAY_TO is set.
func initNanoPayment() error {
	payTo := os.Getenv("NANO_PAY_TO")
	if payTo == "" {
		return nil
	}
	var missing []string
	for _, key := range []string{"NANO_PRICE_XNO", "NANO_RPC_URL", "NANO_CLAIMS_FILE"} {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("NANO_PAY_TO is set; missing %v", missing)
	}
	price, err := nanopay.RawFromXNO(os.Getenv("NANO_PRICE_XNO"))
	if err != nil {
		return fmt.Errorf("NANO_PRICE_XNO: %w", err)
	}
	claims, err := nanopay.OpenFileClaimStore(os.Getenv("NANO_CLAIMS_FILE"))
	if err != nil {
		return fmt.Errorf("NANO_CLAIMS_FILE: %w", err)
	}
	v, err := nanopay.NewVerifier(os.Getenv("NANO_RPC_URL"), payTo, price, claims)
	if err != nil {
		claims.Close()
		return err
	}
	v.Client = &http.Client{Timeout: getVerifierTimeout()}
	nanoVerifier = v
	log.Printf("Nano payments enabled: network=%s pay_to=%s price_raw=%s", nanopay.Network, payTo, price.String())
	return nil
}

func nanoCORSHeaders() []string {
	if nanoVerifier == nil {
		return nil
	}
	return []string{nanoBlockHeader, nanoOrderHeader}
}

func hasNanoPaymentHeaders(c *gin.Context) bool {
	return nanoVerifier != nil && c.GetHeader(nanoBlockHeader) != "" && c.GetHeader(nanoOrderHeader) != ""
}

// nanoChallengeOffer returns a fresh order for the 402 body, or nil when Nano
// is off.
func nanoChallengeOffer() *nanopay.Offer {
	if nanoVerifier == nil {
		return nil
	}
	orderID, err := nanopay.NewOrderID()
	if err != nil {
		log.Printf("[ERROR] nano order id: %v", err)
		return nil
	}
	offer := nanoVerifier.Offer(orderID)
	return &offer
}

func verifyNanoPaidRequest(c *gin.Context) (*verifiedPayment, bool) {
	payment, err := nanoVerifier.Verify(c.Request.Context(), c.GetHeader(nanoOrderHeader), c.GetHeader(nanoBlockHeader))
	if err != nil {
		code, msg := nanoFailureResponse(c.Request.Context(), err)
		label := "invalid"
		if code >= 500 {
			label = "error"
		}
		verificationTotal.WithLabelValues(label).Inc()
		respondError(c, code, msg, err)
		return nil, false
	}
	verificationTotal.WithLabelValues("success").Inc()
	return &verifiedPayment{
		PaymentContext: PaymentContext{
			Recipient: payment.PayTo,
			Token:     nanopay.Asset,
			Amount:    payment.AmountRaw,
			Nonce:     payment.BlockHash,
			Timestamp: uint64(time.Now().Unix()),
		},
		RecoveredAddress: payment.Payer,
	}, true
}

func nanoFailureResponse(ctx context.Context, err error) (int, string) {
	switch {
	case errors.Is(err, nanopay.ErrAlreadyClaimed):
		return http.StatusConflict, "nonce_already_used"
	case errors.Is(err, nanopay.ErrInvalidOrderID), errors.Is(err, nanopay.ErrInvalidBlockID):
		return http.StatusBadRequest, "invalid_nano_payment"
	case errors.Is(err, nanopay.ErrNotConfirmed), errors.Is(err, nanopay.ErrWrongPayTo),
		errors.Is(err, nanopay.ErrWrongAmount), errors.Is(err, nanopay.ErrNotBound),
		errors.Is(err, nanopay.ErrTooOld):
		return http.StatusPaymentRequired, "invalid_nano_payment"
	case errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil:
		return http.StatusGatewayTimeout, "verifier_timeout"
	default:
		return http.StatusBadGateway, "verification_unavailable"
	}
}
