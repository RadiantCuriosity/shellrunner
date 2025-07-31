package main

import (
	"log"
	"net"
	"net/rpc/jsonrpc"
	"os"
	"testing"
)

// NOTE: This is a stress test and requires the shellrunner server to be running.
//
// 1. Start the server in another terminal:
//    go build && ./shellrunner
//
// 2. Run the benchmark from the project root:
//    go test -bench=. -benchmem

func BenchmarkCommandIssuing(b *testing.B) {
	socketPath := "/tmp/shellrunner.sock"
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		b.Fatalf("shellrunner server is not running at %s. Please start it before running the benchmark.", socketPath)
	}

	b.ReportAllocs()
	b.ResetTimer()

	// Run the benchmark in parallel to simulate multiple concurrent clients.
	b.RunParallel(func(pb *testing.PB) {
		// Each parallel goroutine gets its own persistent client connection.
		client, err := net.Dial("unix", socketPath)
		if err != nil {
			log.Printf("dialing error in benchmark goroutine: %v", err)
			return
		}
		defer client.Close()

		c := jsonrpc.NewClient(client)
		var reply string

		// The loop continues until the benchmark is complete.
		for pb.Next() {
			// The command "true" is extremely fast, so we are primarily measuring
			// the overhead of the server's connection and request handling.
			err := c.Call("ShellRunner.Background", "true", &reply)
			if err != nil {
				log.Printf("rpc error during benchmark: %v", err)
			}
			if reply == "" {
				log.Printf("did not receive a job id during benchmark")
			}
		}
	})
}
