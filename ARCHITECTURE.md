# TigerbeetleMiniPIX Architecture

A comprehensive guide to the system design, data flows, component interactions, and settlement mechanics of TigerbeetleMiniPIX.

**Table of Contents**
- [System Overview](#system-overview)
- [High-Level Architecture](#high-level-architecture)
- [Component Details](#component-details)
- [Data Flow](#data-flow)
- [Settlement Engine](#settlement-engine)
- [Ledger & Account Model](#ledger--account-model)
- [Concurrency & Performance](#concurrency--performance)
- [Error Handling & Recovery](#error-handling--recovery)
- [Testing & Verification](#testing--verification)

---

## System Overview

TigerbeetleMiniPIX is a payment settlement simulator that demonstrates atomic payment processing using TigerBeetle's transfer mechanism. The system is built around a **3-legged settlement model** with **2-phase commit (2PC)** to ensure zero partial states.

### Core Principles

1. **Atomicity**: All payment legs either succeed together or fail together
2. **Idempotency**: Deterministic transfer IDs prevent duplicate processing
3. **Offset Management**: Consumer offset commits only after settlement completes
4. **Deterministic Routing**: Single-ledger with code-based account classification (no multi-ledger routing)

---

## High-Level Architecture

### System Components

```mermaid
graph TB
    LoadTest["🔴 Load Test Binary<br/>(cmd/loadtest)"]
    Redpanda["🟠 Redpanda Broker<br/>(Message Queue)"]
    Clearing["🟣 Clearing Engine<br/>(cmd/engine)"]
    TigerBeetle["🟢 TigerBeetle<br/>(Ledger Database)"]
    
    LoadTest -->|produce PaymentMessage| Redpanda
    Redpanda -->|consume PaymentMessage| Clearing
    Clearing -->|CreateTransfers<br/>PostTransfers<br/>VoidTransfers| TigerBeetle
    
    style LoadTest fill:#ffcccc,color:#000
    style Redpanda fill:#fff3e0,color:#000
    style Clearing fill:#f3e5f5,color:#000
    style TigerBeetle fill:#e8f5e9,color:#000
```

### Deployment Topology

```mermaid
graph LR
    Host["Linux Host<br/>(network_mode: host)"]
    
    subgraph Docker["Docker Compose"]
        TB["TigerBeetle<br/>Port: 3001<br/>Single Node<br/>Ledger=1"]
        RP["Redpanda<br/>Port: 9092<br/>Dev Container<br/>Auto-topic"]
    end
    
    subgraph "Local Go Binaries"
        Seed["cmd/seed"]
        Engine["cmd/engine"]
        LoadTest["cmd/loadtest"]
    end
    
    Seed -->|initialize| TB
    Engine -->|connect| TB
    Engine -->|consume| RP
    LoadTest -->|produce| RP
    
    style TB fill:#e8f5e9,color:#000
    style RP fill:#fff3e0,color:#000
    style Seed fill:#e0e0e0,color:#000
    style Engine fill:#f3e5f5,color:#000
    style LoadTest fill:#ffcccc,color:#000
```

---

## Component Details

### 1. Load Test Binary (cmd/loadtest)

Generates synthetic payment load and measures end-to-end latency.

```mermaid
graph TD
    Main["main()"]
    
    Main --> Config["Load Configuration<br/>--count=N<br/>--concurrency=M<br/>--rate=R msg/sec"]
    
    Config --> Producer["Producer Pool<br/>(ProducerWorker)"]
    Config --> PaymentGen["Payment Generator<br/>(GeneratePayment)"]
    Config --> Tracker["Offset Tracker<br/>(OffsetTracker)"]
    
    PaymentGen -->|generates| PaymentMsg["PaymentMessage<br/>id, from, to, amount"]
    Producer -->|rate limit| RateLimiter["Rate Limiter<br/>(rate.Limiter)"]
    RateLimiter -->|produce| Redpanda["Redpanda<br/>pix-payments topic"]
    
    Tracker -->|poll offsets| Redpanda
    Tracker -->|measure E2E latency<br/>send→confirmation| Metrics["MetricsCollector<br/>P50/P95/P99"]
    
    Metrics -->|final report| Report["Benchmark Report<br/>TPS, Latencies"]
    
    style Producer fill:#fff9c4,color:#000
    style PaymentGen fill:#fff9c4,color:#000
    style Tracker fill:#f1f8e9,color:#000
    style Metrics fill:#e0f2f1,color:#000
    style Report fill:#c8e6c9,color:#000
```

**Key Functions**:
- `GeneratePayment()` - Creates random valid payments with sender/receiver balance checks
- `ProducerWorker` - Submits messages with rate limiting and tracks send times
- `OffsetTracker` - Polls consumer offsets to detect when Phase 2 confirms completion
- `MetricsCollector` - Aggregates P50/P95/P99 percentiles for producer and E2E latency

**Parallelism**: Configurable workers (default 10) produce concurrently to the same topic.

---

### 2. Clearing Engine (cmd/engine)

Implements the settlement logic: consumes payments, executes 2PC, manages offsets.

```mermaid
graph TD
    Start["engine/main.go"]
    
    Start --> Config["Load Configuration<br/>bank_b_accept_rate=95%<br/>bank_b_delay_ms=100"]
    
    Config --> TBClient["TigerBeetle Client<br/>Connect to :3001"]
    Config --> Adapter["TigerBeetleAdapter<br/>(wraps types.Transfer)"]
    
    TBClient --> Adapter
    
    Adapter --> Phase1["Phase1Executor<br/>Create pending linked"]
    Adapter --> Phase2["Phase2Executor<br/>Post or Void"]
    Adapter --> BankB["BankBSimulator<br/>95% accept/5% reject"]
    
    Phase1 --> Consumer["Consumer<br/>(Redpanda)"]
    Phase2 --> Consumer
    BankB --> Consumer
    
    Consumer --> Loop["Message Loop"]
    
    Loop -->|for each payment| P1["Phase 1<br/>CreateTransfers"]
    P1 -->|success| DecisionLogic["BankB Decision<br/>Random 95% vs 5%"]
    P1 -->|fail| Retry["Retry with<br/>Backoff<br/>Max 3x"]
    
    DecisionLogic -->|95%| P2A["Phase 2A: POST<br/>All 3 legs posted"]
    DecisionLogic -->|5%| P2B["Phase 2B: VOID<br/>All 3 legs voided"]
    
    P2A --> Commit["Commit Offset<br/>Manual commit"]
    P2B --> Commit
    
    Retry -->|success| DecisionLogic
    Retry -->|max retries| DeadLetter["Dead Letter<br/>Log error"]
    
    style Phase1 fill:#e8f5e9,color:#000
    style Phase2 fill:#c8e6c9,color:#000
    style BankB fill:#fff9c4,color:#000
    style Consumer fill:#f3e5f5,color:#000
    style P1 fill:#b3e5fc,color:#000
    style P2A fill:#81c784,color:#000
    style P2B fill:#ef9a9a,color:#000
```

**Consumer Loop Process**:

1. **Fetch Messages**: Consume from `pix-payments` topic
2. **Phase 1**: Create 3 pending linked transfers atomically
   - All succeed → continue to Phase 2
   - Any fail → retry with exponential backoff (max 3 retries)
   - Max retries exceeded → log error, commit offset
3. **Bank B Decision**: Simulate Bank B acceptance (95%) or rejection (5%)
4. **Phase 2**: Either POST all 3 (accept) or VOID all 3 (reject)
5. **Offset Commit**: Commit offset ONLY after Phase 2 completes

**Guarantees**:
- No offset commit until settlement finishes
- On crash during Phase 2, Kafka retries the message with same Phase 1 pending transfers
- Deterministic IDs ensure Phase 1 retry is idempotent

---

### 3. TigerBeetle Instance

Single-node ledger database with atomic transfer semantics.

```mermaid
graph TB
    TB["TigerBeetle Cluster<br/>Single Node (replica-count=1)"]
    
    TB --> Ledger["Ledger ID=1"]
    
    Ledger --> Accounts["5,011 Accounts<br/>Code-classified"]
    
    Accounts --> CB["Central Bank<br/>code=1, qty=1<br/>balance=1T"]
    Accounts --> BR["Bank Reserves<br/>code=2, qty=5<br/>flags: credits_must_not_exceed_debits"]
    Accounts --> BI["Bank Internals<br/>code=3, qty=5<br/>no special flags"]
    Accounts --> UA["User Accounts<br/>code=10, qty=5,000<br/>flags: debits_must_not_exceed_credits"]
    
    Ledger --> Transfers["Transfer Journal<br/>All ledger=1<br/>Atomic all-or-nothing"]
    
    Transfers --> Flags["Transfer Flags"]
    
    Flags --> Linked["LINKED flag<br/>Chains dependencies<br/>Terminal leg: no linked"]
    Flags --> Pending["PENDING flag<br/>Invisible to balance<br/>Until post/void"]
    Flags --> Reserved["RESERVED flag<br/>(optional)<br/>Preview pending transfers"]
    
    style CB fill:#fff9c4,color:#000
    style BR fill:#f1f8e9,color:#000
    style BI fill:#e0f2f1,color:#000
    style UA fill:#ffcccc,color:#000
    style Pending fill:#fff9c4,color:#000
    style Linked fill:#e0f2f1,color:#000
```

**Account Codes**:

| Code | Name | Count | Flags | Purpose |
|------|------|-------|-------|---------|
| 1 | Central Bank | 1 | none | Source of all funds (infinite credit) |
| 2 | Bank Reserve | 5 | `credits_must_not_exceed_debits` | Per-bank liquidity pool, prevents negative |
| 3 | Bank Internal | 5 | none | Intermediate staging in 3-leg settlement |
| 10 | User Account | 5,000 | `debits_must_not_exceed_credits` | Customer accounts, prevents overdraft |

**Transfer Invariants**:
- All transfers within `ledger=1`
- Linked transfers must follow chain rules (no open chains)
- Pending transfers don't affect available balance
- Posted transfers are final
- Idempotent on transfer ID

---

### 4. Seed Service (cmd/seed)

Bootstraps the ledger with accounts and initial balances.

```mermaid
graph TD
    Start["cmd/seed"]
    
    Start --> Health["1. Health Check<br/>TigerBeetle + Redpanda"]
    
    Health -->|healthy| Account["2. Create Accounts"]
    Health -->|unhealthy| Fail["Exit with error"]
    
    Account --> CB["Central Bank<br/>code=1<br/>balance=1T"]
    
    Account --> BR["Bank Reserves<br/>5x per bank<br/>code=2<br/>balance=10M each"]
    
    Account --> BI["Bank Internals<br/>5x per bank<br/>code=3<br/>balance=0 each"]
    
    Account --> UA["User Accounts<br/>5,000x<br/>code=10<br/>balance=1M each"]
    
    CB --> Batch["3. Batch Submit<br/>to TigerBeetle<br/>Idempotent"]
    
    BR --> Batch
    BI --> Batch
    UA --> Batch
    
    Batch --> Report["4. Print Report<br/>Total: 5,011 accounts"]
    
    style CB fill:#fff9c4,color:#000
    style BR fill:#f1f8e9,color:#000
    style BI fill:#e0f2f1,color:#000
    style UA fill:#ffcccc,color:#000
    style Batch fill:#e1bee7,color:#000
```

**Idempotency**:
- Account IDs are deterministic (hash of account key)
- Can re-run seed without error if accounts exist
- TigerBeetle rejects duplicate account IDs, but seeder tolerates this

---

## Data Flow

### End-to-End Payment Settlement

```mermaid
graph TD
    User["User A (Alice)"]
    
    User -->|payment request| LoadTest["Load Test Generates<br/>PaymentMessage<br/>from=alice<br/>to=bob<br/>amount=100"]
    
    LoadTest -->|produce| Redpanda["Redpanda<br/>pix-payments topic<br/>Records: offset N"]
    
    Redpanda -->|pull message| Clearing["Clearing Engine<br/>Consumer Group"]
    
    Clearing -->|Message received<br/>offset=N<br/>NOT committed yet| P1["PHASE 1<br/>CreateTransfers"]
    
    P1 -->|Create 3 pending| TB["Leg 1: Alice → Bank A Internal<br/>Leg 2: Bank A Res → Bank B Res<br/>Leg 3: Bank B Internal → Bob<br/>Flags: PENDING | LINKED (term=no linked)"]
    
    TB -->|All 3 created| Decision{"BankB<br/>Decision"}
    
    Decision -->|95%: ACCEPT| P2A["PHASE 2A<br/>PostTransfers"]
    Decision -->|5%: REJECT| P2B["PHASE 2B<br/>VoidTransfers"]
    
    P2A -->|Post all 3| Posted["Leg 1: -100 to Alice<br/>Leg 2: -100/+100 between banks<br/>Leg 3: +100 to Bob<br/>All balances FINAL"]
    
    P2B -->|Void all 3| Voided["Leg 1: pending removed<br/>Leg 2: pending removed<br/>Leg 3: pending removed<br/>All balances RESTORED"]
    
    Posted -->|success| Commit["COMMIT OFFSET<br/>Consumer offset = N+1<br/>Message fully processed"]
    
    Voided -->|success| Commit
    
    Commit -->|ack| LoadTest_Track["Load Test Tracker<br/>Polls consumer group offset<br/>Detects: offset = N+1<br/>Records E2E latency"]
    
    style P1 fill:#b3e5fc,color:#000
    style P2A fill:#81c784,color:#000
    style P2B fill:#ef9a9a,color:#000
    style Posted fill:#c8e6c9,color:#000
    style Voided fill:#ffcccc,color:#000
    style Commit fill:#e1bee7,color:#000
```

### State Transitions

```mermaid
graph LR
    Queued["Queued<br/>(in Redpanda)"]
    
    Queued -->|Consumer fetches| Processing["Processing<br/>(Phase 1)"]
    
    Processing -->|Phase 1 fails| Retry["Retry<br/>(backoff)"]
    Retry -->|max retries| Dead["Dead Letter<br/>(logged error)"]
    Retry -->|retry ok| Processing
    
    Processing -->|Phase 1 ok| PendingLeg["Pending Legs<br/>(all 3 transfers<br/>pending | linked)"]
    
    PendingLeg -->|Bank B ← 95%| AcceptDecision["Accept Decision"]
    PendingLeg -->|Bank B ← 5%| RejectDecision["Reject Decision"]
    
    AcceptDecision -->|Phase 2A| Posted["Posted<br/>(all 3 finalized)"]
    RejectDecision -->|Phase 2B| Voided["Voided<br/>(all 3 removed)"]
    
    Posted -->|Commit offset| Confirmed["Confirmed<br/>(offset committed<br/>Message processed)"]
    Voided -->|Commit offset| Confirmed
    
    style Queued fill:#fff9c4,color:#000
    style Processing fill:#b3e5fc,color:#000
    style PendingLeg fill:#fff9c4,color:#000
    style AcceptDecision fill:#c8e6c9,color:#000
    style RejectDecision fill:#ffcccc,color:#000
    style Posted fill:#c8e6c9,color:#000
    style Voided fill:#ffcccc,color:#000
    style Confirmed fill:#e0f2f1,color:#000
```

---

## Settlement Engine

### 3-Legged Settlement Model

Every payment from User A (Bank A) to User B (Bank B) involves exactly 3 transfers:

```mermaid
graph LR
    UserA["Alice<br/>User Account<br/>(code=10)<br/>balance=1,000"]
    BankAInt["Bank A Internal<br/>(code=3)<br/>balance=0"]
    BankARes["Bank A Reserve<br/>(code=2)<br/>balance=10M"]
    BankBRes["Bank B Reserve<br/>(code=2)<br/>balance=10M"]
    BankBInt["Bank B Internal<br/>(code=3)<br/>balance=0"]
    UserB["Bob<br/>User Account<br/>(code=10)<br/>balance=500"]
    
    UserA -->|Leg 1: -100<br/>PENDING AND LINKED<br/>to Bank A Internal| BankAInt
    
    BankARes -->|Leg 2: -100/+100<br/>PENDING AND LINKED<br/>from A to B| BankBRes
    
    BankBInt -->|Leg 3: +100<br/>PENDING - terminal leg<br/>from Bank B Internal| UserB
    
    style UserA fill:#ffcccc,color:#000
    style BankAInt fill:#ffe6cc,color:#000
    style BankARes fill:#ffcccc,color:#000
    style BankBRes fill:#ffffcc,color:#000
    style BankBInt fill:#e6ccff,color:#000
    style UserB fill:#ccffcc,color:#000
```

**Why 3 Legs?**

| Leg | From | To | Amount | Flags | Purpose |
|-----|------|----|----|-------|---------|
| 1 | User A | Bank A Internal | 100 | PENDING, LINKED | Debits user; funds "in flight" |
| 2 | Bank A Reserve | Bank B Reserve | 100 | PENDING, LINKED | Interbank movement; chains to Leg 3 |
| 3 | Bank B Internal | User B | 100 | PENDING (no linked) | Credits user; terminal leg |

**Phase 1: Create Pending**

All 3 transfers are created with `PENDING | LINKED` flags. Balances are **unchanged** (pending doesn't affect available balance).

```go
transfers := []types.Transfer{
    {ID: id(payment_uuid, 1), DebitAccount: alice, CreditAccount: bank_a_internal,
     Amount: 100, Flags: PENDING | LINKED, LinkedFlag: true},
    {ID: id(payment_uuid, 2), DebitAccount: bank_a_reserve, CreditAccount: bank_b_reserve,
     Amount: 100, Flags: PENDING | LINKED, LinkedFlag: true},
    {ID: id(payment_uuid, 3), DebitAccount: bank_b_internal, CreditAccount: bob,
     Amount: 100, Flags: PENDING, LinkedFlag: false},  // Terminal leg: NO linked
}
results := tb.CreateTransfers(transfers)  // Atomic: all or nothing
```

**Phase 2A: Accept (95% of payments)**

All 3 pending transfers are **posted**, making balances **final**:

```go
results := tb.PostTransfers([]types.Transfer{leg1, leg2, leg3})
// Alice:      balance -= 100 (now -100, final)
// Bank A Res: balance -= 100 (now 9.9M)
// Bank B Res: balance += 100 (now 10.1M)
// Bob:        balance += 100 (now +100, final)
// Commit offset → payment complete
```

**Phase 2B: Reject (5% of payments)**

All 3 pending transfers are **voided**, restoring balances:

```go
results := tb.VoidTransfers([]types.Transfer{leg1, leg2, leg3})
// All pending transfers removed
// Alice:      balance unchanged (still 1,000)
// Bank A Res: balance unchanged (still 10M)
// Bank B Res: balance unchanged (still 10M)
// Bob:        balance unchanged (still 500)
// Commit offset → payment complete (failed)
```

### 2-Phase Commit (2PC) Timeline

```mermaid
graph TB
    Start["Payment received<br/>from Redpanda"]
    
    Start --> P1["PHASE 1<br/>CreateTransfers<br/>all 3 pending|linked"]
    
    P1 -->|success| Offset["Offset NOT committed<br/>(essential for crash safety)"]
    P1 -->|failure| Retry["Retry Phase 1<br/>Exponential backoff<br/>Max 3x<br/>Deterministic IDs = idempotent"]
    
    Retry -->|success after retry| Offset
    Retry -->|failure after max| DeadLetter["Log error<br/>commit offset anyway<br/>(give up)"]
    
    Offset --> Decision["BankB Simulator<br/>95%: accept<br/>5%: reject"]
    
    Decision -->|accept| P2A["PHASE 2A<br/>PostTransfers<br/>all 3 finalized"]
    Decision -->|reject| P2B["PHASE 2B<br/>VoidTransfers<br/>all 3 removed"]
    
    P2A -->|success| Commit1["Commit offset<br/>Payment settled"]
    P2B -->|success| Commit2["Commit offset<br/>Payment rejected"]
    
    P2A -->|failure| P2ARetry["Retry Phase 2A<br/>Safe: idempotent<br/>Pending transfers exist"]
    P2B -->|failure| P2BRetry["Retry Phase 2B<br/>Safe: idempotent<br/>Pending transfers exist"]
    
    P2ARetry -->|success| Commit1
    P2BRetry -->|success| Commit2
    
    style P1 fill:#b3e5fc,color:#000
    style P2A fill:#81c784,color:#000
    style P2B fill:#ef9a9a,color:#000
    style Commit1 fill:#e0f2f1,color:#000
    style Commit2 fill:#ffebee,color:#000
```

**Key Invariants**:

1. **All-or-Nothing Phase 1**: Legs 1, 2, 3 either all create or all fail
2. **Atomic Phase 2**: All 3 legs post/void together; no partial posts
3. **Offset Safety**: Offset committed only after Phase 2 succeeds
4. **Idempotent Retry**: Deterministic IDs allow safe re-execution

---

## Ledger & Account Model

### Single-Ledger Strategy

TigerbeetleMiniPIX uses a **single ledger (ID=1)** with **code-based account classification**:

```mermaid
graph TB
    Ledger["Ledger ID=1<br/>(all accounts)"]
    
    Ledger --> CB["Central Bank<br/>code=1<br/>ID=1<br/>balance=1T<br/>credit_limit=unlimited"]
    Ledger --> BA["Bank A Reserve<br/>code=2<br/>ID=2<br/>balance=10M<br/>flag: credits_must_not_exceed_debits"]
    Ledger --> BB["Bank B Reserve<br/>code=2<br/>ID=3<br/>balance=10M<br/>flag: credits_must_not_exceed_debits"]
    Ledger --> Bx["... 3 more banks<br/>code=2"]
    Ledger --> AIx["Bank A Internal<br/>code=3<br/>ID=1002<br/>balance=0"]
    Ledger --> BIx["Bank B Internal<br/>code=3<br/>ID=1003<br/>balance=0"]
    Ledger --> BxI["... 3 more internals<br/>code=3"]
    Ledger --> UA["User Accounts<br/>code=10<br/>ID=10000...14999<br/>balance varies<br/>flag: debits_must_not_exceed_credits"]
    
    style CB fill:#fff9c4,color:#000
    style BA fill:#f1f8e9,color:#000
    style BB fill:#f1f8e9,color:#000
    style Bx fill:#f1f8e9,color:#000
    style AIx fill:#e0f2f1,color:#000
    style BIx fill:#e0f2f1,color:#000
    style BxI fill:#e0f2f1,color:#000
    style UA fill:#ffcccc,color:#000
```

**Advantages**:
1. **No routing**: All transfers within ledger=1 (no cross-ledger errors)
2. **Deterministic codes**: Account type is always visible and consistent
3. **Simpler logic**: No ledger routing tables or mapping
4. **Single journal**: All transfers in one journal (easier auditing)

**Disadvantages**:
1. **Scalability limits**: Single ledger becomes a bottleneck as transaction volume grows; TigerBeetle clusters have finite throughput per ledger
2. **No regulatory isolation**: All account types in one ledger; some regulations require separate ledgers for different entity types or risk categories
3. **Fixed account structure**: Adding new account types requires code changes and potential data migration; less flexible for evolving requirements
4. **ID space constraints**: 64-bit IDs limit total accounts; with 5,000 users, 5 banks, and internal accounts, scaling to millions requires careful ID allocation strategy
5. **Difficult multi-currency support**: Single ledger assumes one currency; multi-currency requires separate ledgers per currency or complex accounting

**Account Identification**:

```
Central Bank:     code=1,  id=1
Bank Reserves:    code=2,  id=2..6 (one per bank)
Bank Internals:   code=3,  id=1002..1006 (one per bank)
User Accounts:    code=10, id=10000..14999 (5,000 users)
```

### Transfer Flags

```mermaid
graph TD
    Transfer["Transfer Flags"]
    
    Transfer --> Pending["PENDING<br/>Transfer not yet final<br/>---<br/>Created in Phase 1<br/>Posted/Voided in Phase 2<br/>Invisible to available balance"]
    
    Transfer --> Linked["LINKED<br/>Chains transfers together<br/>---<br/>If next transfer fails,<br/>this one stays pending<br/>Terminal leg: NO linked"]
    
    Transfer --> Reserved["RESERVED<br/>(optional)<br/>Preview pending balance<br/>---<br/>Useful for pre-flight checks<br/>Not used in this impl"]
    
    style Pending fill:#fff9c4,color:#000
    style Linked fill:#e0f2f1,color:#000
    style Reserved fill:#f1f8e9,color:#000
```

---

## Concurrency & Performance

### Load Test Parallelism

```mermaid
graph LR
    Main["main()"]
    
    Main --> Config["Concurrency=10<br/>Rate=100 msg/sec<br/>Payments=10,000"]
    
    Config --> RateLimiter["Rate Limiter<br/>(rate.Limiter)<br/>100 tokens/sec<br/>Burst=100"]
    
    RateLimiter --> Pool["Producer Pool<br/>10 workers"]
    
    Pool --> W1["Worker 1"]
    Pool --> W2["Worker 2"]
    Pool --> W3["Worker 3"]
    Pool --> Wx["... 7 more"]
    
    W1 --> Gen1["Generate Payment"]
    W2 --> Gen2["Generate Payment"]
    W3 --> Gen3["Generate Payment"]
    
    Gen1 --> Produce1["Produce<br/>track sendTime"]
    Gen2 --> Produce2["Produce<br/>track sendTime"]
    Gen3 --> Produce3["Produce<br/>track sendTime"]
    
    Produce1 --> Redpanda["Redpanda<br/>pix-payments"]
    Produce2 --> Redpanda
    Produce3 --> Redpanda
    
    Redpanda --> Tracker["Offset Tracker<br/>1 consumer<br/>Polls offsets"]
    
    Tracker -->|detect confirmation| Metrics["Metrics<br/>Calculate E2E latency"]
    
    style Pool fill:#fff9c4,color:#000
    style W1 fill:#e0f2f1,color:#000
    style W2 fill:#e0f2f1,color:#000
    style W3 fill:#e0f2f1,color:#000
    style Tracker fill:#f1f8e9,color:#000
    style Metrics fill:#c8e6c9,color:#000
```

**Performance Characteristics**:

| Metric | Value | Notes |
|--------|-------|-------|
| Producer workers | 10 | Configurable via `--concurrency` |
| Rate limit | 100 msg/sec | Configurable via `--rate` |
| Burst | 100 | Allows spike before throttle |
| Max payments | 10,000 | Default; configurable via `--count` |
| Batch size | 1 | Each payment is 1 message |
| Offset polling | 100ms | Poll interval for E2E tracking |

### TigerBeetle Batch Accumulator

The clearing engine batches Phase 1 and Phase 2 operations:

```mermaid
graph TD
    Consumer["Consumer Loop<br/>Fetches message"]
    
    Consumer --> Batch1["Batch Accumulator<br/>(Phase 1)"]
    
    Batch1 -->|add transfer chain| Accumulate1["Accumulate<br/>3 transfers"]
    
    Accumulate1 -->|batch full?<br/>OR timeout?| Submit1["Submit Batch<br/>CreateTransfers<br/>max 8,189/call"]
    
    Submit1 -->|success| TB1["TigerBeetle<br/>Processes atomically"]
    Submit1 -->|failure| Retry1["Retry with<br/>exponential backoff"]
    
    TB1 --> Decision["Bank B Decision"]
    
    Decision --> Batch2["Batch Accumulator<br/>(Phase 2)"]
    
    Batch2 -->|add to batch| Accumulate2["Accumulate<br/>3 transfers"]
    
    Accumulate2 -->|batch full?<br/>OR timeout?| Submit2["Submit Batch<br/>PostTransfers or<br/>VoidTransfers"]
    
    Submit2 -->|success| TB2["TigerBeetle<br/>Finalizes"]
    
    TB2 --> Commit["Commit Offset"]
    
    style Batch1 fill:#fff9c4,color:#000
    style Submit1 fill:#e0f2f1,color:#000
    style TB1 fill:#c8e6c9,color:#000
    style Batch2 fill:#fff9c4,color:#000
    style Submit2 fill:#e0f2f1,color:#000
    style TB2 fill:#c8e6c9,color:#000
```

**Batching Strategy**:
- **Phase 1**: Accumulate up to 2,729 payment chains (~8,187 transfers)
- **Phase 2**: Accumulate up to 2,729 payment chains (~8,187 transfers)
- **Timeout**: Flush batch every 100ms if not full
- **Benefit**: Reduces TigerBeetle round-trips by ~2,700x (vs one per payment)

---

## Error Handling & Recovery

### Phase 1 Error Classification

```mermaid
graph TD
    P1Error["Phase 1 Error"]
    
    P1Error --> Terminal["Terminal Errors<br/>(no retry)"]
    P1Error --> Retryable["Retryable Errors<br/>(with backoff)"]
    P1Error --> Unknown["Unknown Errors<br/>(log and continue)"]
    
    Terminal --> TEE["TransferLinkedEventChainOpen<br/>---<br/>Cause: code bug<br/>Action: log, give up"]
    Terminal --> TPE["TransferPendingTransferExpired<br/>---<br/>Cause: transfer pre-expired<br/>Action: log, give up"]
    Terminal --> TAF["TransferIdAlreadyFailed<br/>---<br/>Cause: ID already failed once<br/>Action: retry with NEW ID"]
    Terminal --> TIF["TransferInsufficientFunds<br/>---<br/>Cause: debit account insufficient<br/>Action: log, give up"]
    
    Retryable --> TNT["TransferNetworkTimeout<br/>---<br/>Cause: cluster consensus timeout<br/>Action: retry with backoff"]
    Retryable --> TIO["TigerBeetle I/O error<br/>---<br/>Cause: transient connection<br/>Action: retry with backoff"]
    Retryable --> KErr["Kafka error<br/>---<br/>Cause: broker unavailable<br/>Action: retry with backoff"]
    
    Unknown --> UErr["Other errors<br/>---<br/>Action: log and give up"]
    
    style Terminal fill:#ffebee,color:#000
    style Retryable fill:#fff9c4,color:#000
    style TNT fill:#b3e5fc,color:#000
    style TIO fill:#b3e5fc,color:#000
    style KErr fill:#b3e5fc,color:#000
```

**Retry Strategy**:

```
Attempt 1: Immediate
Attempt 2: Wait 100ms
Attempt 3: Wait 300ms (total 400ms)
Attempt 4: Wait 900ms (total 1.3s)
Max retries: 3
```

**Idempotency**:
- **Phase 1 retry**: Use SAME transfer IDs (deterministic)
- **ID already failed**: Generate NEW IDs with suffix (per phase1_retry_test.go)
- **Phase 2 retry**: Safe to repeat (pending transfers still exist)

### Offset Commit Safety

```mermaid
graph LR
    P1["Phase 1<br/>Create"]
    P2["Phase 2<br/>Post/Void"]
    Commit["Commit Offset"]
    
    P1 -->|offset NOT committed| P2
    P2 -->|offset NOT committed| P2
    P2 -->|success| Commit
    Commit -->|safe: Phase 2 complete| NextMsg["Process next msg"]
    
    P1 -->|FAIL| Retry1["Retry Phase 1<br/>OR dead letter"]
    Retry1 -->|offset NOT committed| P1
    
    P2 -->|FAIL| Retry2["Retry Phase 2<br/>offset NOT committed"]
    Retry2 -->|pending transfers exist| P2
    
    style P1 fill:#b3e5fc,color:#000
    style P2 fill:#e0f2f1,color:#000
    style Commit fill:#c8e6c9,color:#000
    style Retry1 fill:#ffcccc,color:#000
    style Retry2 fill:#fff9c4,color:#000
```

**Crash Safety**:

1. **Crash during Phase 1**: Kafka retries message, Phase 1 repeats (idempotent)
2. **Crash after Phase 1, before Phase 2**: Kafka retries, Phase 1 pending transfers exist in TigerBeetle, Phase 2 repeats (idempotent)
3. **Crash after Phase 2, before offset commit**: Kafka retries, Phase 1 already posted/voided, Phase 2 repeat tries to post/void again (idempotent)

---

## Testing & Verification

### Test Coverage

```mermaid
graph TB
    Tests["Test Suite"]
    
    Tests --> Unit["Unit Tests"]
    Tests --> Integration["Integration Tests"]
    
    Unit --> P1["phase1_test.go<br/>Happy path<br/>Create 3 transfers"]
    Unit --> P1R["phase1_retry_test.go<br/>Retry logic<br/>Max retries<br/>ID exhaustion"]
    Unit --> P2["phase2_test.go<br/>Post transfers<br/>Void transfers"]
    Unit --> BA["bankb_test.go<br/>Accept rate<br/>Rejection logic"]
    Unit --> OO["offset_manager_test.go<br/>Offset tracking"]
    Unit --> ID["idempotency_test.go<br/>Deterministic IDs"]
    
    Integration --> E2E["integration_test.go<br/>Full 2PC flow<br/>Offset commit<br/>Batch accumulator"]
    Integration --> LoadTest["loadtest<br/>E2E latency<br/>P50/P95/P99<br/>TPS"]
    
    style P1 fill:#e0f2f1,color:#000
    style P1R fill:#b3e5fc,color:#000
    style P2 fill:#e0f2f1,color:#000
    style BA fill:#fff9c4,color:#000
    style E2E fill:#c8e6c9,color:#000
    style LoadTest fill:#ffcccc,color:#000
```

### Verification Checklist

```mermaid
graph TD
    Verify["Verification<br/>Checklist"]
    
    Verify --> A["Atomicity<br/>---<br/>[] 3 legs all created or none<br/>[] 3 legs all posted or none<br/>[] 3 legs all voided or none"]
    
    Verify --> I["Idempotency<br/>---<br/>[] Phase 1 retry safe (same IDs)<br/>[] Phase 2 retry safe (idempotent)<br/>[] Deterministic IDs consistent"]
    
    Verify --> B["Balances<br/>---<br/>[] Pending invisible to balance<br/>[] Posted final<br/>[] Voided restored"]
    
    Verify --> O["Offsets<br/>---<br/>[] Offset NOT committed in Phase 1<br/>[] Offset committed after Phase 2<br/>[] Offset persisted on disk"]
    
    Verify --> E["E2E<br/>---<br/>[] Producer latency measured<br/>[] E2E latency measured<br/>[] P50/P95/P99 calculated"]
    
    style A fill:#e0f2f1,color:#000
    style I fill:#f1f8e9,color:#000
    style B fill:#fff9c4,color:#000
    style O fill:#e6ccff,color:#000
    style E fill:#ffcccc,color:#000
```

### Load Test Verification

```mermaid
graph LR
    Start["Load Test"]
    
    Start -->|10,000 payments<br/>100 msg/sec<br/>100 workers| Run["Run"]
    
    Run -->|measure| Producer["Producer Latency<br/>(send → ProduceSync)<br/>P50, P95, P99"]
    
    Run -->|measure| E2E["E2E Latency<br/>(send → offset commit)<br/>P50, P95, P99"]
    
    Producer --> Report["Benchmark Report"]
    E2E --> Report
    
    Report -->|TPS| TPS["✓ TPS: 10,000"]
    Report -->|Producer P95| PP95["✓ Producer P95 < 1ms"]
    Report -->|E2E P95| EP95["✓ E2E P95 > Producer P95<br/>(adds Phase 2 overhead)"]
    
    style Producer fill:#b3e5fc,color:#000
    style E2E fill:#e0f2f1,color:#000
    style Report fill:#c8e6c9,color:#000
    style TPS fill:#81c784,color:#000
    style PP95 fill:#81c784,color:#000
    style EP95 fill:#81c784,color:#000
```
