// Command verifywshub is a minimal WebSocket test client used by
// server/scripts/verify_redis_hub.sh (Step 31's multi-instance MessageHub
// smoke check, docs/tasks/step31.md). It is not part of the API server
// binary; it exists solely so the shell verification script can wait for a
// single room event frame without depending on an external tool (e.g.
// websocat) that may not be installed.
//
// Usage:
//
//	go run ./cmd/verifywshub -url <ws-url-with-ticket-query-param> -timeout 15s
//
// It connects to the given WebSocket URL (a GET /rooms/:roomId/ws URL
// already carrying a valid ?ticket= query parameter, matching
// interface/handler/websocket_handler.go's Handle contract), waits for the
// first frame the server pushes, prints it to stdout, and exits 0. If the
// connection fails or no frame arrives before the timeout elapses, it prints
// the error to stderr and exits 1, which the calling shell script treats as
// verification failure.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func main() {
	os.Exit(run())
}

// run implements main's logic and returns a process exit code, so defer'd
// cleanup (closing the WebSocket connection) reliably executes before the
// process exits, which a direct os.Exit call in main would skip.
func run() int {
	url := flag.String("url", "", "WebSocket URL to connect to, including the ticket query parameter (required)")
	timeout := flag.Duration("timeout", 15*time.Second, "how long to wait for a single event frame before failing")
	flag.Parse()

	if *url == "" {
		fmt.Fprintln(os.Stderr, "verifywshub: -url is required")
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, *url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "verifywshub: dial failed: %v\n", err)
		return 1
	}
	defer func() { _ = conn.CloseNow() }()

	var frame map[string]any
	if err := wsjson.Read(ctx, conn, &frame); err != nil {
		fmt.Fprintf(os.Stderr, "verifywshub: read failed (timed out waiting for an event frame): %v\n", err)
		return 1
	}

	encoded, err := json.Marshal(frame)
	if err != nil {
		fmt.Fprintf(os.Stderr, "verifywshub: marshal received frame failed: %v\n", err)
		return 1
	}

	fmt.Printf("RECEIVED: %s\n", encoded)
	_ = conn.Close(websocket.StatusNormalClosure, "")
	return 0
}
