# Challenge5a: Single-Node Kafka-Style Log

## How to Run

**How to set up dependencies**

1. Download dependecies that are need to compile code

- ```bash
    go get github.com/jepsen-io/maelstrom/demo/go
  ```

2. Build the Challenge5a_Single_Node_Kafka_Style_Log binary and place it in $GOBIN (typically ~/go/bin)

- ```bash
    go install .
  ```

**How to execuate the code with Maelstorm**

1. Execute Maelstorm with the path to the Challenge5a_Single_Node_Kafka_Style_Log binary

- ```bash
    ./maelstrom/maelstrom test -w kafka --bin ~/go/bin/Challenge5a_Single_Node_Kafka_Style_Log --node-count 1 --concurrency 2n --time-limit 20 --rate 1000
  ```
- single node, run for 20 secs

## Solution Explanation

### What the problem is really asking

In this challenge, I am building a single-node kafka-style log.

### Solution

#### Basic logic

For send RPC handler,

- Receive and read the request message
- Store the integer message for designated key from the message body
- I need to assign a offset for that message of that key
- Store the next available offset for that key
- Send the response message with offset

For poll RPC handler,

- Receive the request message that has map of key and offset
- For each key, find all messages with its offset that is greater than or equal to the offset from request message
- Send the response message with found messages

For commit_offsets RPC handler,

- Receive the request message that has map of key and offset
- Save this information
- Send the response message

For list_committed_offsets RPC handler,

- Receive the request message that has list of keys
- Find the committed offset for each key in the request message
- Send the response message with found offsets

#### Data structure

I need to store three things

- keys and its offset
- keys and its messages with offset
- keys and its committed offset

#### Concurrency

Even though this is single node, n.Run() calls your handlers concurrently. It spawns a new goroutine for each incoming message.
Two send messages can arrive at the same time and both try to write to keyOffset and keyEntry simultaneously.
Need a mutex to protect the shared maps.

Note that a concurrent read and write to a map at the same time will also crash with a fatal error, even if your read isn't used for updating.
Go's map has a built-in race detector and will panic with fatal error: concurrent map read and map write if any goroutine reads while another is writing,

## Problem

Problem Source: https://fly.io/dist-sys/5a/

In this challenge, you’ll need to implement a replicated log service similar to Kafka. Replicated logs are often used as a message bus or an event stream.

This challenge is broken up in multiple sections so that you can build out your system incrementally. First, we’ll start out with a single-node log system and then we’ll distribute it in later challenges.

### Specification

Your nodes will need to store an append-only log in order to handle the "kafka" workload. Each log is identified by a string key (e.g. "k1") and these logs contain a series of messages which are identified by an integer offset. These offsets can be sparse in that not every offset must contain a message.

Maelstrom will check to make sure several anomalies do not occur:

    Lost writes: for example, a client sees offset 10 but not offset 5.
    Monotonic increasing offsets: an offset for a log should always be increasing.

There are no recency requirements so acknowledged send messages do not need to return in poll messages immediately.

### RPC: send

This message requests that a "msg" value be appended to a log identified by "key". Your node will receive a request message body that looks like this:

{
"type": "send",
"key": "k1",
"msg": 123
}

In response, it should send an acknowledge with a send_ok message that contains the unique offset for the message in the log:

{
"type": "send_ok",
"offset": 1000
}

### RPC: poll

This message requests that a node return messages from a set of logs starting from the given offset in each log. Your node will receive a request message body that looks like this:

{
"type": "poll",
"offsets": {
"k1": 1000,
"k2": 2000
}
}

In response, it should return a poll_ok message with messages starting from the given offset for each log. Your server can choose to return as many messages for each log as it chooses:

{
"type": "poll_ok",
"msgs": {
"k1": [[1000, 9], [1001, 5], [1002, 15]],
"k2": [[2000, 7], [2001, 2]]
}
}

### RPC: commit_offsets

This message informs the node that messages have been successfully processed up to and including the given offset. Your node will receive a request message body that looks like this:

{
"type": "commit_offsets",
"offsets": {
"k1": 1000,
"k2": 2000
}
}

In this example, the messages have been processed up to and including offset 1000 for log k1 and all messages up to and including offset 2000 for k2.

In response, your node should return a commit_offsets_ok message body to acknowledge the request:

{
"type": "commit_offsets_ok"
}

### RPC: list_committed_offsets

This message returns a map of committed offsets for a given set of logs. Clients use this to figure out where to start consuming from in a given log.

Your node will receive a request message body that looks like this:

{
"type": "list_committed_offsets",
"keys": ["k1", "k2"]
}

In response, your node should return a list_committed_offsets_ok message body containing a map of offsets for each requested key. Keys that do not exist on the node can be omitted.

{
"type": "list_committed_offsets_ok",
"offsets": {
"k1": 1000,
"k2": 2000
}
}

### Evaluation

Build your Go binary as maelstrom-kafka and run it against Maelstrom with the following command:

./maelstrom test -w kafka --bin ~/go/bin/maelstrom-kafka --node-count 1 --concurrency 2n --time-limit 20 --rate 1000

This will run a single node for 20 seconds with two clients. It will validate that messages are queued and committed properly.

If you’re successful, wahoo! Continue on to the Multi-Node Kafka challenge. If you’re having trouble, jump over to the Fly.io Community forum for help.
