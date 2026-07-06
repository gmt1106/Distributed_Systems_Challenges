package main

import (
	"context"
	"encoding/json"
	"hash/fnv"
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
	localOffset            = make(map[string]int)
	// map to store offset and message for each key
	localEntry             = make(map[string][]entry)
	// map to store the consumer read and acknowledged
	localCommittedOffset   = make(map[string]int)
)

// func syncRPC(n *maelstrom.Node, dest string, body any) (maelstrom.Message, error) {
//   // Creates a buffered channel that can hold one message
//   // This is used to pass the response back from the async callback
//     ch := make(chan maelstrom.Message, 1)

//   // Sends the message to dest and registers a callback
//   // When the response arrives (in a different goroutine), the callback puts it into the channel
//   // <- is Go's channel operator
//   // Its direction indicates whether you're sending or receiving
//     if err := n.RPC(dest, body, func(resp maelstrom.Message) error {
//     // Sending into a channel (arrow points into channel):
//     // "Put resp into ch."
//     // The callback goroutine pushes the response into the channel
//         ch <- resp
//         return nil
//     });

//   err != nil {
//         return maelstrom.Message{}, err
//     }

//   // Receiving from a channel (arrow points away from channel):
//   // "Take a value out of ch."
//   // The current goroutine blocks here until something arrives in the channel, then returns it
//     return <-ch, nil
// }

func ownerOf(key string, nodeIDs []string) string {
	// Creates a FNV hash function. FNV is a fast, simple hash algorithm
    hashFun := fnv.New32a()
	// Feeds the key string into the hash function as bytes
    hashFun.Write([]byte(key))

	// Gets the 32-bit hash result as an integer, then takes modulo by the number of nodes
	// This produces a number between 0 and len(nodeIDs)-1
	// Uses that number as an index into the node list to pick the owner
    return nodeIDs[int(hashFun.Sum32())%len(nodeIDs)]
}

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

		offset := 0

		// check if this key belong to current node
		ownerNodeId := ownerOf(body.Key, n.NodeIDs())
		if ownerNodeId != n.ID() {
			// forward to owner, get response, relay to client
			resp, err := n.SyncRPC(context.Background(), ownerNodeId, map[string]any{
				"type": "send",
				"key":  body.Key,
				"msg":  body.Msg,
			})

			if err != nil {
				return err
			}

			// parse resp, reply to client
			var respBody map[string]any
			json.Unmarshal(resp.Body, &respBody)
			offset = int(respBody["offset"].(float64))

		} else {

			mu.Lock()
			offset = localOffset[body.Key]

			localOffset[body.Key] = offset + 1
			localEntry[body.Key] = append(localEntry[body.Key], entry{offset, body.Msg})
			mu.Unlock()

		}

		return n.Reply(msg, map[string]any{
			"type": "send_ok",
			"offset": offset,
		})

	})

	n.Handle("poll", func(msg maelstrom.Message) error {

		var body struct {
			Type    string         `json:"type"`
			Offsets map[string]int `json:"offsets"`
		}

		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}

		msgs := map[string][][2]int{}

		// group keys by owner
		ownerOffsets := map[string]map[string]int{}
		for key, offset := range body.Offsets {
			ownerNodeId := ownerOf(key, n.NodeIDs())
			if ownerNodeId != n.ID() {
				if ownerOffsets[ownerNodeId] == nil {
					ownerOffsets[ownerNodeId] = map[string]int{}
				}
				ownerOffsets[ownerNodeId][key] = offset
			} else {
				mu.Lock()
				for _, entry := range localEntry[key] {
					if entry.offset >= offset {
						msgs[key] = append(msgs[key], [2]int{entry.offset, entry.msg})
					}
				}
				mu.Unlock()
			}
		}

		// send one batched RPC per owner node
		for ownerNodeId, offsets := range ownerOffsets {
			resp, err := n.SyncRPC(context.Background(), ownerNodeId, map[string]any{
				"type":    "poll",
				"offsets": offsets,
			})
			if err != nil {
				return err
			}
			var respBody struct {
				Msgs map[string][][2]int `json:"msgs"`
			}
			if err := json.Unmarshal(resp.Body, &respBody); err != nil {
				return err
			}
			for key, pairs := range respBody.Msgs {
				msgs[key] = append(msgs[key], pairs...)
			}
		}

		return n.Reply(msg, map[string]any{
			"type": "poll_ok",
			"msgs": msgs,
		})

	})

	n.Handle("commit_offsets", func(msg maelstrom.Message) error {

		var body struct {
			Type        string          `json:"type"`
			Offsets     map[string]int  `json:"offsets"`
		}

		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}

		// group keys by owner and forward
    	ownerOffsets := map[string]map[string]int{}

		for key, offset := range body.Offsets {

			ownerNodeId := ownerOf(key, n.NodeIDs())
			if ownerNodeId != n.ID() {
				if ownerOffsets[ownerNodeId] == nil {
				ownerOffsets[ownerNodeId] = map[string]int{}
				}
				ownerOffsets[ownerNodeId][key] = offset
			} else {
				mu.Lock()
				if offset > localCommittedOffset[key] {
				localCommittedOffset[key] = offset
				}
				mu.Unlock()
			}
		}

		// forward the request to owner node
		for ownerNodeId, offsets := range ownerOffsets {
			_, err := n.SyncRPC(context.Background(), ownerNodeId, map[string]any{
				"type":    "commit_offsets",
				"offsets": offsets,
			})
			if err != nil {
				return err
			}
		}


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


		// committed offset that we found
		filteredCommittedOffset := make(map[string]int)

		// group keys by owner and forward
    	ownerKeys := map[string][]string{}

	
		for _, key := range body.Keys {

			ownerNodeId := ownerOf(key, n.NodeIDs())
			if ownerNodeId != n.ID() {
				ownerKeys[ownerNodeId] = append(ownerKeys[ownerNodeId], key)
			} else {
				mu.Lock()
				filteredCommittedOffset[key] = localCommittedOffset[key]
				mu.Unlock()
			}
		}

		// forward to key owner
		for ownerNodeId, keys := range ownerKeys {
			resp, err := n.SyncRPC(context.Background(), ownerNodeId, map[string]any{
				"type": "list_committed_offsets",
				"keys": keys,
			})
			if err != nil {
				return err
			}
			var respBody struct {
				Offsets map[string]int `json:"offsets"`
			}
			if err := json.Unmarshal(resp.Body, &respBody); err != nil {
				return err
			}
			for key, offset := range respBody.Offsets {
				filteredCommittedOffset[key] = offset
			}
		}
		
		return n.Reply(msg, map[string]any{
			"type":       "list_committed_offsets_ok",
			"offsets":    filteredCommittedOffset,
		})

	})

	if err := n.Run(); err != nil {
		log.Fatal(err)
	}

}
