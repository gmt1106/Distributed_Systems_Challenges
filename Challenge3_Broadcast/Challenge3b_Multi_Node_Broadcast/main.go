package main

import (
	"encoding/json"
	"log"
	"sync"

	maelstrom "github.com/jepsen-io/maelstrom/demo/go"
)

func main() {
	n := maelstrom.NewNode()

	var (
		mu       sync.Mutex 
		messages = make(map[int]struct{})
		neighbors []string
	)

	n.Handle("broadcast", func(msg maelstrom.Message) error {

		var body struct {
			Type    string `json:"type"`
			Message int    `json:"message"`
			MsgID   *int   `json:"msg_id,omitempty"`
		}

		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}

		mu.Lock()
		_, exists := messages[body.Message];
		if !exists {
			messages[body.Message] = struct{}{}
		}
		mu.Unlock() // better to release lock as soon as possible 
		// don't do send() while holding lock

		if !exists {
			mu.Lock()
			myNeighbors := neighbors
			mu.Unlock()

			for _, neighbor := range myNeighbors {
				if neighbor == msg.Src {
					continue
				}
				n.Send(neighbor, map[string]any{
					"type": "broadcast",
					"message": body.Message,
				})
			}
		}

		if body.MsgID != nil {
			return n.Reply(msg, map[string]any{
				"type": "broadcast_ok",
			})
		}
		return nil
	})

	n.Handle("read", func(msg maelstrom.Message) error {

		mu.Lock()
		result := make([]int, 0, len(messages))
		for k := range messages {
			result = append(result, k)
		}
		mu.Unlock()

		return n.Reply(msg, map[string]any{
			"type": "read_ok",
			"messages": result,
		})
	})

	n.Handle("topology", func(msg maelstrom.Message) error {

		var body struct {
			Type     string              `json:"type"`
			Topology map[string][]string `json:"topology"`
		}

		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}

		mu.Lock()
		neighbors = body.Topology[n.ID()]
		mu.Unlock()

		return n.Reply(msg, map[string]any {
			"type": "topology_ok",
		})
	})

	if err := n.Run(); err != nil {
		log.Fatal(err)
	}
}
