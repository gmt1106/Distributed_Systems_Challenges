package main

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	maelstrom "github.com/jepsen-io/maelstrom/demo/go"
)

var (
	mu    sync.Mutex
	keyValueStore = make(map[int]int)
)

func fanOutReplicate(n *maelstrom.Node, txn [][3]any) {
    replicateBody := map[string]any{"type": "replicate", "txn": txn}

	// replicate the message to all nodes except current one
    for _, dest := range n.NodeIDs() {
        if dest == n.ID() {
            continue
        }
        go replicateWithRetry(n, dest, replicateBody)
    }
}

func replicateWithRetry(n *maelstrom.Node, dest string, body any) {

	// keep quietly retrying until it eventually works
    for {
		// this code is tested under partitions with a total-availability requirement
		// so a call that can block forever is unacceptable
		// it gives you a bounded call you can retry, rather than one that might hang your goroutine indefinitely
        ctx, cancel := context.WithTimeout(context.Background(), time.Second)
        _, err := n.SyncRPC(ctx, dest, body)
        cancel()
        if err == nil {
            return
        }
		// It gives the partition time to actually change state
        time.Sleep(200 * time.Millisecond)
    }
}

func main() {
	n := maelstrom.NewNode()

	n.Handle("txn", func(msg maelstrom.Message) error {

		var body struct {
			Type string        `json:"type"`
			Txn  [][3]any      `json:"txn"`
		}

		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}

		mu.Lock()
		for i, op := range body.Txn {
			operation := op[0].(string)
			key := int(op[1].(float64))

			switch operation {
			case "r":
				if value, ok := keyValueStore[key]; ok {
					body.Txn[i][2] = value
				} else {
					body.Txn[i][2] = nil
				}
			case "w":
				keyValueStore[key] = int(op[2].(float64))
			}
		}
		mu.Unlock()

		// replicate to other nodes
		fanOutReplicate(n, body.Txn)

		return n.Reply(msg, map[string]any{
			"type": "txn_ok",
			"txn":  body.Txn,
		})
	})

	n.Handle("replicate", func(msg maelstrom.Message) error {

		var body struct {
			Type string        `json:"type"`
			Txn  [][3]any      `json:"txn"`
		}

		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}

		mu.Lock()
		for _, op := range body.Txn {
			operation := op[0].(string)
			key := int(op[1].(float64))

			if operation == "w" {
				keyValueStore[key] = int(op[2].(float64))
			}
		}
		mu.Unlock()

		return n.Reply(msg, map[string]any{
			"type": "replicate_ok",
		})
	})

	if err := n.Run(); err != nil {
		log.Fatal(err)
	}
}
