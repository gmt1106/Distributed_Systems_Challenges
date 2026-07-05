# Challenge6a: Single-Node, Totally-Available Transactions

## How to Run

**How to set up dependencies**

1. Download dependecies that are need to compile code

- ```bash
    go get github.com/jepsen-io/maelstrom/demo/go
  ```

2. Build the Challenge6a_Single_Node_Totally_Available_Transactions binary and place it in $GOBIN (typically ~/go/bin)

- ```bash
    go install .
  ```

**How to execuate the code with Maelstorm**

1. Execute Maelstorm with the path to the Challenge6a_Single_Node_Totally_Available_Transactions binary

- ```bash
    ./maelstrom/maelstrom test -w txn-rw-register --bin ~/go/bin/Challenge6a_Single_Node_Totally_Available_Transactions --node-count 1 --time-limit 20 --rate 1000 --concurrency 2n --consistency-models read-uncommitted --availability total
  ```
- one node, run for 20 secs

## Solution Explanation

### What the problem is really asking

In this challenge, I am building a single node key-value store which implements multiple transactions. Each client request bundles multiple micro-operations together (a txn) that should be treated as one unit.

The same sequence of reads/writes can legitimately return different results depending on how strict your consistency model is. E.g. under strict consistency, a read must reflect every prior write anywhere in the system. under weaker models, a read is allowed to return a stale or even divergent value. So "correct" behavior isn't fixed. It's relative to whatever consistency guarantee you're claiming to provide.

The goal is to support weak consistency while also being totally available.

- **read-uncommitted consistency**: a weak model — reads can see uncommitted/stale writes, no isolation between concurrent transactions
- **total availability**: every node must respond successfully to every request, even during a network partition, never blocking or erroring out

Since this is single-node with total availability and read-uncommitted consistency, there's no need for anything fancier than a plain guarded map.

### Solution

#### Data structure

1. **keyValueStore**: a global in-memory hash map holding the actual key/value register state, where the key is the integer key from the transaction and the value is the last written integer. This is the whole "database" for the single-node version.
2. **sync.Mutex**: to guard concurrent access to store, since Maelstrom can dispatch handler calls concurrently.

## Problem

Problem Source: https://fly.io/dist-sys/6a/

In this challenge, you’ll need to implement a key/value store which implements transactions. These transactions contain micro-operations (read & write) and the results of those operations depends on the consistency guarantees of the challenge. Your goal is to support weak consistency while also being totally available. We begin with a single-node service and then write a multi-node version.

### Specification

Your node will support the txn-rw-register workload by implementing a key/value store that accepts only one message. How easy, right?? This message is the txn message which passes in a list of operations to perform.

Writes in this workload are unique per-key so key 100 would only ever see a write of 1 once, a write of 2 once, etc. This helps Maelstrom to verify correctness.

### RPC: txn

This message passes in an array operations in the "txn" key. Each operation is represented by a 3-element array containing the operation name, the integer key to operate on, and a possibly-null integer value.

For example, your node will receive a request message body that looks like this:

```json
{
  "type": "txn",
  "msg_id": 3,
  "txn": [
    ["r", 1, null],
    ["w", 1, 6],
    ["w", 2, 9]
  ]
}
```

This represents three operations:

- Read from key 1
- Write the value of 6 to key 1
- Write the value of 9 to key 2

In response, it should send a txn_ok message that contains the same operation list, however, read ("r") operations should have their value filled in with the current value. For example, if the value of key 1 was 3 before this transaction, it should be returned in the read operation. Non-existent keys should be returned as null.

```json
{
  "type": "txn_ok",
  "msg_id": 1,
  "in_reply_to": 3,
  "txn": [
    ["r", 1, 3],
    ["w", 1, 6],
    ["w", 2, 9]
  ]
}
```

### Evaluation

Build your Go binary as maelstrom-txn and run it against Maelstrom with the following command:

```bash
./maelstrom test -w txn-rw-register --bin ~/go/bin/maelstrom-txn --node-count 1 --time-limit 20 --rate 1000 --concurrency 2n --consistency-models read-uncommitted --availability total
```

This will verify your single-node system works before we move on to distributing our writes across nodes.

If you’re successful, continue on to the Totally-Available, Read Uncommitted Transactions challenge. If you’re having trouble, jump over to the Fly.io Community forum for help.
