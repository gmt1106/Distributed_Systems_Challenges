# Challenge4: Grow Only Counter

## How to Run

**How to set up dependencies**

1. Download dependecies that are need to compile code

- ```bash
    go get github.com/jepsen-io/maelstrom/demo/go
  ```

2. Build the Challenge4_Grow_Only_Counter binary and place it in $GOBIN (typically ~/go/bin)

- ```bash
    go install .
  ```

**How to execuate the code with Maelstorm**

1. Execute Maelstorm with the path to the Challenge4_Grow_Only_Counter binary

- ```bash
    ./maelstrom/maelstrom test -w g-counter --bin ~/go/bin/Challenge4_Grow_Only_Counter --node-count 3 --rate 100 --time-limit 20 --nemesis partition
  ```
- 3 nodes, run for 20 secs

## Solution Explanation

### What the problem is really asking

#### Difference between last challenge and this one

The broadcast challenges taught me: "How do I replicate state myself across unreliable nodes?"

This counter challenge teaches me the opposite lesson: "When can I not replicate at all and let a shared service do the hard part?"

##### 1. Where state lives

In the braodcast challenge: \
The sate leaves inside my nodes.

```text
Node A: messages {1,2,3}
Node B: messages {1,2}
Node C: messages {1,3}
```

In the coutner challenge: \
The state leaves outside my nodes and my nodes are just clients.
I do not own consistency.

```text
seq-kv:
  "counter" → 123456
```

##### 2. Communicational model

In the braodcast challenge: \
nodes talk to each other to kkep the consistency

In the coutner challenge: \
nodes never talk to each other

##### 3. Failure model

In the braodcast challenge:

- Messages dropped
- Nodes partitioned
- Duplicate delivery
- Out-of-order arrival

In the coutner challenge:

- Two nodes try to update the counter at the same time

##### 4. Why eventual consistency is allowed

In the braodcast challenge: \
Reads must reflect all delivered messages.

In the coutner challenge: \
Reads can be stale, because

- Counter only increases
- Stale reads still move forward over time
- Final value matters most

### Solution

#### Basic logic

I am building

- A thin stateless adaptor
- Around a shared KV store
- With safe concurrent increments

This is the classic lost-update problem \
If the two nodes read 10 and both write 11, one increment is lost

I must use CompareAndSwap to get hold on a shared value (counter) that multiple nodes are updating concurrently and increment. \
But read can be simple because this service need only be eventually consistent.

#### context.Context

It is a standard Go mechanism for:

- cancellation
- timeouts / deadlines
- request scoping
- passing request-wide metadata

seq-kv needs it becuase kv.ReadInt() and kv.CompareAndSwap() are RPCs over the network. The context tells Go "how long I am willing to wait for this operation". \
Without a context, the RPC could hang forever.

What context should I use for this challege: \
keep it simple

```go
ctx := context.Background()
```

- no timeout
- no cancellation
- let it run until it finishes

Use it for every kv call

#### Contention on the single global key

If I create one global counter called "counter" that is shared among all nodes, combining with the sequential consistency model of seq-kv, these issues might happen:

1. **Contention**: When all nodes try to update the same counter key, many Compare-And-Swap (CAS) operations fail and retry. This creates high traffic and load on the KV service.
2. **Staleness**: Sequential consistency guarantees a total order of operations, but it allows individual nodes to lag behind in their view of the state, especially under partitions or heavy load. If Node 3's view of the counter key is slightly outdated during the final read, it returns an incorrect value.

To fix this, I should implement a G-Counter (Grow-Only Counter) CRDT pattern:

- **Partition the State**: Instead of a single counter key, have each node maintain its own counter (e.g., counter_n1, counter_n2).
- **Writes**: When a node receives an add request, it increments only its own key. This eliminates contention between nodes.
- **Reads**: When a node receives a read request, it fetches the values of all node counters and sums them up.

## Problem

In this challenge, you’ll need to implement a stateless, grow-only counter which will run against Maelstrom’s g-counter workload. This challenge is different than before in that your nodes will rely on a sequentially-consistent key/value store service provided by Maelstrom.

### Specification

Your node will need to accept two RPC-style message types: add & read. Your service need only be eventually consistent: given a few seconds without writes, it should converge on the correct counter value.

Please note that the final read from each node should return the final & correct count.

### RPC: add

Your node should accept add requests and increment the value of a single global counter. Your node will receive a request message body that looks like this:

```json
{
  "type": "add",
  "delta": 123
}
```

and it will need to return an "add_ok" acknowledgement message:

```json
{
  "type": "add_ok"
}
```

### RPC: read

Your node should accept read requests and return the current value of the global counter. Remember that the counter service is only sequentially consistent. Your node will receive a request message body that looks like this:

```json
{
  "type": "read"
}
```

and it will need to return a "read_ok" message with the current value:

```json
{
  "type": "read_ok",
  "value": 1234
}
```

### Service: seq-kv

Maelstrom provides a sequentially-consistent key/value store called seq-kv which has read, write, & cas operations. The Go library provides a KV wrapper for this service that you can instantiate with NewSeqKV():

```go
node := maelstrom.NewNode()
kv := maelstrom.NewSeqKV(node)
```

The API is as follows:

```go
func (kv *KV) Read(ctx context.Context, key string) (any, error)
Read returns the value for a given key in the key/value store. Returns an
*RPCError error with a KeyDoesNotExist code if the key does not exist.

func (kv \*KV) ReadInt(ctx context.Context, key string) (int, error)
ReadInt reads the value of a key in the key/value store as an int.

func (kv \*KV) Write(ctx context.Context, key string, value any) error
Write overwrites the value for a given key in the key/value store.

func (kv \*KV) CompareAndSwap(ctx context.Context, key string, from, to any, createIfNotExists bool) error
CompareAndSwap updates the value for a key if its current value matches the
previous value. Creates the key if createIfNotExists is true.

    Returns an *RPCError with a code of PreconditionFailed if the previous value
    does not match. Return a code of KeyDoesNotExist if the key did not exist.
```

### Evaluation

Build your Go binary as maelstrom-counter and run it against Maelstrom with the following command:

```bash
./maelstrom test -w g-counter --bin ~/go/bin/maelstrom-counter --node-count 3 --rate 100 --time-limit 20 --nemesis partition
```

This will run a 3-node cluster for 20 seconds and increment the counter at the rate of 100 requests per second. It will induce network partitions during the test.

If you’re successful, right on! Continue on to the Kafka-Style Log challenge. If you’re having trouble, ask for help on the Fly.io Community forum.
