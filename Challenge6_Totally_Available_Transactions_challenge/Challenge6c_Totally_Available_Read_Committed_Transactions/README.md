# Challenge6c: Totally-Available, Read Committed Transactions

## How to Run

**How to set up dependencies**

1. Download dependecies that are need to compile code

- ```bash
    go get github.com/jepsen-io/maelstrom/demo/go
  ```

2. Build the Challenge6c_Totally_Available_Read_Committed_Transactions binary and place it in $GOBIN (typically ~/go/bin)

- ```bash
    go install .
  ```

**How to execuate the code with Maelstorm**

1. Execute Maelstorm with the path to the Challenge6c_Totally_Available_Read_Committed_Transactions binary

- ```bash
    ./maelstrom/maelstrom test -w txn-rw-register --bin ~/go/bin/Challenge6c_Totally_Available_Read_Committed_Transactions --node-count 2 --concurrency 2n --time-limit 20 --rate 1000 --consistency-models read-committed --availability total –-nemesis partition
  ```

  - two node, 2 \* node count concurrent virtual clients, run for 20 secs
  - this ensure that transactions are totally-available in the face of network partitions
  - checks consistency + availability together under real partitions
  - it's the test that actually validates the core design constraint from the spec: nodes must keep answering locally without waiting on unreachable peers

## Solution Explanation

### What the problem is really asking

G1a (aborted read) and G1b (intermediate read) — already structurally guaranteed

- G1b: a whole txn's ops run under one mu.Lock() hold and nothing else can observe it mid-batch. It commits atomically or not at all.

- G1a: we never apply-then-undo. A txn is fully applied before we reply, so there's no partial/speculative state to leak. As long as we never introduce undoable writes, G1a can't happen.

G1c (circular wr/rw dependency) — the real one to think about

wr and rw dependencies both connect "a read" to "a write" on the same key, which is why they look alike at first glance. The difference is what actually happened between them:

|                         | wr dependency                                               | rw dependency (anti-dependency)                                      |
| ----------------------- | ----------------------------------------------------------- | -------------------------------------------------------------------- |
| What the reader saw     | The write itself — the read observed the writer's new value | Nothing — the read missed the write entirely and got the older value |
| Edge direction          | `T_writer → T_reader` (reader depends on the writer)        | `T_reader → T_writer` (the writer invalidates what the reader saw)   |
| Requires, physically    | The write to have already reached the reader's node         | The write to have **not yet** reached the reader's node              |
| Possible in our design? | No                                                          | Yes                                                                  |

**wr example — why it's impossible here**

`T1`'s op list: `["w", x, 1]`, `["r", y, 2]`\
`T2`'s op list: `["w", y, 2]`, `["r", x, 1]`

Dependency edges: `T1` writes `x=1`, and `T2` reads that `x=1` → edge `T1 → T2`. `T2` writes `y=2`, and `T1` reads that `y=2` → edge `T2 → T1`. Combined: `T1 → T2 → T1`.

For this to actually happen, `T1` must read `T2`'s `y=2` (meaning `T2` already fully committed before `T1` ran), _and_ `T2` must read `T1`'s `x=1` (meaning `T1` already fully committed before `T2` ran). Both can't be true at once — each side would need to finish before the other. Contradiction, so this cycle can't form as long as replication only ever flows forward from a completed commit.

**rw example — why it's real**

`T1`'s op list: `["w", x, 1]`, `["r", y, nil]`\
`T2`'s op list: `["w", y, 2]`, `["r", x, nil]`

Instead of reading the other's fresh write, each one reads the old value because replication hasn't caught up yet: `T1` writes `x=1` but its read of `y` returns `nil` (misses `T2`'s write, which hasn't replicated over yet); `T2` writes `y=2` but its read of `x` returns `nil` (misses `T1`'s write, same reason).

Dependency edges: `T2` read `x=nil`, the version right before `T1`'s `x=1` → edge `T2 → T1` (T1's write invalidates what T2 read). `T1` read `y=nil`, the version right before `T2`'s `y=2` → edge `T1 → T2`. Combined: `T1 → T2 → T1`.

**The distinction in one sentence**: a wr edge requires the write to have already arrived (which, in our design, is only possible after the writer fully committed, so impossible to cycle), while an rw edge requires the write to have not arrived yet (the ordinary case under async replication). That's why wr-cycles can't happen here, but rw-cycles can.

#### aborted read (G1a)

`T1`'s op list: `["w", x, 1]` — then `T1` gets aborted (replied to with txn-conflict, never commits)\
`T2`'s op list: `["r", x, 1]`

If `T2`'s read returns 1, that's a G1a violation because `T1` never committed, so its write to `x` should never have been visible to anyone.

#### intermediate read (G1b)

`T1`'s op list: `["w", x, 1]` ... later ... `["w", x, 3]` (T1's own two writes to `x` — 3 is `T1`'s final write)
`T2`'s op list: `["r", x, 1]`

