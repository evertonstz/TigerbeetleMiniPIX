package clearing

import (
	"context"
	"fmt"
	"github.com/tigerbeetle/tigerbeetle-go/pkg/types"
	"log"
)

const MaxRetries = 3

type Phase1Result struct {
	Success  bool
	ErrorMsg string
}

type Phase1Executor struct {
	tbClient TigerBeetleClientInterface
}

// TigerBeetleClientInterface defines the interface for TigerBeetle client operations
type TigerBeetleClientInterface interface {
	CreateTransfers(transfers []types.Transfer) []types.TransferEventResult
	PostTransfers(transfers []types.Transfer) []types.TransferEventResult
	VoidTransfers(transfers []types.Transfer) []types.TransferEventResult
}

func NewPhase1Executor(tbClient TigerBeetleClientInterface) *Phase1Executor {
	return &Phase1Executor{
		tbClient: tbClient,
	}
}

// ErrorClassification documents how Phase 1 errors should be handled
// Terminal errors: no retry, payment fails permanently
//   - TransferLinkedEventChainOpen: linked flag on non-terminal transfer (code bug)
//   - TransferPendingTransferExpired: transfer already expired (pre-expired)
//   - TransferIdAlreadyFailed: same ID already failed (retry with new ID)
//   - TransferInsufficientFunds: debit account lacks balance (payment amount too large)
//
// Retryable errors: safe to retry with exponential backoff
//   - TransferNetworkTimeout: cluster consensus timeout
//   - Network I/O errors: transient Kafka/TigerBeetle connection issues
//
// Idempotency: Retries use SAME transfer IDs (SHA-256 based on paymentUUID + legIndex)
// If TigerBeetle rejects with "id_already_failed", generate NEW ID with retry suffix
// (This is handled by phase1_retry_test.go test cases)

func (e *Phase1Executor) Execute(ctx context.Context, chain *TransferChain) (*Phase1Result, error) {
	if e.tbClient == nil {
		return &Phase1Result{
			Success:  false,
			ErrorMsg: "TigerBeetle client not initialized",
		}, fmt.Errorf("TB client nil")
	}

	transfers := []types.Transfer{
		chain.Transfers[0],
		chain.Transfers[1],
		chain.Transfers[2],
	}

	results := e.tbClient.CreateTransfers(transfers)

	for _, result := range results {
		if result.Result != types.TransferOK {
			log.Printf("Phase 1 error for transfer %d: %v", result.Index, result.Result)
			return &Phase1Result{
				Success:  false,
				ErrorMsg: fmt.Sprintf("Transfer %d rejected with result %d", result.Index, result.Result),
			}, nil
		}
	}

	log.Printf("Phase 1 executed successfully for payment %s", chain.PaymentUUID)
	return &Phase1Result{Success: true}, nil
}

// ExecuteWithRetry executes Phase 1 with retry tracking for transient failures
func (e *Phase1Executor) ExecuteWithRetry(ctx context.Context, chain *TransferChain, retryTracker *RetryTracker) (*Phase1Result, error) {
	retryCount := retryTracker.GetRetryCount(chain.PaymentUUID)

	if retryCount >= MaxRetries {
		return &Phase1Result{
			Success:  false,
			ErrorMsg: fmt.Sprintf("max retries exceeded (%d attempts)", MaxRetries),
		}, fmt.Errorf("max retries exceeded for payment %s", chain.PaymentUUID)
	}

	result, err := e.Execute(ctx, chain)
	if err != nil {
		return result, err
	}

	if !result.Success {
		log.Printf("Phase 1 failed for payment %s: %s", chain.PaymentUUID, result.ErrorMsg)
		return result, nil
	}

	if retryCount > 0 {
		log.Printf("Phase 1 succeeded after %d retry(ies)", retryCount)
		retryTracker.ResetRetryCount(chain.PaymentUUID)
	}

	return result, nil
}
