# Challenge3b: Multi-Node Broadcast

## How to Run

**How to set up dependencies**

1. Download dependecies that are need to compile code

- ```bash
    go get github.com/jepsen-io/maelstrom/demo/go
  ```

2. Build the Challenge3b_Multi_Node_Broadcast binary and place it in $GOBIN (typically ~/go/bin)

- ```bash
    go install .
  ```

**How to execuate the code with Maelstorm**

1. Execute Maelstorm with the path to the Challenge3b_Multi_Node_Broadcast binary

- ```bash
    ./maelstrom/maelstrom test -w broadcast --bin ~/go/bin/Challenge3b_Multi_Node_Broadcast --node-count 5 --time-limit 20 --rate 10
  ```
- 5 nodes, run for 20 secs

## Solution Explanation

### What the problem is really asking

Don’t resend everything you know every time. Send only what the other nodes are missing.

### Solution

#### Basic logic

The naive approach:

- Send All stored messages to All neighbors

\
What a "real broadcast system" does: Incremental Gossip

- Each message has a unique value
- Nodes remember what they've seen
- When a node sees a new message,
  - Stores it
  - Forwards only that message to neighbors
- Neighbors also repeat that process

\
For broadcast RPC handler,

- Before storing the message integer
- Check if the message is not in the map
- If not, forward the message to all neighbors

For toplogy RPC handler,

- Receive and read the request message
- Store the neighbor nodes information

#### Race condition

Even for neighbors list that stores the names of neighboring nodes can cause the race condition.
Even if it is written once.

I don't have to make a deep copy of the neighbors before releasing the lock to start calling Send() for broadcast.
This is because when the array elements are written (for the first and the last time), new array is created in a new address inside the json.Unmarshal() function. \
Therefore, just a soft copy like below is fine. No one is writting there.

```go
myNeighbors := neighbors
```

#### forwarding to sender

_Never forward a broadcast back to the sender._ \
This is the implicit rule of gossip system.

Note that checking exists is not enough. \
Let's say there is node A and B and they are neighbor to each other. Then the message will be forwarded like this: A -> B -> A \
Once the message reaches A again, checking exists will stop from resending, but through out the process, sending alredy received message, receving the same messge, encoding json, blocking handler already happened.

Therefore, I must check if forwarding destination is not the sender.

#### Node crash - no handler

```bash
WARN [2026-01-18 21:54:01,445] jepsen test runner - jepsen.core Test crashed! clojure.lang.ExceptionInfo: Node n1 crashed with exit status 1. Before crashing, it wrote to STDOUT:
2026/01/19 00:50:17 No handler for {"id":24,"src":"n3","dest":"n0","body":{"in_reply_to":0,"type":"broadcast_ok"}}
```

What happended:

- n0 sent a broadcast to n3 using Send()
- n3 replied with broadcast_ok
- n0 was NOT expecting a reply
- The Maelstrom Go library treated this as a protocol error
- The process exited with status 1

The acknowledge with a broadcast_ok message should be only for client broadcast not for forwarded broadcasts.

Also the reason that in_reply_to is set to 0 is that, messages sent with Send() do not have a msg_id, so the library sets it to 0.

Check the msg_id and if that is not nil, reply.

#### Maelstrom messages characteristic

What is unique:

- Each boradcast value (the integer) is unique globally
- No two broadcast requests contain the same message value

What is not unique:

- Delivery is not unique
- Maelstrom might send a broadcast message to one node or multiple nodes or might retry delivery

## Problem

Problem Source: https://fly.io/dist-sys/3b/

In this challenge, we’ll build on our Single-Node Broadcast implementation and replicate our messages across a cluster that has no network partitions.

### Specification

Your node should propagate values it sees from broadcast messages to the other nodes in the cluster. It can use the topology passed to your node in the topology message or you can build your own topology.

The simplest approach is to simply send a node’s entire data set on every message, however, this is not practical in a real-world system. Instead, try to send data more efficiently as if you were building a real broadcast system.

Values should propagate to all other nodes within a few seconds.

### Evaluation

Build your Go binary as maelstrom-broadcast and run it against Maelstrom with the following command:

```bash
./maelstrom test -w broadcast --bin ~/go/bin/maelstrom-broadcast --node-count 5 --time-limit 20 --rate 10
```

This will run a 5-node cluster for 20 seconds and broadcast messages at the rate of 10 messages per second. It will validate that all values sent by broadcast messages are present on all nodes.

If you’re successful, continue on to the Fault Tolerant Broadcast challenge. If you’re having trouble, mosey on over to the Fly.io Community forum for tips.