History of key `x` from `T1` alone: 1 → 3

`T2` read 1 which is an intermediate value `T1` produced on its way to its real final value of 3. Notice this is different from your G0 example: there, `T2` wrote `x`=2 between `T1`'s two writes, creating a ww-cycle. Here it's purely `T1`'s own progression being peeked at mid-transaction. `T2` should only ever have been able to see 3 (`T1`'s committed final state) or nothing at all, never 1.

#### circular information flow (G1c)

`T1`'s op list: `["w", x, 1]`, `["r", y, 2]`
`T2`'s op list: `["w", y, 2]`, `["r", x, 1]`

Dependency edges (this time wr — write-read — not ww):

- `T1` writes `x`=1, and `T2` reads that `x`=1 → edge `T1` → `T2`
- `T2` writes `y`=2, and `T1` reads that `y`=2 → edge `T2` → `T1`

Combined: `T1` → `T2` → `T1`. Same shape as G0's cycle, but built from wr edges instead of ww edges. Each transaction read a value the other wrote, so neither can be said to have run "before" the other.

### Solution

- Base: fixed 6b main.go (atomic local batch apply, LWW writes, async fire-and-forget retry replication) — covers G0, G1a, and the wr-half of G1c.
- Per-write versioning (fixes G1b): version each write op, not each transaction — add an `OpIndex` tiebreaker between `Counter` and `NodeID` (`Counter`, `OpIndex`, `NodeID`), so a later write to the same key within the same txn always outranks an earlier one. `replicate` carries a version per write, reconstructed locally on each side from the write's position in the op array, instead of one shared version per batch. Found this bug the same way as the 6b bug: ran the code against the real checker instead of assuming correctness, saw `:anomaly-types (:G1b)`, traced it to same-key double-writes sharing one version.
- rw-cycles (the remaining G1c gap): considered, but not implemented. Shrinking the staleness window that enables an rw-cycle requires waiting somewhere in the request path — either the writer blocks its reply on peer acks, or the reader blocks on a causal watermark before answering. Both add latency for a risk that's only theoretical here: with just the two fixes above, the actual 6c Maelstrom test (`read-committed`, `--availability total`, `--nemesis partition`) passes. No anomalies observed, so no wait-based fix was added — it'd be solving a problem with no evidence it occurs at this workload's scale.
- Structural limit, for the record: during a real partition, staleness on the cut-off side is unavoidable under total availability, so rw-cycle risk can never be fully eliminated by any local-only mechanism — only shrunk, at the cost of latency.

## Problem

Problem Source: https://fly.io/dist-sys/6c/

Now that you have data replicating between nodes from the Totally-Available, Read Uncommitted Transactions challenge, we’ll increase the difficulty by strengthening our consistency model to Read Committed.

Read Committed adds consistency guarantees while still being a relatively weak consistency model. In addition to G0, it also prohibits three additional anomalies:

- **G1a (aborted read)**: An aborted transaction T1 writes key x, and a committed transaction T2 reads that write to x.

- **G1b (intermediate read)**: A committed transaction T2 reads a value of key x that was generated by transaction T1 other than T1’s final write of x.

- **G1c (circular information flow)**: A cycle of transactions linked by either write-read or read-write dependencies. For instance, transaction T1 writes x = 1 and reads y = 2, and transaction T2 writes y = 2 and reads x = 1

In summary, these ensure that changes made during a transaction are not visible to other transactions until they are committed.

### Specification

Ensure your distributed key/value store implements a Read Committed consistency model while also preserving total availability.

#### Aborting transactions

To test G1a, you can abort a transaction by replying:

```json
{
  "type": "error",
  "in_reply_to": 1,
  "code": 30,
  "text": "The requested transaction has been aborted because of a conflict with another transaction. Servers need not return this error on every conflict: they may choose to retry automatically instead."
}
```

The Maelstrom code of 30 is txn-conflict. The in_reply_to should be the message ID of the request to be aborted.

This can be done with the Go client with:

```go
return n.Reply(
    msg,
    map[string]any{
        "type": "error",
        "code": maelstrom.TxnConflict,
        "text": "txn abort",
    },
)
```

### Evaluation

Build your Go binary as maelstrom-txn and run it against Maelstrom with the following command:

./maelstrom test -w txn-rw-register --bin ~/go/bin/maelstrom-txn --node-count 2 --concurrency 2n --time-limit 20 --rate 1000 --consistency-models read-committed --availability total –-nemesis partition

If you’re successful, that’s amazing! You’ve made it to the end of the Fly.io Distributed Systems Challenge. 🎉🎉🎉

If you’re having trouble, jump over to the Fly.io Community forum for help.

If you’ve helped others working on these challenges in the Community forum, we offer our sincere thanks. These are hard topics to wrap our brains around and we all need a hand from time to time.
