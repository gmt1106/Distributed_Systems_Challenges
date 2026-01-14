# Challenge1: Echo

## How to Run

**How to set up dependencies**

1. Donwload depednecies that are need to run Maelstorm in Installing Maelstrom step

- `bash brew install openjdk graphviz gnuplot`

2. Donwlaod Maelstorm executable

- [Maelstrom 0.2.3](https://github.com/jepsen-io/maelstrom/releases/tag/v0.2.3)

3. Download dependecies that are need to compile code

- ```bash
    go get github.com/jepsen-io/maelstrom/demo/go
  ```

4. Build the Challenge1_Echo binary and place it in $GOBIN (typically ~/go/bin)

- ```bash
    go install .
  ```

**How to execuate the code with Maelstorm**

1. Execute Maelstorm with the path to the Challenge1_Echo binary

- ```bash
    ./maelstrom/maelstrom test -w echo --bin ~/go/bin/Challenge1_Echo --node-count 1 --time-limit 10
  ```
- single node, run for 10 secs

## Problem

Problem Source: https://fly.io/dist-sys/1/

Our first challenge is more of a “getting started” guide" to get the hang of working with Maelstrom in Go. In Maelstrom, we create a node which is a binary that receives JSON messages from STDIN and sends JSON messages to STDOUT. You can find a full protocol specification on the Maelstrom project.

We’ve created a Maelstrom Go library which provides maelstrom.Node that handles all this boilerplate for you. It lets you register handler functions for each message type—similar to how http.Handler works in the standard library.

### Specification

In this challenge, your node will receive an "echo" message that looks like this from Maelstrom's [Echo](https://github.com/jepsen-io/maelstrom/blob/main/doc/workloads.md#workload-echo) workload:

```json
{
  "src": "c1",
  "dest": "n1",
  "body": {
    "type": "echo",
    "msg_id": 1,
    "echo": "Please echo 35"
  }
}
```

Nodes & clients are sequentially numbered (e.g. n1, n2, etc). Nodes are prefixed with "n" and external clients are prefixed with "c". Message IDs are unique per source node but that is handled automatically by the Go library.

Your job is to send a message with the same body back to the client but with a message type of "echo_ok". It should also associate itself with the original message by setting the "in_reply_to" field to the original message ID. This reply field is handled automatically if you use the Node.Reply() method.

It should look something like:

```json
{
  "src": "n1",
  "dest": "c1",
  "body": {
    "type": "echo_ok",
    "msg_id": 1,
    "in_reply_to": 1,
    "echo": "Please echo 35"
  }
}
```

### Implementing a node

For this first challenge, we’ll walk you through how to implement the echo program. First, create a directory for your binary called maelstrom-echo and initialize a Go module for it:

```bash
$ mkdir maelstrom-echo
$ cd maelstrom-echo
$ go mod init maelstrom-echo
$ go mod tidy
```

Then create your main.go file in this directory. This should start with the main package declaration and a few imports:

package main

```go
import (
    "encoding/json"
    "log"

    maelstrom "github.com/jepsen-io/maelstrom/demo/go"
)
```

Inside our main() function, we’ll start by instantiating our node type:

```go
n := maelstrom.NewNode()
```

From here, we can register a handler callback function for our “echo” message. This function accepts a maelstrom.Message which contains the source and destination nodes for the message as well as the body content.

```go
n.Handle("echo", func(msg maelstrom.Message) error {
    // Unmarshal the message body as an loosely-typed map.
    var body map[string]any
    if err := json.Unmarshal(msg.Body, &body); err != nil {
        return err
    }

    // Update the message type to return back.
    body["type"] = "echo_ok"

    // Echo the original message back with the updated message type.
    return n.Reply(msg, body)
})
```

In this handler, we’re unmarshaling to a generic map since we simply want to echo back the same message we received. The Reply() method will automatically set the source and destination fields in the return message and it will associate the message as a reply to the original one received.

Finally, we’ll delegate execution to the Node by calling its Run() method. This method continuously reads messages from STDIN and fires off a goroutine for each one to the associated handler. If no handler exists for a message type, Run() will return an error.

```go
if err := n.Run(); err != nil {
    log.Fatal(err)
}
```

You can find a full implementation of this maelstrom-echo program in the Maelstrom codebase.

To compile our program, fetch the Maelstrom library and install:

```bash
go get github.com/jepsen-io/maelstrom/demo/go
go install .
```

This will build the maelstrom-echo binary and place it in your $GOBIN path which is typically ~/go/bin.

### Installing Maelstrom

Maelstrom is built in Clojure so you’ll need to install OpenJDK. It also provides some plotting and graphing utilities which rely on Graphviz & gnuplot. If you’re using Homebrew, you can install these with this command:

```bash
brew install openjdk graphviz gnuplot
```

You can find more details on the Prerequisites section on the Maelstrom docs.

Next, you’ll need to download Maelstrom itself. These challenges have been tested against the Maelstrom 0.2.3. Download the tarball & unpack it. You can run the maelstrom binary from inside this directory.

### Running our node in Maelstrom

We can now start up Maelstrom and pass it the full path to our binary:

```bash
./maelstrom test -w echo --bin ~/go/bin/maelstrom-echo --node-count 1 --time-limit 10
```

This command instructs maelstrom to run the "echo" workload against our binary. It runs a single node and it will send "echo" commands for 10 seconds.

Maelstrom will only inject network failures and it will not intentionally crash your node process so you don’t need to worry about persistence. You can use in-memory data structures for these challenges.

If everything ran correctly, you should see a bunch of log messages and stats and then finally a pleasent message from Maelstrom:

```bash
Everything looks good! ヽ(‘ー`)ノ
```

Success! If everything is working, move on to the Unique ID Generation challenge to build and test a distributed unique ID generator on your own.

If you’re not seeing this success message, head over to our Fly.io Community forum for some help.

### Debugging maelstrom

If your test fail, you can run the Maelstrom web server to view your results in more depth:

```bash
./maelstrom serve
```

You can then open a browser to http://localhost:8080 to see results. Consult the Maelstrom result documentation for further details.
