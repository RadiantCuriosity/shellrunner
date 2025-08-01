// Package main implements a client for the ShellRunner JSON-RPC server.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/rpc/jsonrpc"
	"os"
)

// RunArgs matches the server's argument struct for the Run method.
type RunArgs struct {
	Command string
	Keep    bool
}

// OutputArgs matches the server's argument struct for the Output method.
type OutputArgs struct {
	ID      string
	Release bool
}

func main() {
	// Define flags
	socketPath := flag.String("socket", os.Getenv("SHELLRUNNER_SOCKET_PATH"), "Path to the Unix socket. Defaults to SHELLRUNNER_SOCKET_PATH env var.")
	flag.Parse()

	args := flag.Args()

	// Basic command-line argument validation.
	if len(args) < 1 {
		fmt.Println("Usage: go run client/main.go [-socket /path/to/socket] <method> [args...]")
		fmt.Println("Methods: run, background, status, output, release, list, release-all, statistics, since")
		return
	}

	if *socketPath == "" {
		log.Fatal("Error: -socket flag or SHELLRUNNER_SOCKET_PATH environment variable must be set.")
	}

	// Connect to the server's unix socket.
	client, err := net.Dial("unix", *socketPath)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer client.Close()

	// Create a new JSON-RPC client.
	c := jsonrpc.NewClient(client)

	method := args[0]
	var result interface{}
	var callErr error

	// Dispatch the RPC call based on the command-line arguments.
	switch method {
	case "run":
		if len(args) < 2 {
			log.Fatal("Usage: ... run <command> [--keep]")
		}
		runArgs := RunArgs{Command: args[1]}
		if len(args) > 2 && args[2] == "--keep" {
			runArgs.Keep = true
		}
		var reply map[string]interface{}
		callErr = c.Call("ShellRunner.Run", runArgs, &reply)
		result = reply
	case "background":
		if len(args) < 2 {
			log.Fatal("Usage: ... background <command>")
		}
		var reply string
		callErr = c.Call("ShellRunner.Background", args[1], &reply)
		result = map[string]string{"job_id": reply}
	case "status":
		if len(args) < 2 {
			log.Fatal("Usage: ... status <job_id>")
		}
		var reply map[string]interface{}
		callErr = c.Call("ShellRunner.Status", args[1], &reply)
		result = reply
	case "output":
		if len(args) < 2 {
			log.Fatal("Usage: ... output <job_id> [--release]")
		}
		outputArgs := OutputArgs{ID: args[1]}
		if len(args) > 2 && args[2] == "--release" {
			outputArgs.Release = true
		}
		var reply map[string]interface{}
		callErr = c.Call("ShellRunner.Output", outputArgs, &reply)
		result = reply
	case "release":
		if len(args) < 2 {
			log.Fatal("Usage: ... release <job_id>")
		}
		var reply bool
		callErr = c.Call("ShellRunner.Release", args[1], &reply)
		result = map[string]bool{"released": reply}
	case "list":
		var reply []struct {
			ID     string
			Status string
		}
		callErr = c.Call("ShellRunner.List", struct{}{}, &reply)
		result = reply
	case "release-all":
		var reply int
		callErr = c.Call("ShellRunner.ReleaseAll", struct{}{}, &reply)
		result = map[string]int{"released_count": reply}
	case "statistics":
		var reply map[string]interface{}
		callErr = c.Call("ShellRunner.Statistics", struct{}{}, &reply)
		result = reply
	case "since":
		if len(args) < 2 {
			log.Fatal("Usage: ... since <job_id>")
		}
		var reply map[string]interface{}
		callErr = c.Call("ShellRunner.Since", args[1], &reply)
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
