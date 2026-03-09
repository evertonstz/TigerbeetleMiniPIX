package clearing

import (
	"github.com/tigerbeetle/tigerbeetle-go/pkg/types"
)

// MockTigerBeetleClient implements TigerBeetleClientInterface for testing
type MockTigerBeetleClient struct {
	CreatedTransfers []types.Transfer
	PostedTransfers  []types.Transfer
	VoidedTransfers  []types.Transfer
	shouldFailCreate bool
	shouldFailPost   bool
	shouldFailVoid   bool
	failTransferIdx  uint32
}

func NewMockTigerBeetleClient() *MockTigerBeetleClient {
	return &MockTigerBeetleClient{
		CreatedTransfers: []types.Transfer{},
		PostedTransfers:  []types.Transfer{},
		VoidedTransfers:  []types.Transfer{},
	}
}

func (m *MockTigerBeetleClient) CreateTransfers(transfers []types.Transfer) []types.TransferEventResult {
	results := make([]types.TransferEventResult, len(transfers))
	for i, transfer := range transfers {
		m.CreatedTransfers = append(m.CreatedTransfers, transfer)
		result := types.TransferOK
		if m.shouldFailCreate && uint32(i) == m.failTransferIdx {
			result = types.TransferDebitAccountIDMustNotBeZero // Example error
		}
		results[i] = types.TransferEventResult{
			Index:  uint32(i),
			Result: result,
		}
	}
	return results
}

// PostTransfers simulates posting pending transfers (Phase 2 accept)
func (m *MockTigerBeetleClient) PostTransfers(transfers []types.Transfer) []types.TransferEventResult {
	results := make([]types.TransferEventResult, len(transfers))
	for i, transfer := range transfers {
		m.PostedTransfers = append(m.PostedTransfers, transfer)
		result := types.TransferOK
		if m.shouldFailPost && uint32(i) == m.failTransferIdx {
			result = types.TransferLinkedEventFailed
		}
		results[i] = types.TransferEventResult{
			Index:  uint32(i),
			Result: result,
		}
	}
	return results
}

// VoidTransfers simulates voiding pending transfers (Phase 2 reject)
func (m *MockTigerBeetleClient) VoidTransfers(transfers []types.Transfer) []types.TransferEventResult {
	results := make([]types.TransferEventResult, len(transfers))
	for i, transfer := range transfers {
		m.VoidedTransfers = append(m.VoidedTransfers, transfer)
		result := types.TransferOK
		if m.shouldFailVoid && uint32(i) == m.failTransferIdx {
			result = types.TransferLinkedEventFailed
		}
		results[i] = types.TransferEventResult{
			Index:  uint32(i),
			Result: result,
		}
	}
	return results
}

// SetFailure configures failure for create transfers at specific index
func (m *MockTigerBeetleClient) SetFailure(shouldFail bool, transferIdx uint32) {
	m.shouldFailCreate = shouldFail
	m.failTransferIdx = transferIdx
}

// setPostFailure configures failure for post operations
func (m *MockTigerBeetleClient) setPostFailure(transferIdx uint32) {
	m.shouldFailPost = true
	m.failTransferIdx = transferIdx
}

// setVoidFailure configures failure for void operations
func (m *MockTigerBeetleClient) setVoidFailure(transferIdx uint32) {
	m.shouldFailVoid = true
	m.failTransferIdx = transferIdx
}

// ClearFailure clears all failure flags
func (m *MockTigerBeetleClient) ClearFailure() {
	m.shouldFailCreate = false
	m.shouldFailPost = false
	m.shouldFailVoid = false
}
