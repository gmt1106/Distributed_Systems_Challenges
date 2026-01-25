# Challenge3d: Efficient Broadcast, Part I

## How to Run

**How to set up dependencies**

1. Download dependecies that are need to compile code

- ```bash
    go get github.com/jepsen-io/maelstrom/demo/go
  ```

2. Build the Challenge3d_Efficient_Broadcast_Part_1 binary and place it in $GOBIN (typically ~/go/bin)

- ```bash
    go install .
  ```

**How to execuate the code with Maelstorm**

1. Execute Maelstorm with the path to the Challenge3d_Efficient_Broadcast_Part_1 binary

- ```bash
    ./maelstrom/maelstrom test -w broadcast --bin ~/go/bin/Challenge3d_Efficient_Broadcast_Part_1 --node-count 25 --time-limit 20 --rate 100 --latency 100
  ```
- 25 nodes, run for 20 secs

## Solution Explanation

### What the problem is really asking

The code already passed the correctness. The challenge is about reducing fanout, duplicaiton, and latency without breaking safety.

I need to achieve the following:

- Messages-per-operation is below 30
- Median latency is below 400ms
- Maximum latency is below 600ms

In order to check the result from results.edn, search following:

- Messages-per-operation -> :msgs-per-op
- Median latency -> :stable-latencies 0.5
- Maximum latency -> :stable-latencies 1

According to the results.edn of the challenge 3c solution code running with 25 nodes and 100ms delay to each message, this is current status:

- Messages-per-operation is 4.73
- Median latency is 1529 ms
- Maximum latency is 2395 ms

If messages-per-operation is too high, need to reduce fanout, gossip frequency and full-set sends.\
If latency is too high, increase fanout, fast-path forwarding and gossip frequency.

#### Fanout

fanout is how many peers you send a message to in one forwarding step.

- Low fanout -> Fewer messages, higher latency
- High fanout -> More messages, lower latency

Instead of sending messages to all neighbours (full-set sending), set fanout to 2 (pick random 2 neighbours) is better because:

- Minimal redundancy
- Exponetial spread
- Same or fewer hops
- Lower queueing delay

Latency is dominated by hop depth, not neighbor count.

#### Full-set sending

Full-set sending is sending everything you know every time. \
This is bad because of huge payloads, some data sent repeatedly, massive duplication and msgs-per-op explodes.

#### Fast-path forwarding

Fast-path forwarding is immediately forwarding new messages when you receive them.

### Solution

#### Basic logic

I have too high latency.\
There is 100ms network delay and gossip wait for tick in every 300ms. Therefore, for meesages to hop from one node to the other costs ~400ms.

What I should do:

- Add fast-path forwarding
  - When a node receives a new message, immediately forward it to few peers
- Set fanout
- Do not send full message sets every time
  - Keep track of message I sent to each neighbour and only send missing ones
- Ignore the grid topology
  - Gossip with random nodes from the entire cluster to create a "small-world" network, which reduces the number of hops

Result:

- Messages-per-operation is 12.495944
- Median latency is 309 ms
- Maximum latency is 584 ms

#### rand library

rand.Perm(n):

- It returns a slice of integers [0, 1, ..., n-1] in random order
- The length of the slice is n
- It does not modify anything else, just gives a random permutation of indices

## Problem

In this challenge, we’ll improve on our Fault-Tolerant, Multi-Node Broadcast implementation. Distributed systems have different metrics for success. Not only do they need to be correct but they also need to be fast.

The neighbors Maelstrom suggests are, by default, arranged in a two-dimensional grid. This means that messages are often duplicated en route to other nodes, and latencies are on the order of 2 \* sqrt(n) network delays.

### Specification

We will increase our node count to 25 and add a delay of 100ms to each message to simulate a slow network. This could be geographic latencies (such as US to Europe) or it could simply be a busy network.

Your challenge is to achieve the following:

    Messages-per-operation is below 30
    Median latency is below 400ms
    Maximum latency is below 600ms

Feel free to ignore the topology you’re given by Maelstrom and use your own; it’s only a suggestion. Don’t compromise safety under faults. Double-check that your solution is still correct (even though it will be much slower) with --nemesis partition

### Messages-per-operation

In the results.edn file produced by Maelstrom, you’ll find a :net key with information about the number of network messages. The :servers key shows just messages between server nodes, and :msgs-per-op shows the number of messages exchanged per logical operation. Almost all our operations are broadcast or read, in a 50/50 mix.

```bash
:net {:all {:send-count 129592,
            :recv-count 129592,
            :msg-count 129592,
            :msgs-per-op 65.121605},
    :clients {:send-count 4080, :recv-count 4080, :msg-count 4080},
    :servers {:send-count 125512,
                :recv-count 125512,
                :msg-count 125512,
                :msgs-per-op 63.071358}
```

In this example we exchanged 63 messages per operation. Half of those are reads, which require no inter-server messages. That means we sent on average 126 messages per broadcast, between 25 nodes: roughly five messages per node.

### Stable latencies

Under :workload you’ll find a map of :stable-latencies. These are quantiles which show the broadcast latency for the minimum, median, 95th, 99th, and maximum latency request. These latencies are measured from the time a broadcast request was acknowledged to when it was last missing from a read on any node. For example, here’s a system whose median latency was 452 milliseconds:

```bash
:stable-latencies {0 0,
0.5 452,
0.95 674,
0.99 731,
1 794},
```

### Evaluation

Build your Go binary as maelstrom-broadcast and run it against Maelstrom with the following command:

```bash
./maelstrom test -w broadcast --bin ~/go/bin/maelstrom-broadcast --node-count 25 --time-limit 20 --rate 100 --latency 100
```

You can run maelstrom serve to view results or you can locate your most recent run in the ./store directory.

On success, continue on to Part Two of the Broadcast Efficiency challenge. If you’re having trouble, head to the Fly.io Community forum.
