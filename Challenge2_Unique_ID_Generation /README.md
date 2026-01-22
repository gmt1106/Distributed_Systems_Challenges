# Challenge2: Unique ID Generation

## How to Run

**How to set up dependencies**

1. Download dependecies that are need to compile code

- ```bash
    go get github.com/jepsen-io/maelstrom/demo/go
  ```

2. Build the Challenge2_Unique_ID_Generation binary and place it in $GOBIN (typically ~/go/bin)

- ```bash
    go install .
  ```

**How to execuate the code with Maelstorm**

1. Execute Maelstorm with the path to the Challenge2_Unique_ID_Generation binary

- ```bash
    ./maelstrom/maelstrom test -w unique-ids --bin ~/go/bin/Challenge2_Unique_ID_Generation --time-limit 30 --rate 1000 --node-count 3 --availability total --nemesis partition
  ```
- three node, run for 30 secs

## Solution Explanation

### What the problem is really asking

- No two nodes ever return the same id
- The system is totally available
- Works even during network partitions
  - Network partition happens when some nodes cannot communicate with other nodes, but each node is still running
  - Therefore, this requirement means service must continue responding correctly even if it cannot talk to other nodes
- No coordinateion between nodes is allwoed
- "Totally Available" = Node must response immediately

### CAP theorem

Choose:

- Availability
- Partition Tolerance

Give up:

- Consistency

### Solution

#### Basic logic

Use node ID + local counter

```
<node-id>-<counter>
```

- Node IDs are unique
- Counter is local

#### Counter

Each Maelstrom node is a separate process. A counter in each node is not shared across nodes.\
But there is concurrency inside one node. Inside a single node, serveris not a single-threaded.\
\
Maelstrom sends multiple generate requests very quickly often in parallel.\

My Go server reads messages and handles them concurrently. Usually one goroutine per message.
All goroutines access the same counter variable at the same time. \
\
Goroutine is a lightweight unit of execution managed by Go.\

- Goroutine = mutiple tasks in progress
- Parallelism = multiple tasks exectung at the same time

Goroutines give you concurrecny.\
\
Therefore, I need to make a thread-safe local counter using "sync/atmic".

- automatically increments counter
- returns the new value
- safe accross mutiple goroutines
- no locks needed

#### format string

Use formatting library, "fmt" to format the id.

## Problem

Problem Source: https://fly.io/dist-sys/2/

In this challenge, you’ll need to implement a globally-unique ID generation system that runs against Maelstrom’s [unique-ids](https://github.com/jepsen-io/maelstrom/blob/main/doc/workloads.md#workload-unique-ids) workload. Your service should be totally available, meaning that it can continue to operate even in the face of network partitions.

### Specification

RPC: generate\
Your node will receive a request message body that looks like this:

```json
{
  "type": "generate"
}
```

and it will need to return a "generate_ok" message with a unique ID:

```json
{
  "type": "generate_ok",
  "id": 123
}
```

The msg_id and in_reply_to fields have been removed for clarity but they exist as described in the previous challenge. IDs may be of any type–strings, booleans, integers, floats, arrays, etc.

### Evaluation

Build your node binary as maelstrom-unique-ids and run it against Maelstrom with the following command:

```bash
./maelstrom test -w unique-ids --bin ~/go/bin/maelstrom-unique-ids --time-limit 30 --rate 1000 --node-count 3 --availability total --nemesis partition
```

This will run a 3-node cluster for 30 seconds and request new IDs at the rate of 1000 requests per second. It checks for total availability and will induce network partitions during the test. It will also verify that all IDs are unique.

If you see an “Everything looks good” message, congrats! Continue on to the Broadcast challenge. If you’re having trouble, ask for help on the Fly.io Community forum.
