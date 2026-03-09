# TigerbeetleMiniPIX: Key Findings

## Why TigerBeetle Made This System Simple

Building a 2-phase commit settlement engine from scratch is hard. PostgreSQL or MongoDB could have worked. TigerBeetle was chosen instead. Here's why that matters.

---

## 1. Atomicity Without Complexity

Try building atomic 3-leg settlement with SQL.

```sql
BEGIN TRANSACTION;
  -- Leg 1: User debited
  UPDATE accounts SET balance = balance - 100 WHERE id = alice;
  INSERT INTO transfers VALUES (...);
  
  -- Leg 2: Interbank movement
  UPDATE accounts SET balance = balance - 100 WHERE id = bank_a_reserve;
  UPDATE accounts SET balance = balance + 100 WHERE id = bank_b_reserve;
  INSERT INTO transfers VALUES (...);
  
  -- Leg 3: User credited
  UPDATE accounts SET balance = balance + 100 WHERE id = bob;
  INSERT INTO transfers VALUES (...);
COMMIT;
```

Now the questions start:
- What if the transaction times out halfway through?
- Which leg failed?
- Is the balance in a partial state?
- How do you retry this safely?
- Do you use savepoints? Nested transactions? Custom triggers?

With TigerBeetle:

```go
results := e.tbClient.CreateTransfers(transfers)  // 1 call, 3 lines
```

All three legs succeed atomically, or all three fail together. No partial states. No savepoints. No triggers. TigerBeetle doesn't give you a choice—it's atomic by design.

---

## 2. Idempotency Is Baked In

When a network fails and you retry, you need to know: did I already do this?

With MongoDB, you'd write something like:

```javascript
try {
  db.transfers.insertOne({
    _id: ObjectId(),  // Random
    paymentUUID: "uuid-123",
    amount: 100
  });
} catch (DuplicateKeyError) {
  // Was this a duplicate insert, or a different error?
  const existing = db.transfers.findOne({paymentUUID: "uuid-123"});
  if (!existing) {
    throw new Error("Something else went wrong");
  }
  // Found it. Is it in the right state? Need more checks...
}
```

This is a trap. You're checking if a document exists, but that doesn't mean the transfer completed correctly. You could have a document in a half-failed state.

With TigerBeetle, the clearing engine uses deterministic IDs:

```go
transfer.ID = sha256(paymentUUID + "1")  // Same ID every time
results := e.tbClient.CreateTransfers(transfers)
// Retry with same ID? Idempotent. Already exists? TigerBeetle deduplicates.
```

No deduplication logic. No state checking. Same ID = same result, guaranteed.

---

## 3. Linked Transfers (No Cascade Logic)

In this settlement system, Leg 2 depends on Leg 1. If Leg 1 fails, Leg 2 shouldn't exist.

With PostgreSQL, you'd use foreign keys and triggers:

```sql
-- Create Leg 1
INSERT INTO transfers (id, from_account, to_account, amount, status)
VALUES (1, alice, bank_a_internal, 100, 'pending');

-- Create Leg 2, link to Leg 1
INSERT INTO transfers (id, from_account, to_account, amount, status, pending_id)
VALUES (2, bank_a_reserve, bank_b_reserve, 100, 'pending', 1)
ON DELETE CASCADE;

-- Now add a trigger to enforce order
CREATE TRIGGER enforce_linked_order BEFORE UPDATE ON transfers
FOR EACH ROW
WHEN (NEW.status = 'posted' AND OLD.pending_id IS NOT NULL)
EXECUTE FUNCTION check_predecessor_posted();
```

Now you have trigger logic. You have cascade rules. You have stored procedures. All to say: "these transfers are linked."

With TigerBeetle, two flags:

```go
transfers[0].Flags = 0x3  // PENDING | LINKED
transfers[1].Flags = 0x3  // PENDING | LINKED
transfers[1].PendingID = transfers[0].ID  // Chain to Leg 1
transfers[2].Flags = 0x2  // PENDING only (terminal)
transfers[2].PendingID = transfers[1].ID
```

If Leg 1 fails, Legs 2 and 3 fail too. Atomically. No triggers. TigerBeetle enforces this.

---

## 4. Pending Balances Are Invisible

When a transfer is pending, it shouldn't affect available balance. You need to show the user: "You have $1000 available, but $100 is pending."

With SQL, every balance query becomes complicated:

```sql
SELECT 
  SUM(CASE WHEN status = 'posted' THEN amount ELSE 0 END) as available_balance,
  SUM(amount) as total_balance
FROM transfers
WHERE account_id = 'alice' AND direction = 'debit';
```

But what if a developer forgets this logic? They query the raw balance and get the wrong number.

Also, when you post a transfer in Phase 2, did you already deduct the balance in Phase 1? Or not yet? You need to track this carefully.

With TigerBeetle:

```go
transfer.Flags = 0x2  // PENDING
// That's it. Pending transfers don't affect available balance.
// Post the transfer, and the balance updates automatically.
```

No CASE statements. No developer confusion. Pending = invisible.

---

## 5. Crash Recovery Without Compensation Logic

Imagine this timeline:

