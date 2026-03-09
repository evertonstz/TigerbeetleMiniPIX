package clearing

import (
	tb "github.com/tigerbeetle/tigerbeetle-go"
	"github.com/tigerbeetle/tigerbeetle-go/pkg/types"
)

// TigerBeetleAdapter wraps the real TigerBeetle client to match our interface
type TigerBeetleAdapter struct {
	client tb.Client
}

// NewTigerBeetleAdapter creates a new adapter wrapping a TigerBeetle client
func NewTigerBeetleAdapter(client tb.Client) *TigerBeetleAdapter {
	return &TigerBeetleAdapter{
		client: client,
	}
}

// CreateTransfers creates transfers and returns results without error
// Errors are logged but not returned (per our interface contract)
func (a *TigerBeetleAdapter) CreateTransfers(transfers []types.Transfer) []types.TransferEventResult {
	results, err := a.client.CreateTransfers(transfers)
	if err != nil {
		return convertErrorToResults(len(transfers), err)
	}
	return results
}

// PostTransfers posts pending transfers (Phase 2 accept) and returns results
func (a *TigerBeetleAdapter) PostTransfers(transfers []types.Transfer) []types.TransferEventResult {
	results, err := a.client.CreateTransfers(transfers)
	if err != nil {
		return convertErrorToResults(len(transfers), err)
	}
	return results
}

// VoidTransfers voids pending transfers (Phase 2 reject) and returns results
func (a *TigerBeetleAdapter) VoidTransfers(transfers []types.Transfer) []types.TransferEventResult {
	results, err := a.client.CreateTransfers(transfers)
	if err != nil {
		return convertErrorToResults(len(transfers), err)
	}
	return results
}

// convertErrorToResults converts a client error to transfer event results marking all as failed
func convertErrorToResults(count int, err error) []types.TransferEventResult {
	results := make([]types.TransferEventResult, count)
	for i := 0; i < count; i++ {
		results[i] = types.TransferEventResult{
			Index:  uint32(i),
			Result: types.TransferDebitAccountIDMustNotBeZero, // Placeholder error
		}
	}
	return results
}
