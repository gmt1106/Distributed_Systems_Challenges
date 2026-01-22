package main

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	maelstrom "github.com/jepsen-io/maelstrom/demo/go"
)

func main() {
	n := maelstrom.NewNode()

	var (
		mu       sync.Mutex 
		messages = make(map[int]struct{})
		neighbors []string
	)

	go func() {
		ticker := time.NewTicker(300 * time.Millisecond)
		defer ticker.Stop() 

		for range ticker.C {

			mu.Lock()
			myNeighbors := neighbors
			messagesCopy := make([]int, 0, len(messages))
			for k := range messages {
				messagesCopy = append(messagesCopy, k)
			}
			mu.Unlock()

			for _, neighbor := range myNeighbors { 
				n.Send(neighbor, map[string]any{
					"type": "broadcast",
					"messages": messagesCopy,
				})
			}
		}
	}()

	n.Handle("broadcast", func(msg maelstrom.Message) error {

		var body struct {
			Type     string `json:"type"`
			Message  int    `json:"message"`
			Messages []int  `json:"messages"`
			MsgID    *int   `json:"msg_id,omitempty"`
		}

		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}
 
		mu.Lock()
		if (len(body.Messages) > 0) {
			for _, v := range body.Messages {
				messages[v] = struct{}{}
			}
		} else {
			messages[body.Message] = struct{}{}
		}
		mu.Unlock() 

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
