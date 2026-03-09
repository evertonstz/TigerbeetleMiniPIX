package clearing

import (
	"context"
	"fmt"
	"github.com/tigerbeetle/tigerbeetle-go/pkg/types"
	"log"
)

type Phase2Result struct {
	Success     bool
	ErrorMsg    string
	ShouldRetry bool // false = terminal, no retry
}

type Phase2Executor struct {
	tbClient TigerBeetleClientInterface
}

func NewPhase2Executor(tbClient TigerBeetleClientInterface) *Phase2Executor {
	return &Phase2Executor{
		tbClient: tbClient,
	}
}

// Execute posts or voids pending transfers based on bankBAccepted
// If bankBAccepted is true, post the transfers
// If bankBAccepted is false, void the transfers
func (e *Phase2Executor) Execute(ctx context.Context, chain *TransferChain, bankBAccepted bool) (*Phase2Result, error) {
	if e.tbClient == nil {
		return &Phase2Result{
			Success:     false,
			ErrorMsg:    "TigerBeetle client not initialized",
			ShouldRetry: false,
		}, fmt.Errorf("TB client nil")
	}

	if chain == nil {
		return &Phase2Result{
			Success:     false,
			ErrorMsg:    "Transfer chain is nil",
			ShouldRetry: false,
		}, fmt.Errorf("chain nil")
	}

	transfers := []types.Transfer{
		chain.Transfers[0],
		chain.Transfers[1],
		chain.Transfers[2],
	}

	var results []types.TransferEventResult

	if bankBAccepted {
		log.Printf("Phase 2: Posting transfers for payment %s (Bank B accepted)", chain.PaymentUUID)
		results = e.tbClient.PostTransfers(transfers)
	} else {
		log.Printf("Phase 2: Voiding transfers for payment %s (Bank B rejected)", chain.PaymentUUID)
		results = e.tbClient.VoidTransfers(transfers)
	}

	for _, result := range results {
		if result.Result != types.TransferOK {
			operation := "post"
			if !bankBAccepted {
				operation = "void"
			}
			log.Printf("Phase 2 error while %sing transfer %d: %v", operation, result.Index, result.Result)
			return &Phase2Result{
				Success:     false,
				ErrorMsg:    fmt.Sprintf("Transfer %d %s failed with result %d", result.Index, operation, result.Result),
				ShouldRetry: false,
			}, nil
		}
	}

	log.Printf("Phase 2 completed successfully for payment %s", chain.PaymentUUID)
	return &Phase2Result{Success: true, ShouldRetry: false}, nil
}
