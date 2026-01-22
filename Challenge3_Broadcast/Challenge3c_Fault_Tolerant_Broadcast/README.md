# Challenge3c: Fault Tolerant Broadcast

## How to Run

**How to set up dependencies**

1. Download dependecies that are need to compile code

- ```bash
    go get github.com/jepsen-io/maelstrom/demo/go
  ```

2. Build the Challenge3c_Fault_Tolerant_Broadcast binary and place it in $GOBIN (typically ~/go/bin)

- ```bash
    go install .
  ```

**How to execuate the code with Maelstorm**

1. Execute Maelstorm with the path to the Challenge3c_Fault_Tolerant_Broadcast binary

- ```bash
    ./maelstrom/maelstrom test -w broadcast --bin ~/go/bin/Challenge3c_Fault_Tolerant_Broadcast --node-count 5 --time-limit 20 --rate 10 --nemesis partition
  ```
- 5 node, run for 20 secs

## Solution Explanation

### What the problem is really asking

Values should propagate to all other nodes by the end of the test

- Eventual consistency, not immediate
- Must retry/reconcile after partitions heal

### Solution

#### Basic logic

Implement Periodic Gossip:

- Every x ms, send known message to neighbors
- After partition heals, nodes re-sync automatically

For broadcast RPC handler,

- Just store the meesage body

Create a goroutine that has a timer from "time"\
For that goroutine with timer,

- Periodically send all known messages to neighbors

#### time library

time.Ticker:

- Repeatedly "ticks" at a fixed interval
- Sends the current time on a channel
- Keeps running unit I stop it

```go
ticker := time.NewTicker(300 * time.Millisecond)
```

every 300ms → send time on ticker.C\
ticker.C is a receive-only channel

```go
defer ticker.Stop()
```

ticker.Stop() releases resources and prevents the ticker from leaking goroutines \
defer ensures it runs no matter how the function exits \
So when ticker is done, cleans up the memory

Inside time.NewTicker(...), go internally starts another goroutine and it sleeps for set time and sends timestamps into ticker.C every set time

#### Network partition

It is when the cluster is split into groups of nodes that cannot communicate with each other, even though:

- all nodes are still running
- no code crashed
- no node knows why messages aren't arriving

Suppose there are 5 nodes: n0, n1, n2, n3, n4 \
A parition might split them like this:

- Partition A: n0, n1
- Partition B: n2, n3, n4

## Problem

In this challenge, we’ll build on our Multi-Node Broadcast implementation, however, this time we’ll introduce network partitions between nodes so they will not be able to communicate for periods of time.

### Specification

Your node should propagate values it sees from broadcast messages to the other nodes in the cluster—even in the face of network partitions! Values should propagate to all other nodes by the end of the test. Nodes should only return copies of their own local values.

### Evaluation

Build your Go binary as maelstrom-broadcast and run it against Maelstrom with the following command:

```bash
./maelstrom test -w broadcast --bin ~/go/bin/maelstrom-broadcast --node-count 5 --time-limit 20 --rate 10 --nemesis partition
```

This will run a 5-node cluster like before, but this time with a failing network! Fun!

On success, continue on to Part One of the Broadcast Efficiency challenge. If you’re having trouble, head to the Fly.io Community forum.
