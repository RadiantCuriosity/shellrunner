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

func main() {
	// Basic command-line argument validation.
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run client/main.go <method> [args...]")
		fmt.Println("Methods: run, background, status, output")
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
			log.Fatal("Usage: go run client/main.go output <job_id>")
		}
		var reply map[string]interface{}
		callErr = c.Call("ShellRunner.Output", os.Args[2], &reply)
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
