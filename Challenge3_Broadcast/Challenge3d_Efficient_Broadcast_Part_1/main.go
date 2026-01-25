package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"sync"
	"time"

	maelstrom "github.com/jepsen-io/maelstrom/demo/go"
)
	const fanout = 3
	
	var (
		mu       sync.Mutex 
		messages = make(map[int]struct{})
		lastSent = make(map[string]map[int]struct{})
		n *maelstrom.Node
	)

func pickFanout(candidateNeighbors []string, fanoutCount int) []string {

	if len(candidateNeighbors) <= fanoutCount {
		return candidateNeighbors;
	}

	perm := rand.Perm(len(candidateNeighbors))
	picked := make([]string, 0, fanoutCount)
	for i := 0; i < fanoutCount; i++ {
		picked = append(picked, candidateNeighbors[perm[i]])
	}
	return picked
}

func fanoutMessages() {

	mu.Lock()
	// snapshot messages
	allMessagesCopy := make([]int, 0, len(messages))
	for m := range messages {
		allMessagesCopy = append(allMessagesCopy, m)
	}

	mu.Unlock()

	// randomly pick neighbours to communicate with
	var candidates []string
	for _, id := range n.NodeIDs() {
		if id != n.ID() {
			candidates = append(candidates, id)
		}
	}
	fanoutNeighbors := pickFanout(candidates, fanout)

	for _, neighbor := range fanoutNeighbors {

		mu.Lock()
        if lastSent[neighbor] == nil {
       		lastSent[neighbor] = make(map[int]struct{})
    	}
		neighborSentCopy := make(map[int]struct{}, len(lastSent[neighbor]))
		for k := range lastSent[neighbor] {
			neighborSentCopy[k] = struct{}{}
		}
    	mu.Unlock()

		// collect messages not yet sent to this neighbor
		var sendOutMessages []int
		for _, m := range allMessagesCopy {
			if _, ok := neighborSentCopy[m]; !ok {
				sendOutMessages = append(sendOutMessages, m)
			}
		}

		if len(sendOutMessages) > 0 {
			n.Send(neighbor, map[string]any{
				"type":     "broadcast_batch",
				"messages": sendOutMessages,
			})

			// update lastSent
			mu.Lock()
			for _, m := range sendOutMessages {
				lastSent[neighbor][m] = struct{}{}
			}
			mu.Unlock()
		}
	}
}

func main() {

	n = maelstrom.NewNode()

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop() 

		for range ticker.C {
		
			fanoutMessages()
		}
	}()

	n.Handle("broadcast", func(msg maelstrom.Message) error {

		var body struct {
			Type     string `json:"type"`
			Message  int    `json:"message"`
			MsgID    *int   `json:"msg_id,omitempty"`
		}

		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}

		mu.Lock()
		messages[body.Message] = struct{}{}
		mu.Unlock() 

		fanoutMessages()

		if body.MsgID != nil {
			return n.Reply(msg, map[string]any{
				"type": "broadcast_ok",
			})
		}
		return nil
	})

	n.Handle("broadcast_batch", func(msg maelstrom.Message) error {
		var body struct {
			Type     string `json:"type"`
			Messages []int  `json:"messages"`
		}

		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}

		mu.Lock()
		for _, m := range body.Messages {
			messages[m] = struct{}{}
		}
		mu.Unlock()

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

		return n.Reply(msg, map[string]any {
			"type": "topology_ok",
		})
	})

	if err := n.Run(); err != nil {
		log.Fatal(err)
	}
}