1. Phase 1 creates three pending transfers
2. **System crashes**
3. Kafka retries the message
4. Phase 2 runs

With MongoDB, you'd implement the Saga pattern:

```javascript
// Step 1: Record the saga
const saga = db.sagas.insertOne({
  paymentId: "uuid-123",
  phase: 1,
  status: "in_progress"
});

// Step 2: Create transfers
const result = await createPendingTransfers(payment);

// Step 3: Update saga (but crash before this?)
await db.sagas.updateOne({_id: saga._id}, {status: "phase1_complete"});

// ** CRASH HERE **

// On startup: Check pending sagas
const pendingSaga = db.sagas.findOne({status: "phase1_complete"});
if (pendingSaga) {
  // Were all 3 transfers created?
  const leg1 = db.transfers.findOne({paymentId, legIndex: 1});
  const leg2 = db.transfers.findOne({paymentId, legIndex: 2});
  const leg3 = db.transfers.findOne({paymentId, legIndex: 3});
  
  if (leg1 && leg2 && leg3) {
    // Continue to Phase 2
  } else if (!leg1 && !leg2 && !leg3) {
    // Restart Phase 1
  } else {
    // Partial state! Need compensation logic
    await compensatePartialPhase1(saga);
  }
}
```

Now you're maintaining saga state. You're checking individual transfer state. You have compensation logic for partial failures.

With TigerBeetle and Kafka:

1. Phase 1 creates three pending transfers (atomic)
2. **System crashes before committing offset**
3. Kafka retries the message
4. Phase 1 repeats with same transfer IDs → idempotent, succeeds immediately
5. Phase 2 runs

No saga table. No compensation logic. No partial state checking. Kafka's offset is the saga log.

---

## 6. Double-Entry Bookkeeping Is Automatic

Every financial transaction needs a debit and a credit. Always.

User loses $100. Bank gains $100. These must always match.

With SQL, you'd enforce this with constraints:

```sql
-- Every transfer is a debit and credit pair
INSERT INTO ledger_entries (account_id, direction, amount)
VALUES ('alice', 'debit', 100),
       ('bank_internal', 'credit', 100);

-- What if only one insert succeeds?
-- Add a trigger
CREATE TRIGGER enforce_double_entry AFTER INSERT ON ledger_entries
FOR EACH ROW
BEGIN
  -- Check that total debits == total credits
  -- Throw error if not
END;

-- Also need periodic reconciliation
SELECT 
  SUM(CASE WHEN direction = 'debit' THEN amount ELSE -amount END) as balance
FROM ledger_entries;
-- If this doesn't equal zero, something went wrong
```

With TigerBeetle, every transfer is inherently a debit-credit pair:

```go
// Leg 1: alice -100, bank_internal +100
// Leg 2: bank_a -100, bank_b +100
// Leg 3: bank_b_internal -100, bob +100
// Total: Always balanced. TigerBeetle guarantees this.
```

No triggers. No reconciliation loops. The ledger is double-entry by design.

---

## What This Project Actually Needed to Implement

Let's count actual code:

**Phase 1 executor: 106 lines**
- Create three pending transfers atomically
- Retry logic
- Error classification

**Phase 2 executor: 79 lines**
- Post or void the transfers
- Terminal error handling

**Total for 2-phase commit: ~185 lines of core logic**

With SQL or NoSQL, the same functionality would require:

- Transaction management: 50+ lines
- Trigger definitions: 60+ lines
- Stored procedures: 100+ lines
- Saga pattern implementation: 150+ lines
- Compensation logic: 80+ lines
- Reconciliation logic: 100+ lines
- Balance query helpers: 40+ lines

You're easily at 500+ lines for the same functionality. And it's still brittle. Still has edge cases.

---

## What TigerBeetle Gives You

TigerBeetle isn't a database that happens to work for payments. It's a ledger designed from the ground up for:

1. **Atomic operations** - All or nothing, no partial states
2. **Idempotency** - Same input = same result, always
3. **Linked transfers** - Dependencies between legs, enforced
4. **Pending holds** - Balance visibility control
5. **Double-entry** - Automatically balanced ledger
6. **Crash safety** - Designed for distributed systems

You don't get these from PostgreSQL or MongoDB without writing hundreds of lines of application logic. TigerBeetle gives them to you by default.

---

## The Trade-Off

TigerBeetle is specialized. It's not a general-purpose database.

You can't query payments by user without a separate index. You can't do full-text search. You can't aggregate arbitrary fields.

That's the point. TigerBeetle does one thing: move money atomically. Everything else you build on top of it.

For payment systems, that's a good trade-off.

---

## Lessons for Building Financial Systems

1. **Atomicity matters** - Partial states will ruin you. Enforce it at the database level, not in application code.

2. **Idempotency is non-negotiable** - Networks fail. Your system must be safe to retry. Deterministic IDs, not random ones.

3. **Don't implement saga patterns if you can avoid them** - Sagas are complex. Pending transfers are simpler.

4. **Double-entry bookkeeping is a constraint, not a feature** - Make it impossible to violate, not just hard.

5. **Crash recovery should be boring** - If you need compensation logic, something's wrong. The system should recover by replaying the input.

TigerBeetle forces you to get these right. That's the real win.
