# Challenge5b: Multi-Node Kafka-Style Log

## How to Run

**How to set up dependencies**

1. Download dependecies that are need to compile code

- ```bash
    go get github.com/jepsen-io/maelstrom/demo/go
  ```

2. Build the Challenge5b_Multi_Node_Kafka_Style_Log binary and place it in $GOBIN (typically ~/go/bin)

- ```bash
    go install .
  ```

**How to execuate the code with Maelstorm**

1. Execute Maelstorm with the path to the Challenge5b_Multi_Node_Kafka_Style_Log binary

- ```bash
    ./maelstrom/maelstrom test -w kafka --bin ~/go/bin/Challenge5b_Multi_Node_Kafka_Style_Log --node-count 2 --concurrency 2n --time-limit 20 --rate 1000
  ```
- two nodes, run for 20 secs

## Solution Explanation

### What the problem is really asking

In this challenge, I am building a multi-node kafka-style log.
Instead of storing your logs in each node's own memory like challenge 5a, you store them in the shared lin-kv store (the linearizable key/value store) that Maelstrom provides.

It’s important to consider which components require linearizability versus sequential consistency.

#### Linearizability

One of the strongest single-object consistency models\
Implies that every operation appears to take place atomically, in some order, consistent with the real-time ordering of those operations

This model cannot be totally or sticky available; in the event of a network partition, some or all nodes will be unable to make progress

Linearizability is a single-object model, but the scope of “an object” varies\
Some systems provide linearizability on individual keys in a key-value store\
Others might provide linearizable operations on multiple keys in a table, or multiple tables in a database—but not between different tables or databases, respectively

#### linearizability vs. sequential consistency

Both guarantee that operations appear in some consistent order, but differ in how tightly that order is tied to real time

Sequential consistency:

- Operations appear in some valid order, but that order doesn't have to match real time
- A read can return a stale value as long as all nodes agree on the same order
- Real time: Node1 writes 5 → Node2 reads → gets 3 (old value, but valid)

Linearizability:

- Operations must appear in real-time order
- Once a write completes, any subsequent read by any node must see that value.
- Real time: Node1 writes 5 → Node2 reads → must get 5

Maelstrom provided key-value stores:

- seq-kv — all nodes agree on order, but reads can be slightly stale
- lin-kv — all nodes see writes immediately after they complete, no staleness

For a multi-node Kafka log, you need lin-kv because if node 1 assigns offset 5 to a message, node 2 must immediately know that offset 5 is taken before it assigns the next offset. Otherwise two nodes could assign the same offset.

### Solution

#### Data structure

I need to store three things

- keys and its offset
- keys and its messages with offset
- keys and its committed offset

These should be shared between nodes\
Use a Maelstrom's shared KV store and store them all in one place with key and value pair\
All nodes read/write from the same store\
No more in-memory maps

```go
kv := maelstrom.NewLinKV(n)
// then use keys like:
// "offset_k1", "offset_k2"    → for keyOffset
// "entries_k1", "entries_k2"  → for keyEntry (list of log)
// "committed_k1"              → for committedOffset
```

Another possible solution:\
Forward all requests to a single leader node\
One node owns all the state\
Other nodes forward their requests to the leader and relay the response back

#### obtain KV store

Need infinite loop to obtain KV store to update the value only for keyOffset:\
Two nodes could try to assign the next offset at the same time and you need to avoid duplicates

For keyEntry and committedOffset, a simple write is enough:\
Entries are written to a specific offset key that won't conflict\
Committed offsets only need the latest value

```go
kv.ReadInt(ctx, OffsetKey)
```

Returns an error in two cases:

- Key doesn't exist — returns RPCError with code KeyDoesNotExist
- Network/timeout issue — something went wrong communicating with the KV service

```go
kv.CompareAndSwap(ctx, OffsetKey, 0, 1, true)
```

Only CompareAndSwap returns an error due to contention\
PreconditionFailed when another node updated the value between your read and your compare and swap attempt\
That's why you need the retry loop around compare and swap, not around reads

#### updating offset after adding entry with that offset

My first solution was this:

1. Get the offset
2. Add new message with the found offset
3. Increment the offset. If fails, try again with the same value -> infinite loop

The issue with this solution is that if another node read the offset and use it before I increment the offset, then I added a message with offset that is already used.
I will realize I read the offset that is already read and used by other node, after I added the message with that offset and when I tried to increment the offset.

To solve this issue, fix the order of action:

1. Get the offset
2. Increment the offset. If fails, back to step 1 to get a fresh offset.
3. Add new message with the found offset

## Problem

Problem Source: https://fly.io/dist-sys/5b/

In this challenge, you’ll need to take your Single-Node Kafka system and distribute it out to multiple nodes.

Your nodes can use the linearizable key/value store provided by Maelstrom to implement your distributed, replicated log. This challenge is about correctness and not efficiency. You only need to keep up with a reasonable request rate. It’s important to consider which components require linearizability versus sequential consistency.

### Specification

This challenge works the same as the single-node except that it’s now running with two nodes. All correctness checks in Maelstrom should pass.

#### Service: lin-kv

You’ve used the seq-kv service in the Grow-only Counter challenge, however, in this challenge you can use the linearizable version called lin-kv. The API is the same but they have different consistency guarantees.

You can instantiate the Go client in the library by using the NewLinKV() function:

node := maelstrom.NewNode()
kv := maelstrom.NewLinKV(node)

### Evaluation

Build your Go binary as maelstrom-kafka and run it against Maelstrom with the following command:

./maelstrom test -w kafka --bin ~/go/bin/maelstrom-kafka --node-count 2 --concurrency 2n --time-limit 20 --rate 1000

This will run a two-node system for 20 seconds with 4 clients (2n). It will validate the system for correctness.

If you’re successful, that’s great! Continue on to the Efficient Kafka challenge. If you’re having trouble, jump over to the Fly.io Community forum for help.
