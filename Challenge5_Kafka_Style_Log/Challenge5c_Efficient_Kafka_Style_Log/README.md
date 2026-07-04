# Challenge5c: Efficient Kafka-Style Log

## How to Run

**How to set up dependencies**

1. Download dependecies that are need to compile code

- ```bash
    go get github.com/jepsen-io/maelstrom/demo/go
  ```

2. Build the Challenge5c_Efficient_Kafka_Style_Log binary and place it in $GOBIN (typically ~/go/bin)

- ```bash
    go install .
  ```

**How to execuate the code with Maelstorm**

1. Execute Maelstorm with the path to the Challenge5c_Efficient_Kafka_Style_Log binary

- ```bash
    ./maelstrom/maelstrom test -w kafka --bin ~/go/bin/Challenge5c_Efficient_Kafka_Style_Log --node-count 2 --concurrency 2n --time-limit 20 --rate 1000
  ```
- two nodes, run for 20 secs

## Solution Explanation

### What the problem is really asking

In this challenge, I am making a multi-node kafka-style log solution efficient.
Storing data in lin-kv has a drawback in that with multiple leaders and a high request rate, you’re likely to see frequent compare-and-swap errors due to contention on shared keys.

Current bottlenecks in 5b:

1. Two CaS (Compare and Swap) operations per send: One for the offset counter, one for the growing entry list blob. Both are shared across all nodes for the same key, so high contention.
2. Growing blob problem: `entry_<key>` accumulates all entries as JSON. As it grows, each CaS reads/writes more data and contention gets worse.
3. Every poll reads the entire blob: Then sorts it every time.

### make function

In Go, `make` is a built-in function used only to initialize three specific data types: slices, maps, and channels. Unlike `new` (which only allocates memory and returns a pointer), `make` allocates memory **and** initializes the internal data structure so it is instantly ready to use.

#### 1. Channels (`chan`)

Sets up a communication pipe between concurrent goroutines.

```go
ch := make(chan Type, capacity)
```

| Parameter             | Description                                                                                                                                                                                                    |
| --------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `chan Type`           | The type of data that passes through (e.g. `chan int`, `chan maelstrom.Message`)                                                                                                                               |
| `capacity` (optional) | Buffer size. Omit or `0` for unbuffered — sender and receiver must meet at the same time or they block. `1` or higher for buffered — sender can drop off data and leave immediately until the buffer fills up. |

#### 2. Slices (`[]Type`)

Allocates a dynamic array with a pre-configured size.

```go
slice := make([]Type, length, capacity)
```

| Parameter             | Description                                                                               |
| --------------------- | ----------------------------------------------------------------------------------------- |
| `[]Type`              | The type of slice (e.g. `[]int`, `[]string`)                                              |
| `length`              | Initial number of elements. Go fills them with zero values (`0` for int, `""` for string) |
| `capacity` (optional) | Max size before Go reallocates a bigger array. Defaults to `length` if omitted.           |

#### 3. Maps (`map[KeyType]ValueType`)

Initializes a key-value dictionary. Without `make`, the map is `nil` and will crash if you write to it.

```go
myMap := make(map[KeyType]ValueType, initialSize)
```

| Parameter                | Description                                                                       |
| ------------------------ | --------------------------------------------------------------------------------- |
| `map[KeyType]ValueType`  | Key and value types (e.g. `map[string]int`)                                       |
| `initialSize` (optional) | Hint for expected number of items — prevents repeated resizing as items are added |

### Solution

#### Option 1: Store entries individually instead of as a blob

- Replace `entry_<key>` with per-offset keys: `msg_<key>_<offset>`
- After claiming an offset via CaS, write the message with kv.Write (not CaS) — the key is unique since only one node wins each offset
- This eliminates the second CaS entirely, halving lin-kv contention
- poll reads `msg_<key>_0, msg_<key>_1, ...` sequentially until key-not-found

```go
kv := maelstrom.NewLinKV(n)
// then use keys like:
// "offset_k1", "offset_k2"    → for keyOffset
// "msg_k1_0", "msg_k2_0"      → for keyOffsetLogs
// "committed_k1"              → for committedOffset
```

