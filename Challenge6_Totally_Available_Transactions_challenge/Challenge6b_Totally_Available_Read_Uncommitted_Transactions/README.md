# Challenge6b: Totally-Available, Read Uncommitted Transactions

## How to Run

**How to set up dependencies**

1. Download dependecies that are need to compile code

- ```bash
    go get github.com/jepsen-io/maelstrom/demo/go
  ```

2. Build the Challenge6b_Totally_Available_Read_Uncommitted_Transactions binary and place it in $GOBIN (typically ~/go/bin)

- ```bash
    go install .
  ```

**How to execuate the code with Maelstorm**

1. Execute Maelstorm with the path to the Challenge6b_Totally_Available_Read_Uncommitted_Transactions binary

- ```bash
    ./maelstrom/maelstrom test -w txn-rw-register --bin ~/go/bin/Challenge6b_Totally_Available_Read_Uncommitted_Transactions -node-count 2 --concurrency 2n --time-limit 20 --rate 1000 --consistency-models read-uncommitted
  ```

  - two node, 2 \* node count concurrent virtual clients, run for 20 secs
  - only checks consistency (does read-uncommitted hold) under normal conditions

- ```bash
    ./maelstrom/maelstrom test -w txn-rw-register --bin ~/go/bin/Challenge6b_Totally_Available_Read_Uncommitted_Transactions --node-count 2 --concurrency 2n --time-limit 20 --rate 1000 --consistency-models read-uncommitted --availability total --nemesis partition
  ```

  - this ensure that transactions are totally-available in the face of network partitions
  - checks consistency + availability together under real partitions
  - it's the test that actually validates the core design constraint from the spec: nodes must keep answering locally without waiting on unreachable peers

## Solution Explanation

### What the problem is really asking

In this challenge, I am taking the single node key/value store and replicate our writes across all nodes while ensuring a Read Uncommitted consistency model.

Two things matter:

1. **Read Uncommitted consistency model**
   - Read Uncommitted is almost the weakest possible consistency model
   - It only forbids G0 (dirty write) — a cycle of write-write dependencies across concurrent transactions
   - Since this workload guarantees each write to a key uses a unique value (key 100 only ever gets written 1 once, 2 once, etc.), it's structurally very hard to even construct a G0 violation here. So the checker is extremely permissive — I don't need any ordering/locking/consensus between nodes.
2. **Total availability must survive partitions**
   - Nodes must keep answering txn requests immediately using only local state
   - They can never block waiting on other nodes, and can never fail a request just because peers are unreachable
   - This is the real constraint that shapes the design: whatever replication mechanism I add must be best-effort/asynchronous, never a dependency of the client-facing response path.

Put together: the correctness bar is low (no ordering constraints to enforce), but the availability bar is strict (never wait on or require peers).\
So the natural design is "answer locally, replicate lazily."

#### write-write dependencies

A write-write dependency (written `T_a →(ww) T_b`) exists between two transactions when they write to the same key, and `T_b`'s write is the one that comes immediately after `T_a`'s write in that key's history. It's just a record of "who overwrote whom."

if key x gets written 1 (by `T1`) and then 2 (by `T2`), with nothing else in between, then:
`T1 →(ww) T2`

#### A cycle of transactions

if you follow the dependency arrows starting from some transaction, you eventually get back to that same transaction. Like a loop in a graph: `T1 → T2 → T1`.\
Normally dependencies should form a straight line (a valid order: `T1` happened, then `T2`, then `T3`...).\
A cycle means there's no consistent way to say "this one happened before that one," because it's before and after at the same time.

#### A cycle of transactions linked by write-write dependencies (= G0)

Take the ww-dependency arrows between transactions, and check if they loop back on themselves.

`T1`'s op list: `["w", x, 1]` ... later ... `["w", x, 3]`\
`T2`'s op list: `["w", x, 2]`

History of key `x`: 1 → 2 → 3

ww-dependency edges between transactions: `T1 → T2 → T1`

This is a cycle and it is forbidden.

Both of these orderings would be fine:

- `x`: 1 → 3 → 2 (`T1 → T1 → T2`) — `T1`'s two writes are adjacent, `T2`'s write comes entirely after. No cycle: only edge is `T1→T2`.
- `x`: 2 → 1 → 3 (`T2 → T1 → T1`) — `T2`'s write comes entirely before both of `T1`'s writes. No cycle: only edge is `T2→T1`.

