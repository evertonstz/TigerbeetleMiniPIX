package account

import (
	"github.com/tigerbeetle/tigerbeetle-go/pkg/types"
)

// NewCentralBankAccount creates the single Central Bank account
// Code: 1, Ledger: 1, no flags, large initial balance
func NewCentralBankAccount() types.Account {
	return types.Account{
		ID:        types.ToUint128(1),
		Ledger:    SingleLedger,
		Code:      CentralBankCode,
		Flags:     0,
		Timestamp: 0,
	}
}

// NewBankReserveAccounts creates 5 Bank Reserve accounts (one per bank)
// Code: 2, Ledger: 1, no flags, large initial balance per account
// Bank indices: 1-5 (for 5 participant banks)
func NewBankReserveAccounts() []types.Account {
	accounts := make([]types.Account, BankReserveCount)
	for i := 1; i <= BankReserveCount; i++ {
		accounts[i-1] = types.Account{
			ID:        types.ToUint128(uint64(10000 + i)),
			Ledger:    SingleLedger,
			Code:      BankReserveCode,
			Flags:     0,
			Timestamp: 0,
		}
	}
	return accounts
}

// NewBankInternalTransitAccounts creates 5 Bank Internal Transit accounts (one per bank)
// Code: 3, Ledger: 1, no flags, zero initial balance
// These act as relay accounts in the 3-legged transfer chain
// Bank indices: 1-5
func NewBankInternalTransitAccounts() []types.Account {
	accounts := make([]types.Account, BankInternalTransitCount)
	for i := 1; i <= BankInternalTransitCount; i++ {
		accounts[i-1] = types.Account{
			ID:        types.ToUint128(uint64(20000 + i)),
			Ledger:    SingleLedger,
			Code:      BankInternalTransitCode,
			Flags:     0,
			Timestamp: 0,
		}
	}
	return accounts
}

// NewEndUserAccounts creates TotalEndUsers (5,000) end-user accounts
// Code: 10, Ledger: 1, debits_must_not_exceed_credits flag set (prevents overdraft)
// Distributed across 5 banks: 1,000 per bank
// Account IDs: 100001-105000 (10 per user ID range, user 1 → ID 100001, user 2 → ID 100002, etc.)
func NewEndUserAccounts() []types.Account {
	accounts := make([]types.Account, TotalEndUsers)

	for userID := 0; userID < TotalEndUsers; userID++ {
		accountID := uint64(100000 + userID + 1)

		accounts[userID] = types.Account{
			ID:        types.ToUint128(accountID),
			Ledger:    SingleLedger,
			Code:      EndUserCode,
			Flags:     types.AccountFlags{DebitsMustNotExceedCredits: true}.ToUint16(),
			Timestamp: 0,
		}
	}

	return accounts
}
