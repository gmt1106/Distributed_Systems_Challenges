package main

import (
	"encoding/json"
	"log"
	"sync"

	maelstrom "github.com/jepsen-io/maelstrom/demo/go"
)

type entry struct {
	offset int
	msg    int
}

var (
	mu              sync.Mutex
	// map to store offset for each key of messag
	keyOffset       = make(map[string]int)
	// map to store offset and message for each key
	keyEntry        = map[string][]entry{}
	// map to store the consumer read and acknowledged
	committedOffset = make(map[string]int)
)
	
func main() {
	n := maelstrom.NewNode()

	n.Handle("send", func(msg maelstrom.Message) error {

		var body struct {
			Type    string  `json:"type"`
			Key     string  `json:"key"`
			Msg     int     `json:"msg"`
		}

		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}
		
		mu.Lock()
		offset := keyOffset[body.Key]

		keyOffset[body.Key] = offset + 1
		keyEntry[body.Key] = append(keyEntry[body.Key], entry{offset, body.Msg})
		mu.Unlock()

		return n.Reply(msg, map[string]any{
			"type": "send_ok",
			"offset": offset,
		})
	})

	n.Handle("poll", func(msg maelstrom.Message) error {

		var body struct {
			Type        string          `json:"type"`
			Offsets     map[string]int  `json:"offsets"`
		}

		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}

		responseMap := make(map[string]any)
		responseMap["type"] = "poll_ok"
		
		msgs := map[string][][2]int{}

		mu.Lock()
		for key, offset := range body.Offsets {
			for _, entry := range keyEntry[key] {

				if entry.offset >= offset {
					msgs[key] = append(msgs[key], [2]int{entry.offset, entry.msg})
				}
			}
		}
		mu.Unlock()

		responseMap["msgs"] = msgs

		return n.Reply(msg, responseMap)

	})

	n.Handle("commit_offsets", func(msg maelstrom.Message) error {

		var body struct {
			Type        string          `json:"type"`
			Offsets     map[string]int  `json:"offsets"`
		}

		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}

		mu.Lock()
		for key, offset := range body.Offsets {
			if offset > committedOffset[key] {
				committedOffset[key] = offset
			}
		}
		mu.Unlock()

		return n.Reply(msg, map[string]any{
			"type": "commit_offsets_ok",
		})

	})

	n.Handle("list_committed_offsets", func(msg maelstrom.Message) error {

		var body struct {
			Type     string      `json:"type"`
			Keys     []string    `json:"keys"`
		}

		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}

		responseMap := make(map[string]any)
		responseMap["type"] = "list_committed_offsets_ok"

		filteredCommittedOffset := make(map[string]int)

		mu.Lock()
		for _, key := range body.Keys {

			filteredCommittedOffset[key] = committedOffset[key];
		}
		mu.Unlock()
		
		responseMap["offsets"] = filteredCommittedOffset;

		return n.Reply(msg, responseMap)

	})

	if err := n.Run(); err != nil {
		log.Fatal(err)
	}

}
