package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

var serverCmd *exec.Cmd

func TestMain(m *testing.M) {
	// Build the server binary
	buildCmd := exec.Command("go", "build", "-o", "shellrunner_test")
	if err := buildCmd.Run(); err != nil {
		panic("failed to build server binary: " + err.Error())
	}
	defer os.Remove("shellrunner_test")

	// Start the server
	serverCmd = exec.Command("./shellrunner_test")
	// Start the command in a new process group
	serverCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := serverCmd.Start(); err != nil {
		panic("failed to start server: " + err.Error())
	}

	// Give the server a moment to start up and create the socket
	time.Sleep(100 * time.Millisecond)

	// Run the tests
	code := m.Run()

	// Stop the server by killing the process group
	if err := syscall.Kill(-serverCmd.Process.Pid, syscall.SIGKILL); err != nil {
		// It might have already exited, so don't panic
		// panic("failed to kill server process group: " + err.Error())
	}
	serverCmd.Wait() // Clean up zombie processes

	// Clean up the socket file
	os.Remove("/tmp/shellrunner.sock")

	os.Exit(code)
}

func runClient(t *testing.T, args ...string) map[string]interface{} {
	t.Helper()
	cmdArgs := append([]string{"run", "client/main.go"}, args...)
	cmd := exec.Command("go", cmdArgs...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("client command failed with args %v: %s\n%s", args, err, string(exitErr.Stderr))
		}
		t.Fatalf("client command failed with args %v: %s", args, err)
	}

	var reply map[string]interface{}
	if err := json.Unmarshal(out, &reply); err != nil {
		t.Fatalf("failed to unmarshal client output: %s\nOutput was: %s", err, string(out))
	}
	return reply
}

func TestIntegrationRun(t *testing.T) {
	reply := runClient(t, "run", `echo "hello integration"`)
	if reply["stdout"] != "hello integration\n" {
		t.Errorf("expected stdout 'hello integration\\n', got %q", reply["stdout"])
	}
	if reply["exit_code"].(float64) != 0 {
		t.Errorf("expected exit code 0, got %v", reply["exit_code"])
	}
}

func TestIntegrationBackgroundWorkflow(t *testing.T) {
	// 1. Start a background job
	bgReply := runClient(t, "background", `sleep 0.3; echo "workflow done"`)
	jobID, ok := bgReply["job_id"].(string)
	if !ok || jobID == "" {
		t.Fatalf("did not get a valid job_id from background command: %v", bgReply)
	}

	// 2. Check status while running
	statusReply := runClient(t, "status", jobID)
	if statusReply["status"] != "running" {
		t.Errorf("expected status to be 'running', got %q", statusReply["status"])
	}

	// 3. Wait for it to finish
	time.Sleep(400 * time.Millisecond)

	// 4. Check status after completion
	finalStatusReply := runClient(t, "status", jobID)
	if finalStatusReply["status"] != "exited" {
		t.Errorf("expected status to be 'exited', got %q", finalStatusReply["status"])
	}

	// 5. Check the output
	outputReply := runClient(t, "output", jobID)
	if outputReply["stdout"] != "workflow done\n" {
		t.Errorf("expected stdout 'workflow done\\n', got %q", outputReply["stdout"])
	}
}
