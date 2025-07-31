package main

import (
	"bytes"
	"fmt"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/google/uuid"
)

type BackgroundJob struct {
	Cmd       *exec.Cmd
	Stdout    bytes.Buffer
	Stderr    bytes.Buffer
	StartTime time.Time
	Status    string // "running", "exited", "errored"
	ExitCode  int
}

var (
	jobs  = make(map[string]*BackgroundJob)
	mutex = &sync.Mutex{}
)

type ShellRunner struct{}

func (s *ShellRunner) Run(cmd string, reply *map[string]interface{}) error {
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
			return err
		}
	} else {
		(*reply)["exit_code"] = 0
	}

	return nil
}

func (s *ShellRunner) Background(cmd string, reply *string) error {
	mutex.Lock()
	defer mutex.Unlock()

	id := uuid.New().String()
	command := exec.Command("bash", "-c", cmd)

	job := &BackgroundJob{
		Cmd:       command,
		StartTime: time.Now(),
		Status:    "running",
	}
	command.Stdout = &job.Stdout
	command.Stderr = &job.Stderr

	jobs[id] = job

	go func(job *BackgroundJob) {
		err := job.Cmd.Run()
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
	}(job)

	*reply = id
	return nil
}

func (s *ShellRunner) Status(id string, reply *map[string]interface{}) error {
	mutex.Lock()
	defer mutex.Unlock()

	job, ok := jobs[id]
	if !ok {
		return fmt.Errorf("job with id %s not found", id)
	}

	(*reply)["status"] = job.Status
	(*reply)["execution_time_seconds"] = time.Since(job.StartTime).Seconds()

	return nil
}

func (s *ShellRunner) Output(id string, reply *map[string]interface{}) error {
	mutex.Lock()
	defer mutex.Unlock()

	job, ok := jobs[id]
	if !ok {
		return fmt.Errorf("job with id %s not found", id)
	}

	(*reply)["stdout"] = job.Stdout.String()
	(*reply)["stderr"] = job.Stderr.String()

	return nil
}

func main() {
	shellRunner := new(ShellRunner)
	rpc.Register(shellRunner)

	socketPath := "/tmp/shellrunner.sock"
	os.Remove(socketPath) // Remove any old socket file

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		fmt.Println("Error listening:", err)
		return
	}
	defer listener.Close()

	fmt.Println("Server listening on", socketPath)

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}
		go jsonrpc.ServeConn(conn)
	}
}
