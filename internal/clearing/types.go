package clearing

import (
	"encoding/json"
	"github.com/tigerbeetle/tigerbeetle-go/pkg/types"
)

type PaymentMessage struct {
	PaymentUUID        string `json:"payment_uuid"`
	AmountCents        int64  `json:"amount_cents"`
	SenderAccountID    uint64 `json:"sender_account_id"`
	RecipientAccountID uint64 `json:"recipient_account_id"`
	PixKey             string `json:"pix_key"`
}

func DeserializePayment(rawData []byte) (*PaymentMessage, error) {
	var payment PaymentMessage
	err := json.Unmarshal(rawData, &payment)
	if err != nil {
		return nil, err
	}
	return &payment, nil
}

// TransferChain represents the 3-leg atomic settlement structure
type TransferChain struct {
	PaymentUUID string
	Transfers   [3]types.Transfer
}

// NewTransferChain constructs a 3-leg linked transfer chain
// Leg 1: sender debit → internal reserve (linked)
// Leg 2: internal reserve → recipient credit (linked)
// Leg 3: internal reserve → main bank reserve (terminal, NOT linked)
func NewTransferChain(
	paymentUUID string,
	senderAccountID uint64,
	recipientAccountID uint64,
	amountCents int64,
) *TransferChain {
	leg1ID := GenerateTransferID(paymentUUID, 1, "phase1")
	leg2ID := GenerateTransferID(paymentUUID, 2, "phase1")
	leg3ID := GenerateTransferID(paymentUUID, 3, "phase1")

	leg1 := types.Transfer{
		ID:              types.ToUint128(leg1ID),
		DebitAccountID:  types.ToUint128(senderAccountID),
		CreditAccountID: types.ToUint128(3),
		Amount:          types.ToUint128(uint64(amountCents)),
		Ledger:          1,
		Code:            1,
		Timeout:         30,
		Flags:           0x1 | 0x2,
		Timestamp:       0,
	}

	leg2 := types.Transfer{
		ID:              types.ToUint128(leg2ID),
		DebitAccountID:  types.ToUint128(3),
		CreditAccountID: types.ToUint128(recipientAccountID),
		Amount:          types.ToUint128(uint64(amountCents)),
		Ledger:          1,
		Code:            1,
		Timeout:         30,
		PendingID:       types.ToUint128(leg1ID),
		Flags:           0x1 | 0x2,
		Timestamp:       0,
	}

	leg3 := types.Transfer{
		ID:              types.ToUint128(leg3ID),
		DebitAccountID:  types.ToUint128(3),
		CreditAccountID: types.ToUint128(2),
		Amount:          types.Uint128{},
		Ledger:          1,
		Code:            1,
		Timeout:         30,
		PendingID:       types.ToUint128(leg2ID),
		Flags:           0x2,
		Timestamp:       0,
	}

	return &TransferChain{
		PaymentUUID: paymentUUID,
		Transfers:   [3]types.Transfer{leg1, leg2, leg3},
	}
}

// NewTransferChainWithTimeout is like NewTransferChain but accepts custom timeout
func NewTransferChainWithTimeout(
	paymentUUID string,
	senderAccountID uint64,
	recipientAccountID uint64,
	amountCents int64,
	timeoutSeconds uint32,
) *TransferChain {
	chain := NewTransferChain(paymentUUID, senderAccountID, recipientAccountID, amountCents)

	for i := range chain.Transfers {
		chain.Transfers[i].Timeout = timeoutSeconds
	}

	return chain
}
