package main

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	maelstrom "github.com/jepsen-io/maelstrom/demo/go"
)

type version struct {
	Counter int    `json:"counter"`
	OpIndex int    `json:"-"` // position of this write within its txn's op list; not sent over the wire, reconstructed locally from array position on both sender and receiver
	NodeID  string `json:"node_id"`
}

func (v version) greaterThan(other version) bool {
	if v.Counter != other.Counter {
		return v.Counter > other.Counter
	}
	if v.OpIndex != other.OpIndex {
		return v.OpIndex > other.OpIndex
	}
	return v.NodeID > other.NodeID
}

var (
	mu            sync.Mutex
	keyValueStore = make(map[int]int)
	keyVersion    = make(map[int]version)
	localCounter  int
)

// applyWrite only overwrites the stored value if v is newer than the key's current version,
// so retried/out-of-order replicate messages can never clobber a newer write with a stale one.
// Caller must hold mu.
func applyWrite(key int, value int, v version) {
	// key never written or the incoming version v outranks whatever's currently stored
	if current, ok := keyVersion[key]; !ok || v.greaterThan(current) {
		keyValueStore[key] = value
		keyVersion[key] = v
	}
}

func fanOutReplicate(n *maelstrom.Node, txn [][3]any, v version) {
	replicateBody := map[string]any{"type": "replicate", "txn": txn, "version": v}

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
			Type string   `json:"type"`
			Txn  [][3]any `json:"txn"`
		}

		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}

		mu.Lock()
		localCounter++
		baseVersion := version{Counter: localCounter, NodeID: n.ID()}

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
				// OpIndex = i ensures a later write to the same key within this
				// same txn always outranks an earlier one, even though they share Counter/NodeID.
				v := baseVersion
				v.OpIndex = i
				applyWrite(key, int(op[2].(float64)), v)
			}
		}
		mu.Unlock()

		// replicate to other nodes
		fanOutReplicate(n, body.Txn, baseVersion)

		return n.Reply(msg, map[string]any{
			"type": "txn_ok",
			"txn":  body.Txn,
		})
	})

	n.Handle("replicate", func(msg maelstrom.Message) error {

		var body struct {
			Type    string   `json:"type"`
			Txn     [][3]any `json:"txn"`
			Version version  `json:"version"`
		}

		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}

		mu.Lock()
		for i, op := range body.Txn {
			operation := op[0].(string)
			key := int(op[1].(float64))

			if operation == "w" {
				// same array, same order as the sender iterated, so index i
				// reconstructs the same OpIndex the sender used for this op.
				v := body.Version
				v.OpIndex = i
				applyWrite(key, int(op[2].(float64)), v)
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