### Solution

- Keep local keyValueStore as-is
- txn handler stays synchronous and local-only for the response
  - apply reads/writes against the local map, reply immediately with txn_ok
  - Never wait on other nodes before replying — this is what makes it totally available.
- New "replicate" message and handler
  - Extract the write ops from the incoming txn (the ("w", key, value) triples) and batch all of them from a single txn into one "replicate" message, sent to every other node asynchronously — carrying the full ordered list of writes ({writes: [{key, value}, ...]}), not one message per write
  - Fire this off in a goroutine after replying, so replication latency/failure never affects the client response time or success
  - Retry replication in the background (simple retry loop, e.g. via n.RPC with an ack, retried until success or a bounded number of attempts) so that once a partition heals, the batch eventually reaches nodes that missed it
  - This gives eventual convergence without violating total availability, since retries happen off the request path
  - On the receiving node, the whole batch is applied atomically under a single mu.Lock()/mu.Unlock() — mirroring how the local txn handler applies its own writes — so a transaction's writes can never be split apart by another transaction's write landing in between (G0-safety), not just on the origin node but on every replica

This mirrors the broadcast challenge's gossip pattern (local-write-then-async-propagate) rather than the kafka challenge's shared-KV-store pattern, because here we explicitly must not depend on any node being reachable to succeed.

#### Independent per-write replication vs. batched per-transaction replication

| Aspect                              | Independent per-write messages                                                                 | Batched (per-transaction) messages                                                                 |
| ----------------------------------- | ---------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| Message shape                       | `{key, value}` — one write per message                                                         | `{writes: [{key, value}, ...]}` — all writes from one txn together                                 |
| Implementation complexity           | Simple — no need to track which txn a write came from                                          | More bookkeeping — must group writes by their originating txn before sending                       |
| Retry granularity                   | Retry just the one failed write                                                                | Must retry the whole batch, even if part of it already landed                                      |
| G0 (dirty write) safety on replicas | Not guaranteed — another txn's write can be applied in between two writes from the same txn    | Guaranteed — receiver applies the whole batch atomically under one lock, so nothing can interleave |
| Parallelism                         | Writes can be sent/retried concurrently                                                        | Batch is typically applied as one sequential, locked unit                                          |
| Receiver-side logic                 | Trivial: `store[key] = value` on each message                                                  | Slightly more: lock once, loop over the batch, unlock                                              |
| Relevance to this challenge (6b)    | Sufficient — G0 rarely arises in this workload, and Maelstrom's checker can't detect G0 anyway | Overkill for passing 6b, but better practice heading into Read Committed (6c)                      |

## Problem

Problem Source: https://fly.io/dist-sys/6b/

In this challenge, we’ll take our key/value store from the Single-Node Totally-Available Transactions challenge and replicate our writes across all nodes while ensuring a Read Uncommitted consistency model.

Read Uncommitted is an incredibly weak consistency model. It prohibits only a single anomaly:

    G0 (dirty write): a cycle of transactions linked by write-write dependencies. For instance, transaction T1 appends 1 to key x, transaction T2 appends 2 to x, and T1 appends 3 to x again, producing the value [1, 2, 3].

### Specification

Replicate writes from a node that receives a txn message to all other nodes.

### Evaluation

Build your Go binary as maelstrom-txn and run it against Maelstrom with the following command:

```bash
./maelstrom test -w txn-rw-register --bin ~/go/bin/maelstrom-txn --node-count 2 --concurrency 2n --time-limit 20 --rate 1000 --consistency-models read-uncommitted
```

Also, ensure that your transactions are totally-available in the face of network partitions:

```bash
./maelstrom test -w txn-rw-register --bin ~/go/bin/maelstrom-txn --node-count 2 --concurrency 2n --time-limit 20 --rate 1000 --consistency-models read-uncommitted --availability total --nemesis partition
```

There’s currently an issue in the Maelstrom checker that prohibits detection of G0 anomalies. Shout out to Ivan Prisyazhnyy for finding the issue!

However, Read Uncommitted allows almost any state to be valid so it’s likely your system is ok and you now have a distributed transaction system ready for the next challenge: Totally-Available, Read Committed Transactions.

If you’re having trouble, jump over to the Fly.io Community forum for help.
