package main

import (
	"encoding/json"
	"log"
	"sync"

	maelstrom "github.com/jepsen-io/maelstrom/demo/go"
)

var (
	mu    sync.Mutex
	keyValueStore = make(map[int]int)
)

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

		return n.Reply(msg, map[string]any{
			"type": "txn_ok",
			"txn":  body.Txn,
		})
	})

	if err := n.Run(); err != nil {
		log.Fatal(err)
	}
}
