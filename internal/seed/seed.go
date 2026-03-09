package seed

import (
	"context"
	"fmt"
	"log"

	tigerbeetle_go "github.com/tigerbeetle/tigerbeetle-go"
	"github.com/tigerbeetle/tigerbeetle-go/pkg/types"

	"github.com/evertoncorreia/tigerbeetle-minipix/internal/account"
)

// QueryExistingAccounts retrieves all accounts that already exist in TigerBeetle.
// Returns a set (map) of account IDs that are already created.
// This is the first step in query-first idempotency.
func QueryExistingAccounts(ctx context.Context, client tigerbeetle_go.Client) (map[string]bool, error) {
	targetIDs := buildTargetAccountIDs()

	log.Printf("Querying %d target accounts for existing state...", len(targetIDs))

	resultRaw, err := RetryTransient(ctx, func() (interface{}, error) {
		return client.LookupAccounts(targetIDs)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query existing accounts: %w", err)
	}

	existingAccounts := resultRaw.([]types.Account)

	existingIDsMap := make(map[string]bool)
	for _, acc := range existingAccounts {
		existingIDsMap[acc.ID.String()] = true
	}

	log.Printf("✓ Found %d existing accounts", len(existingIDsMap))
	return existingIDsMap, nil
}

// CreateAccountsByType creates accounts of a single type (CB, Reserves, Internal, End-Users).
// Batches accounts respecting TigerBeetleBatchSize limit.
// Treats CreateAccountResult.Exists as success (idempotency).
// Returns error only on permanent failures (invalid params, constraint violation).
func CreateAccountsByType(
	ctx context.Context,
	client tigerbeetle_go.Client,
	accountType string,
	accounts []types.Account,
	existingIDs map[string]bool,
) error {
	var accountsToCreate []types.Account
	for _, acc := range accounts {
		if !existingIDs[acc.ID.String()] {
			accountsToCreate = append(accountsToCreate, acc)
		}
	}

	if len(accountsToCreate) == 0 {
		log.Printf("✓ %s: All %d accounts already exist, skipping creation", accountType, len(accounts))
		return nil
	}

	log.Printf("Creating %s: %d accounts (%d already exist, %d new)",
		accountType, len(accounts), len(accounts)-len(accountsToCreate), len(accountsToCreate))

	const BATCH_SIZE = account.TigerBeetleBatchSize
	totalCreated := 0

	for i := 0; i < len(accountsToCreate); i += BATCH_SIZE {
		end := i + BATCH_SIZE
		if end > len(accountsToCreate) {
			end = len(accountsToCreate)
		}

		batch := accountsToCreate[i:end]
		batchNum := (i / BATCH_SIZE) + 1
		totalBatches := (len(accountsToCreate) + BATCH_SIZE - 1) / BATCH_SIZE

		log.Printf("  Batch %d/%d: Creating %d accounts...", batchNum, totalBatches, len(batch))

		resultRaw, err := RetryTransient(ctx, func() (interface{}, error) {
			return client.CreateAccounts(batch)
		})
		if err != nil {
			return fmt.Errorf("failed to create %s batch %d: %w", accountType, batchNum, err)
		}

		accountErrors := resultRaw.([]types.AccountEventResult)

		for _, ae := range accountErrors {
			if ae.Result != types.AccountOK && ae.Result != types.AccountExists {
				return fmt.Errorf("permanent error creating %s batch %d, account at index %d: %v",
					accountType, batchNum, ae.Index, ae.Result)
			}
			if ae.Result == types.AccountExists {
				log.Printf("    Account at index %d already exists (race condition), continuing", ae.Index)
			}
		}

		totalCreated += len(batch)
		log.Printf("  ✓ Batch %d/%d complete", batchNum, totalBatches)
	}

	log.Printf("✓ %s: Created %d new accounts", accountType, totalCreated)
	return nil
}

// SeedAll runs the complete bootstrap sequence:
// 1. Query existing accounts
// 2. Create Central Bank (1 account)
// 3. Create Bank Reserves (5 accounts)
// 4. Create Bank Internal Transit (5 accounts)
// 5. Create End-Users (5,000 accounts)
//
// Safe to re-run: existing accounts are skipped, missing accounts are created.
// Returns error if any permanent failure occurs; logs all progress.
func SeedAll(ctx context.Context, client tigerbeetle_go.Client) error {
	log.Println("Starting account bootstrap...")

	existingIDs, err := QueryExistingAccounts(ctx, client)
	if err != nil {
		return err
	}

	accountTypes := []struct {
		name     string
		accounts []types.Account
	}{
		{"Central Bank", []types.Account{account.NewCentralBankAccount()}},
		{"Bank Reserves", account.NewBankReserveAccounts()},
		{"Bank Internal Transit", account.NewBankInternalTransitAccounts()},
		{"End-Users", account.NewEndUserAccounts()},
	}

	for _, t := range accountTypes {
		if err := CreateAccountsByType(ctx, client, t.name, t.accounts, existingIDs); err != nil {
			return err
		}
	}

	log.Println("✓ Account bootstrap complete")
	return nil
}

// Helper: Build list of all target account IDs (used by QueryExistingAccounts)
func buildTargetAccountIDs() []types.Uint128 {
	var ids []types.Uint128

	ids = append(ids, types.ToUint128(1))

	for i := 1; i <= account.BankReserveCount; i++ {
		ids = append(ids, types.ToUint128(uint64(10000+i)))
	}

	for i := 1; i <= account.BankInternalTransitCount; i++ {
		ids = append(ids, types.ToUint128(uint64(20000+i)))
	}

	for userID := 0; userID < account.TotalEndUsers; userID++ {
		ids = append(ids, types.ToUint128(uint64(100000+userID+1)))
	}

	return ids
}
