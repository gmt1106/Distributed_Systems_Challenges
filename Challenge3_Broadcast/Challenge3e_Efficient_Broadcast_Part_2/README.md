# Challenge3e: Efficient Broadcast, Part II

## How to Run

**How to set up dependencies**

1. Download dependecies that are need to compile code

- ```bash
    go get github.com/jepsen-io/maelstrom/demo/go
  ```

2. Build the Challenge3e_Efficient_Broadcast_Part_2 binary and place it in $GOBIN (typically ~/go/bin)

- ```bash
    go install .
  ```

**How to execuate the code with Maelstorm**

1. Execute Maelstorm with the path to the Challenge3e_Efficient_Broadcast_Part_2 binary

- ```bash
    ./maelstrom/maelstrom test -w broadcast --bin ~/go/bin/Challenge3e_Efficient_Broadcast_Part_2 --node-count 25 --time-limit 20 --rate 100 --latency 100
  ```
- 25 nodes, run for 20 secs

## Solution Explanation

### What the problem is really asking

Part I Goal:

- msgs-per-op < 30
- median latency < 400ms
- max latency < 600ms

This means:

- Fast-path forwarding
- Aggressive fanout
- More messages in exchange for speed

Part II Goal:

- msgs-per-op < 20 ← MUCH harder
- median latency < 1s ← relaxed
- max latency < 2s ← relaxed

This means:

- Fewer sends
- More batching
- Slower, more deliberate propagation

The core tradeoff: in the distributed systems, usually can't optimize all three at once
| Metric | Meaning |
| :------- | :------: |
| Latency | How fast updates propagate |
| Bandwidth | How many messages you send |
| Fault tolerance | How robust under partitions |

### Solution

#### Basic logic

To reduce messages-per-op below 20

- Increase bathcing window slightly
  - This increase latency slightly and massively reduces message count
- Reduce gossip fanout further

Result:

- Messages-per-operation is 9.812973
- Median latency is 310 ms
- Maximum latency is 538 ms

## Problem

In this challenge, we’ll make our Efficient, Multi-Node Broadcast implementation even more efficient. Why settle for a fast distributed system when you could always make faster?

### Specification

With the same node count of 25 and a message delay of 100ms, your challenge is to achieve the following performance metrics:

- Messages-per-operation is below 20
- Median latency is below 1 second
- Maximum latency is below 2 seconds

### Evaluation

Build your Go binary as maelstrom-broadcast and run it against Maelstrom with the same command as before:

```bash
./maelstrom test -w broadcast --bin ~/go/bin/maelstrom-broadcast --node-count 25 --time-limit 20 --rate 100 --latency 100
```

On success, congratulations, you’ve completed the Broadcast challenge. Move on to the Grow-Only Counter challenge. If you’re having trouble, poke your head in at Fly.io Community forum and ask for some help.
