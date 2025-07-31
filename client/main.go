// Package main implements a client for the ShellRunner JSON-RPC server.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/rpc/jsonrpc"
	"os"
)

// OutputArgs matches the server's argument struct for the Output method.
type OutputArgs struct {
	ID      string
	Release bool
}

func main() {
	// Basic command-line argument validation.
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run client/main.go <method> [args...]")
		fmt.Println("Methods: run, background, status, output, release, list")
		return
	}

	// Connect to the server's unix socket.
	client, err := net.Dial("unix", "/tmp/shellrunner.sock")
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer client.Close()

	// Create a new JSON-RPC client.
	c := jsonrpc.NewClient(client)

	method := os.Args[1]
	var result interface{}
	var callErr error

	// Dispatch the RPC call based on the command-line arguments.
	switch method {
	case "run":
		if len(os.Args) < 3 {
			log.Fatal("Usage: go run client/main.go run <command>")
		}
		var reply map[string]interface{}
		callErr = c.Call("ShellRunner.Run", os.Args[2], &reply)
		result = reply
	case "background":
		if len(os.Args) < 3 {
			log.Fatal("Usage: go run client/main.go background <command>")
		}
		var reply string
		callErr = c.Call("ShellRunner.Background", os.Args[2], &reply)
		result = map[string]string{"job_id": reply}
	case "status":
		if len(os.Args) < 3 {
			log.Fatal("Usage: go run client/main.go status <job_id>")
		}
		var reply map[string]interface{}
		callErr = c.Call("ShellRunner.Status", os.Args[2], &reply)
		result = reply
	case "output":
		if len(os.Args) < 3 {
			log.Fatal("Usage: go run client/main.go output <job_id> [--release]")
		}
		args := OutputArgs{ID: os.Args[2]}
		if len(os.Args) > 3 && os.Args[3] == "--release" {
			args.Release = true
		}
		var reply map[string]interface{}
		callErr = c.Call("ShellRunner.Output", args, &reply)
		result = reply
	case "release":
		if len(os.Args) < 3 {
			log.Fatal("Usage: go run client/main.go release <job_id>")
		}
		var reply bool
		callErr = c.Call("ShellRunner.Release", os.Args[2], &reply)
		result = map[string]bool{"released": reply}
	case "list":
		var reply []string
		callErr = c.Call("ShellRunner.List", struct{}{}, &reply)
		result = reply
	default:
		log.Fatalf("Unknown method: %s", method)
	}

	if callErr != nil {
		log.Fatalf("rpc error calling %s: %v", method, callErr)
	}

	// Pretty-print the JSON response.
	prettyJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatal("json marshal error:", err)
	}

	fmt.Printf("%s\n", prettyJSON)
}
