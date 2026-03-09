package account

// Account type codes (ledger classification)
// All accounts must use ledger=1 (single-ledger model)
const (
	CentralBankCode         = 1  // Central Bank (1 account)
	BankReserveCode         = 2  // Bank Reserve (5 accounts, one per participant bank)
	BankInternalTransitCode = 3  // Bank Internal Transit (5 accounts, one per participant bank)
	EndUserCode             = 10 // End-User accounts (5,000 accounts, 1,000 per bank)
)

// Ledger ID: All accounts in phase 1 must use the same ledger
const SingleLedger = 1

// TigerBeetle batch size limit
// TigerBeetle reserves 3 internal slots per call, max user payload is 8192 - 3 = 8189
const TigerBeetleBatchSize = 8189

// Redpanda configuration (verified, not created by seed)
const (
	RedpandaTopic          = "pix-payments"
	RedpandaPartitionCount = 3
	RedpandaRetentionHours = 24
)

// Broker addresses (from Phase 1 docker-compose, hardcoded for phase 1)
const (
	TigerBeetleAddresses = "127.0.0.1:3001,127.0.0.1:3002,127.0.0.1:3003,127.0.0.1:3004,127.0.0.1:3005"
	RedpandaBroker       = "127.0.0.1:9092"
)

// Initial balance amounts (Claude's discretion)
// Central Bank: Large balance to fund Bank Reserves and back all user balances
// Bank Reserve: Large balance per bank (will be checked but not debited in Phase 1)
// Internal Transit: Initialized at zero (used as relay in 3-legged chain)
// End-User: Initialized at zero (users can only receive, not overdraft)
const (
	CentralBankInitialBalance int64 = 10_000_000_000 // 10 billion units (arbitrary, sufficient for 10,000+ TPS load test)
	BankReserveInitialBalance int64 = 500_000_000    // 500 million per bank (sufficient for 1-2% of max throughput as reserve)
)

// Account counts (from REQUIREMENTS.md)
const (
	CentralBankCount         = 1
	BankReserveCount         = 5
	BankInternalTransitCount = 5
	EndUserCountPerBank      = 1000
	TotalEndUsers            = 5000
)
