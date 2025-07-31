// Package main implements a JSON-RPC server that executes shell commands.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"os/exec"
	"sync"
	"time"
)

// BackgroundJob represents a command running in the background.
type BackgroundJob struct {
	Command   string
	Cmd       *exec.Cmd
	Stdout    bytes.Buffer
	Stderr    bytes.Buffer
	StartTime time.Time
	EndTime   time.Time
	Status    string // "running", "exited", "errored"
	ExitCode  int
}

var (
	// jobs stores all background jobs, keyed by their unique ID.
	jobs = make(map[string]*BackgroundJob)
	// jobCounter is used to generate sequential job IDs.
	jobCounter uint64
	// mutex protects access to the jobs map and the jobCounter.
	mutex = &sync.Mutex{}
	// logger is used for optional logging.
	logger *log.Logger
)

// ShellRunner is the receiver for the RPC methods.
type ShellRunner struct{}

// Run executes a command synchronously and returns its output and exit code.
func (s *ShellRunner) Run(cmd string, reply *map[string]interface{}) error {
	logger.Printf("Run called with command: %q", cmd)
	command := exec.Command("bash", "-c", cmd)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	err := command.Run()

	(*reply)["stdout"] = stdout.String()
	(*reply)["stderr"] = stderr.String()

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			(*reply)["exit_code"] = exitError.ExitCode()
		} else {
			// For other errors (e.g., command not found), we still want to return
			// the captured output, so we don't return the error directly.
			(*reply)["exit_code"] = -1
		}
	} else {
		(*reply)["exit_code"] = 0
	}

	logger.Printf("Run finished for command: %q", cmd)
	return nil
}

// Background executes a command asynchronously, returning a unique job ID.
func (s *ShellRunner) Background(cmd string, reply *string) error {
	logger.Printf("Background called with command: %q", cmd)
	mutex.Lock()
	defer mutex.Unlock()

	jobCounter++
	id := fmt.Sprintf("%08x", jobCounter)
	command := exec.Command("bash", "-c", cmd)

	job := &BackgroundJob{
		Command:   cmd,
		Cmd:       command,
		StartTime: time.Now(),
		Status:    "running",
	}
	command.Stdout = &job.Stdout
	command.Stderr = &job.Stderr

	jobs[id] = job

	// Run the command in a goroutine to make it non-blocking.
	go func(job *BackgroundJob) {
		logger.Printf("Starting background job %s: %s", id, cmd)
		err := job.Cmd.Run()
		mutex.Lock()
		defer mutex.Unlock()

		job.EndTime = time.Now()
		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				job.ExitCode = exitError.ExitCode()
				job.Status = "exited"
			} else {
				job.Status = "errored"
				job.ExitCode = -1
			}
		} else {
			job.Status = "exited"
			job.ExitCode = 0
		}
		logger.Printf("Background job %s finished with status %s and exit code %d", id, job.Status, job.ExitCode)
	}(job)

	*reply = id
	return nil
}

// Status returns the current status and execution time of a background job.
func (s *ShellRunner) Status(id string, reply *map[string]interface{}) error {
	logger.Printf("Status called for job ID: %s", id)
	mutex.Lock()
	defer mutex.Unlock()

	job, ok := jobs[id]
	if !ok {
		return fmt.Errorf("job with id %s not found", id)
	}

	(*reply)["command"] = job.Command
	(*reply)["status"] = job.Status
	(*reply)["start_time"] = job.StartTime.Format(time.RFC3339)

	var duration float64
	if job.Status == "running" {
		duration = time.Since(job.StartTime).Seconds()
	} else {
		duration = job.EndTime.Sub(job.StartTime).Seconds()
	}
	(*reply)["duration_seconds"] = duration

	return nil
}

// OutputArgs defines the arguments for the Output method.
type OutputArgs struct {
	ID      string
	Release bool
}

// Output returns the stdout and stderr of a background job.
func (s *ShellRunner) Output(args OutputArgs, reply *map[string]interface{}) error {
	logger.Printf("Output called for job ID: %s, Release: %t", args.ID, args.Release)
	mutex.Lock()
	defer mutex.Unlock()

	job, ok := jobs[args.ID]
	if !ok {
		return fmt.Errorf("job with id %s not found", args.ID)
	}

	(*reply)["stdout"] = job.Stdout.String()
	(*reply)["stderr"] = job.Stderr.String()

	if args.Release {
		logger.Printf("Releasing job %s", args.ID)
		delete(jobs, args.ID)
	}

	return nil
}

// Release removes a job's data from memory.
func (s *ShellRunner) Release(id string, reply *bool) error {
	logger.Printf("Release called for job ID: %s", id)
	mutex.Lock()
	defer mutex.Unlock()

	if _, ok := jobs[id]; !ok {
		return fmt.Errorf("job with id %s not found", id)
	}

	delete(jobs, id)
	*reply = true
	logger.Printf("Released job %s", id)
	return nil
}

// ReleaseAll removes all finished jobs from memory.
func (s *ShellRunner) ReleaseAll(args struct{}, reply *int) error {
	logger.Println("ReleaseAll called")
	mutex.Lock()
	defer mutex.Unlock()

	releasedCount := 0
	for id, job := range jobs {
		if job.Status == "exited" || job.Status == "errored" {
			delete(jobs, id)
			releasedCount++
		}
	}
	*reply = releasedCount
	logger.Printf("Released %d finished jobs", releasedCount)
	return nil
}

// List returns a list of all job IDs.
func (s *ShellRunner) List(args struct{}, reply *[]string) error {
	logger.Printf("List called")
	mutex.Lock()
	defer mutex.Unlock()

	ids := make([]string, 0, len(jobs))
	for id := range jobs {
		ids = append(ids, id)
	}

	*reply = ids
	return nil
}

func main() {
	// Setup command-line flags.
	logging := flag.Bool("logging", false, "Enable logging to stdout.")
	flag.Parse()

	// Setup logging.
	if *logging || os.Getenv("SHELLRUNNER_LOGGING") == "true" {
		logger = log.New(os.Stdout, "[shellrunner] ", log.LstdFlags)
	} else {
		// Discard logs if not enabled.
		logger = log.New(io.Discard, "", 0)
	}

	logger.Println("Server starting...")

	shellRunner := new(ShellRunner)
	rpc.Register(shellRunner)

	socketPath := "/tmp/shellrunner.sock"
	// Ensure the socket from a previous run is removed.
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		log.Fatalf("failed to remove old socket: %v", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		logger.Fatalf("Error listening: %v", err)
	}
	defer listener.Close()

	logger.Println("Server listening on", socketPath)

	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Printf("Error accepting connection: %v", err)
			continue
		}
		logger.Printf("Accepted new connection from %s", conn.RemoteAddr().String())
		// Handle each connection in a new goroutine.
		go jsonrpc.ServeConn(conn)
	}
}
