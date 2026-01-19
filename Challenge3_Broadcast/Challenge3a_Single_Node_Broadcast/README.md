# Challenge3a: Single-Node Broadcast

## How to Run

**How to set up dependencies**

1. Download dependecies that are need to compile code

- ```bash
    go get github.com/jepsen-io/maelstrom/demo/go
  ```

2. Build the Challenge3a_Single_Node_Broadcast binary and place it in $GOBIN (typically ~/go/bin)

- ```bash
    go install .
  ```

**How to execuate the code with Maelstorm**

1. Execute Maelstorm with the path to the Challenge1_Echo binary

- ```bash
    ./maelstrom/maelstrom test -w broadcast --bin ~/go/bin/Challenge3a_Single_Node_Broadcast --node-count 1 --time-limit 20 --rate 10
  ```
- single node, run for 20 secs

## Solution Explanation

### What the problem is really asking

In this challenge, I am building a single-node broadcast system, so I don't really need to worry about sending messages between nodes yet.

### Solution

#### Basic logic

For broadcast RPC handler,

- Receive and read the request message
- Store the integer number from the message body
- Send the reseponse message

For read RPC handler,

- Receive the request message
- Send the response message with all integers that I got from broadcast RPC

For toplogy RPC handler,

- Receive the request message
- Doesn't really need to do anything with the received data since this is single node broadcast system
- Send the response message

#### Memory

I need to store the integer I got from broadcast message.\
The best data structure would be set but goes does not have Set data structure.\
This is the idiomatic Go set:

```go
map[int]struct{}
```

This is a map with key as int and value as struct{} (empty struct).\
The reason that value type is empty struct is that it takes 0 bytes while signal presence.\
It is the most memory-efficient way to implement a set.

To handle concurrency of memory access, we must limit one goroutine to access to the map at a time.\
This requires a lock.

```go
sync.Mutex
```

## Problem

Problem Source: https://fly.io/dist-sys/3a/

In this challenge, you’ll need to implement a broadcast system that gossips messages between all nodes in the cluster. Gossiping is a common way to propagate information across a cluster when you don’t need strong consistency guarantees.

This challenge is broken up in multiple sections so that you can build out your system incrementally. First, we’ll start out with a single-node broadcast system. That may sound like an oxymoron but this lets us get our message handlers working correctly in isolation before trying to share messages between nodes.

### Specification

Your node will need to handle the "broadcast" workload which has 3 RPC message types: broadcast, read, & topology. Your node will need to store the set of integer values that it sees from broadcast messages so that they can be returned later via the read message RPC.

The Go library has two methods for sending messages:

1. Send() sends a fire-and-forget message and doesn’t expect a response. As such, it does not attach a message ID.

2. RPC() sends a message and accepts a response handler. The message will be decorated with a message ID so the handler can be invoked when a response message is received.

Data can be stored in-memory as node processes are not killed by Maelstrom.

#### RPC: broadcast

This message requests that a value be broadcast out to all nodes in the cluster. The value is always an integer and it is unique for each message from Maelstrom.

Your node will receive a request message body that looks like this:

```json
{
  "type": "broadcast",
  "message": 1000
}
```

It should store the "message" value locally so it can be read later. In response, it should send an acknowledge with a broadcast_ok message:

```json
{
  "type": "broadcast_ok"
}
```

#### RPC: read

This message requests that a node return all values that it has seen.

Your node will receive a request message body that looks like this:

```json
{
  "type": "read"
}
```

In response, it should return a read_ok message with a list of values it has seen:

```json
{
  "type": "read_ok",
  "messages": [1, 8, 72, 25]
}
```

The order of the returned values does not matter.

#### RPC: topology

This message informs the node of who its neighboring nodes are. Maelstrom has multiple topologies available or you can ignore this message and make your own topology from the list of nodes in the Node.NodeIDs() method. All nodes can communicate with each other regardless of the topology passed in.

Your node will receive a request message body that looks like this:

```json
{
  "type": "topology",
  "topology": {
    "n1": ["n2", "n3"],
    "n2": ["n1"],
    "n3": ["n1"]
  }
}
```

In response, your node should return a topology_ok message body:

```json
{
  "type": "topology_ok"
}
```

#### Evaluation

Build your Go binary as maelstrom-broadcast and run it against Maelstrom with the following command:

```bash
./maelstrom test -w broadcast --bin ~/go/bin/maelstrom-broadcast --node-count 1 --time-limit 20 --rate 10
```

This will run a single node for 20 seconds and send messages at the rate of 10 messages per second. It will validate that all values sent by broadcasts are returned via read.

If you’re successful, that’s awesome! Continue on to the Multi-Node Broadcast challenge. If you’re having trouble, jump over to the Fly.io Community forum for help.
