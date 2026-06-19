package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"

	maelstrom "github.com/jepsen-io/maelstrom/demo/go"
)

type entry struct {
	Offset int
	Msg    int
}

var (

	// map to store offset for each key of messag
	keyOffset       = make(map[string]int)
	// map to store offset and message for each key
	keyEntry        = map[string][]entry{}
	// map to store the consumer read and acknowledged
	committedOffset = make(map[string]int)
)

func main() {
	n := maelstrom.NewNode()
	kv := maelstrom.NewLinKV(n)
	ctx := context.Background()

	n.Handle("send", func(msg maelstrom.Message) error {

		var body struct {
			Type    string  `json:"type"`
			Key     string  `json:"key"`
			Msg     int     `json:"msg"`
		}

		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}
		
		offsetKey := fmt.Sprintf("offset_%s", body.Key)
		entryKey := fmt.Sprintf("entry_%s", body.Key)
		foundCurOffset := 0


		for {
			// try to get offset by key
			curOffset, offsetErr := kv.ReadInt(ctx, offsetKey)
			// key does not exist yet, set to 0
			if offsetErr != nil {
				curOffset = 0
			}
			// increment offset
			offsetErr = kv.CompareAndSwap(ctx, offsetKey, curOffset, curOffset + 1, true)
			// offset is not incremented. Get a new one
			if offsetErr != nil {
				continue
			}

			// offset found!
			foundCurOffset = curOffset

			for {
				// try to get entry by key
				val, entryErr := kv.Read(ctx, entryKey)
				curEntry := []entry{}
				// key exist, convert val to Entry list
				if entryErr == nil {
					json.Unmarshal([]byte(val.(string)), &curEntry)
				}

				// add new entry with offset
				curEntry = append(curEntry, entry{Offset: foundCurOffset, Msg: body.Msg})
				bytes, _ := json.Marshal(curEntry)
				// update entry list
				entryErr = kv.CompareAndSwap(ctx, entryKey, val, string(bytes), true)

				// entry is added. Stop retry.
				if entryErr == nil {
					break
				}
			}
			break
		}

		return n.Reply(msg, map[string]any{
			"type": "send_ok",
			"offset": foundCurOffset,
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


		for key, offset := range body.Offsets {

			entryKey := fmt.Sprintf("entry_%s", key)
			
			val, entryErr := kv.Read(ctx, entryKey)
			curEntry := []entry{}

			// key exist, convert val to Entry list
			if entryErr == nil {
				json.Unmarshal([]byte(val.(string)), &curEntry)
			}

			// sort
			sort.Slice(curEntry, func(i, j int) bool {
    			return curEntry[i].Offset < curEntry[j].Offset
			})

			expected := offset

			for _, entry := range curEntry {
				
				if entry.Offset < offset {
        			continue
    			}
				if entry.Offset != expected {
        			break  // gap found, stop here
    			}
				msgs[key] = append(msgs[key], [2]int{entry.Offset, entry.Msg})
				expected++
			}
		}

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

		for key, offset := range body.Offsets {

			committedKey := fmt.Sprintf("committed_%s", key)

			committed, committedErr := kv.Read(ctx, committedKey)

			if committedErr != nil {
				committed = 0
			}

			if offset > committed.(int) {
				kv.Write(ctx, committedKey, offset)
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

		responseMap := make(map[string]any)
		responseMap["type"] = "list_committed_offsets_ok"

		filteredCommittedOffset := make(map[string]int)

		for _, key := range body.Keys {

			committedKey := fmt.Sprintf("committed_%s", key)

			committed, committedErr := kv.Read(ctx, committedKey)
			
			if committedErr != nil {
				committed = 0
			}

			filteredCommittedOffset[key] = committed.(int);
		}
		
		responseMap["offsets"] = filteredCommittedOffset;

		return n.Reply(msg, responseMap)

	})

	if err := n.Run(); err != nil {
		log.Fatal(err)
	}

}