**Result**

| Metric                  | 5b (baseline) | 5c (individual keys) |
| ----------------------- | ------------- | -------------------- |
| `msgs-per-op` (all)     | 9.41          | 15.06                |
| `msgs-per-op` (servers) | 6.95          | 12.59                |
| availability            | 99.96%        | 99.95%               |
| `valid?`                | true          | true                 |

\
**Analysis**

5c performed worse than 5b despite eliminating one CaS per `send`. The individual key approach (`msg_<key>_<offset>`) causes `poll` to make N lin-kv reads (one per message) instead
of 1 blob read, and since polls are more frequent than sends, that cost dominates.

The next optimization direction is **key ownership routing** — routing all operations for a key to a single designated owner node, eliminating lin-kv contention entirely.

#### Option 2: Key-based node ownership

- Route each key deterministically to one node: hash(key) % nodeCount
- The owner node keeps the log in memory, no lin-kv at all for sends
- Non-owner nodes forward to the owner
- Completely eliminates CaS contention for sends

Use following function to forward to the owner

```go
func (n *Node) RPC(dest string, body any, handler HandlerFunc) error
```

https://pkg.go.dev/github.com/jepsen-io/maelstrom/demo/go#Node.RPC

This is the function that finds the owner node of the key

```go
func ownerOf(key string, n *maelstrom.Node) string {
	// Creates a FNV hash function. FNV is a fast, simple hash algorithm
    hashFun := fnv.New32a()
	// Feeds the key string into the hash function as bytes
    hashFun.Write([]byte(key))

    nodeIDs := n.NodeIDs()
	// Gets the 32-bit hash result as an integer, then takes modulo by the number of nodes
	// This produces a number between 0 and len(nodeIDs)-1
	// Uses that number as an index into the node list to pick the owner
    return nodeIDs[int(hashFun.Sum32())%len(nodeIDs)]
}
```

Example with 2 nodes ["n0", "n1"]:

- key "foo" → hash 2851307223 → 2851307223 % 2 = 1 → owner is "n1"
- key "bar" → hash 1996459178 → 1996459178 % 2 = 0 → owner is "n0"

\
**Result**

| Metric                  | 5b (baseline) | 5c (individual keys) | 5c (ownership routing) |
| ----------------------- | ------------- | -------------------- | ---------------------- |
| `msgs-per-op` (all)     | 9.41          | 15.06                | **3.84**               |
| `msgs-per-op` (servers) | 6.95          | 12.59                | **1.39**               |
| availability            | 99.96%        | 99.95%               | **99.96%**             |
| `valid?`                | true          | true                 | **true**               |

\
**Analysis**

The ownership routing solution is significantly better than both previous approaches:

- **2.4x fewer total messages** than 5b
- **5x fewer server messages** than 5b
- Eliminated lin-kv entirely and all hot path operations are in-memory

## Problem

Problem Source: https://fly.io/dist-sys/5c/

In this challenge, you’ll need to take your Multi-Node Kafka system and make it more efficient. Storing data in lin-kv has a drawback in that with multiple leaders and a high request rate, you’re likely to see frequent compare-and-swap errors due to contention on shared keys.

### Specification

Using the Lamport diagrams (messages.svg) and network stats (results.edn) as your guide, reduce the probability of CaS failures and improve overall efficiency. Your solution should exchange significantly fewer messages per operation. Improve your latency & availability.

All correctness checks should still pass.

### Evaluation

Build your Go binary as maelstrom-kafka and run it against Maelstrom with the following command:

```
./maelstrom test -w kafka --bin ~/go/bin/maelstrom-kafka --node-count 2 --concurrency 2n --time-limit 20 --rate 1000
```

This challenge has a looser definition of success. See how much you can improve your performance and availability. Post your results or swap notes with folks on the Fly.io Community forum

Once you’re done, then you’ve completed the Kafka challenge! Continue on to the Totally-Available Transactions challenge.
