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

// ExecutionStatistics holds statistics about command executions.
type ExecutionStatistics struct {
	TotalCount    int64
	TotalDuration time.Duration
	MaxDuration   time.Duration
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
	// stats holds the execution statistics.
	stats      = &ExecutionStatistics{}
	statsMutex = &sync.Mutex{}
)

func updateStats(duration time.Duration) {
	statsMutex.Lock()
	defer statsMutex.Unlock()

	stats.TotalCount++
	stats.TotalDuration += duration
	if duration > stats.MaxDuration {
		stats.MaxDuration = duration
	}
}

// ShellRunner is the receiver for the RPC methods.
type ShellRunner struct{}

// RunArgs defines the arguments for the Run method.
type RunArgs struct {
	Command string
	Keep    bool
}

// Run executes a command synchronously and returns its output and exit code.
func (s *ShellRunner) Run(args RunArgs, reply *map[string]interface{}) error {
	logger.Printf("Run called with command: %q, Keep: %t", args.Command, args.Keep)
	command := exec.Command("bash", "-c", args.Command)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	startTime := time.Now()
	err := command.Run()
	endTime := time.Now()

	updateStats(endTime.Sub(startTime))

	(*reply)["stdout"] = stdout.String()
	(*reply)["stderr"] = stderr.String()

	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = -1
		}
	}
	(*reply)["exit_code"] = exitCode

	if args.Keep {
		mutex.Lock()
		defer mutex.Unlock()
		jobCounter++
		id := fmt.Sprintf("%d", jobCounter)
		job := &BackgroundJob{
			Command:   args.Command,
			Cmd:       command,
			Stdout:    stdout,
			Stderr:    stderr,
			StartTime: startTime,
			EndTime:   endTime,
			Status:    "exited",
			ExitCode:  exitCode,
		}
		jobs[id] = job
		(*reply)["job_id"] = id
		logger.Printf("Kept job %s for command: %q", id, args.Command)
	}

	logger.Printf("Run finished for command: %q", args.Command)
	return nil
}

// Background executes a command asynchronously, returning a unique job ID.
func (s *ShellRunner) Background(cmd string, reply *string) error {
	logger.Printf("Background called with command: %q", cmd)
	mutex.Lock()
	defer mutex.Unlock()

	jobCounter++
	id := fmt.Sprintf("%d", jobCounter)
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
		job.EndTime = time.Now()
		updateStats(job.EndTime.Sub(job.StartTime))

		mutex.Lock()
		defer mutex.Unlock()
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

// Statistics returns statistics about command executions.
func (s *ShellRunner) Statistics(args struct{}, reply *map[string]interface{}) error {
	logger.Println("Statistics called")
	statsMutex.Lock()
	defer statsMutex.Unlock()

	var avgDuration float64
	if stats.TotalCount > 0 {
		avgDuration = stats.TotalDuration.Seconds() / float64(stats.TotalCount)
	}

	(*reply)["total_count"] = stats.TotalCount
	(*reply)["average_duration_seconds"] = avgDuration
	(*reply)["max_duration_seconds"] = stats.MaxDuration.Seconds()

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
