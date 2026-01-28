package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	maelstrom "github.com/jepsen-io/maelstrom/demo/go"
)

func main() {
	n := maelstrom.NewNode()
	kv := maelstrom.NewSeqKV(n)
	ctx := context.Background()

	n.Handle("add", func(msg maelstrom.Message) error {
		
		var body struct {
			Type     string `json:"type"`
			Delta    int    `json:"delta"`
		}

		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}

		// Use a unique key for this node to avoid contention
		// This must be done inside the handler because n.ID() is not populated until the node is initialized
		// If this line is located outside the handler, it runs when your program starts, 
		// but before the node has received its init message from the Maelstrom test harness. 
		// At that early stage, n.ID() returns an empty string.
		key := fmt.Sprintf("counter_%s", n.ID())

		// infinite for loop
		for {
			cur, err := kv.ReadInt(ctx, key)
			// key does not exist yet
			if err != nil { 
				// try to make a counter
				err = kv.CompareAndSwap(ctx, key, 0, body.Delta, true)
				// counter is created
				if err == nil {
					break
				}
				// go back to the first line of for loop and try to read the counter again
				continue
			}
			// succesfully read the current counter value so now try to update the counter
			err = kv.CompareAndSwap(ctx, key, cur, cur+body.Delta, false)
			// counter is updated
			if err == nil {
				break
			}
			// go back to the first line of for loop and try to read the counter again
		}

		return n.Reply(msg, map[string]any{
			"type": "add_ok",
		})
	})

	n.Handle("read", func(msg maelstrom.Message) error {

		var total int
		for _, nodeID := range n.NodeIDs() {
			val, err := kv.ReadInt(ctx, fmt.Sprintf("counter_%s", nodeID))
			if err != nil {
				// counter does not exist for this node
				if rpcErr, ok := err.(*maelstrom.RPCError); ok && rpcErr.Code == maelstrom.KeyDoesNotExist {
					val = 0
				} else {
					return err
				}
			}
			total += val
		}

		return n.Reply(msg, map[string]any{
			"type": "read_ok",
			"value": total,
		})
	})

	if err := n.Run(); err != nil {
		log.Fatal(err)
	}


}