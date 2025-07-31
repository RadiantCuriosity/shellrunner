package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

var serverCmd *exec.Cmd

// TestMain sets up and tears down the integration test environment.
func TestMain(m *testing.M) {
	// Build the server binary for testing.
	buildCmd := exec.Command("go", "build", "-o", "shellrunner_test")
	if err := buildCmd.Run(); err != nil {
		panic("failed to build server binary: " + err.Error())
	}
	defer os.Remove("shellrunner_test")

	// Start the server in a separate process group.
	serverCmd = exec.Command("./shellrunner_test")
	serverCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := serverCmd.Start(); err != nil {
		panic("failed to start server: " + err.Error())
	}

	// Give the server a moment to start up and create the socket.
	time.Sleep(100 * time.Millisecond)

	// Run the integration tests.
	code := m.Run()

	// Stop the server by killing its process group.
	if err := syscall.Kill(-serverCmd.Process.Pid, syscall.SIGKILL); err != nil {
		// It might have already exited, so don't panic.
	}
	serverCmd.Wait() // Clean up zombie processes.

	// Clean up the socket file.
	os.Remove("/tmp/shellrunner.sock")

	os.Exit(code)
}

// runClient is a helper function to execute the client CLI and parse its JSON output.
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

// TestIntegrationRun tests the synchronous "run" command.
func TestIntegrationRun(t *testing.T) {
	t.Run("without keep", func(t *testing.T) {
		reply := runClient(t, "run", `echo "hello integration"`)
		if reply["stdout"] != "hello integration\n" {
			t.Errorf("expected stdout 'hello integration\\n', got %q", reply["stdout"])
		}
		if reply["exit_code"].(float64) != 0 {
			t.Errorf("expected exit code 0, got %v", reply["exit_code"])
		}
		if _, ok := reply["job_id"]; ok {
			t.Error("did not expect a job_id when not using --keep")
		}
	})

	t.Run("with keep", func(t *testing.T) {
		reply := runClient(t, "run", `echo "kept"`, "--keep")
		jobID, ok := reply["job_id"].(string)
		if !ok || jobID == "" {
			t.Fatalf("did not get a valid job_id from run --keep: %v", reply)
		}

		// Verify the job can be queried via status
		statusReply := runClient(t, "status", jobID)
		if statusReply["command"] != `echo "kept"` {
			t.Errorf("kept job has wrong command: %q", statusReply["command"])
		}
		if statusReply["status"] != "exited" {
			t.Errorf("kept job has wrong status: %q", statusReply["status"])
		}

		// Clean up the kept job
		runClient(t, "release", jobID)
	})
}

// TestIntegrationBackgroundWorkflow tests the full asynchronous workflow.
func TestIntegrationBackgroundWorkflow(t *testing.T) {
	// 1. Start a background job.
	command := `sleep 0.3; echo "workflow done"`
	bgReply := runClient(t, "background", command)
	jobID, ok := bgReply["job_id"].(string)
	if !ok || jobID == "" {
		t.Fatalf("did not get a valid job_id from background command: %v", bgReply)
	}

	// 2. Check status while running.
	statusReply := runClient(t, "status", jobID)
	if statusReply["status"] != "running" {
		t.Errorf("expected status to be 'running', got %q", statusReply["status"])
	}
	if statusReply["command"] != command {
		t.Errorf("expected command to be %q, got %q", command, statusReply["command"])
	}
	if _, ok := statusReply["start_time"]; !ok {
		t.Error("expected status reply to have 'start_time'")
	}
	if _, ok := statusReply["duration_seconds"]; !ok {
		t.Error("expected status reply to have 'duration_seconds'")
	}

	// 3. Wait for it to finish.
	time.Sleep(400 * time.Millisecond)

	// 4. Check status after completion.
	finalStatusReply := runClient(t, "status", jobID)
	if finalStatusReply["status"] != "exited" {
		t.Errorf("expected status to be 'exited', got %q", finalStatusReply["status"])
	}
	if duration, ok := finalStatusReply["duration_seconds"].(float64); !ok || duration < 0.3 {
		t.Errorf("expected duration to be at least 0.3, got %v", duration)
	}

	// 5. Check the output and release the job.
	outputReply := runClient(t, "output", jobID, "--release")
	if outputReply["stdout"] != "workflow done\n" {
		t.Errorf("expected stdout 'workflow done\\n', got %q", outputReply["stdout"])
	}

	// 6. Verify the job was released by checking its status again.
	// The client should fail because the job doesn't exist.
	cmdArgs := append([]string{"run", "client/main.go"}, "status", jobID)
	cmd := exec.Command("go", cmdArgs...)
	_, err := cmd.Output()
	if err == nil {
		t.Fatalf("expected client command to fail for released job, but it succeeded")
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		if !strings.Contains(string(exitErr.Stderr), "not found") {
			t.Errorf("expected error message to contain 'not found', got %q", string(exitErr.Stderr))
		}
	} else {
		t.Fatalf("unexpected error type: %v", err)
	}
}

func TestIntegrationRelease(t *testing.T) {
	// 1. Start a background job.
	bgReply := runClient(t, "background", `sleep 1`)
	jobID, ok := bgReply["job_id"].(string)
	if !ok || jobID == "" {
		t.Fatalf("did not get a valid job_id from background command: %v", bgReply)
	}

	// 2. Release the job.
	releaseReply := runClient(t, "release", jobID)
	if released, ok := releaseReply["released"].(bool); !ok || !released {
		t.Errorf("expected release to be successful, got %v", releaseReply)
	}

	// 3. Verify the job was released.
	cmdArgs := append([]string{"run", "client/main.go"}, "status", jobID)
	cmd := exec.Command("go", cmdArgs...)
	_, err := cmd.Output()
	if err == nil {
		t.Fatalf("expected client command to fail for released job, but it succeeded")
	}
}

func TestIntegrationList(t *testing.T) {
	// 1. Start a couple of jobs
	bgReply1 := runClient(t, "background", `sleep 1`)
	jobID1, _ := bgReply1["job_id"].(string)
	bgReply2 := runClient(t, "background", `sleep 1`)
	jobID2, _ := bgReply2["job_id"].(string)

	// 2. List the jobs
	// The output of the client for a list is not a map, but a JSON array.
	// We need to handle this differently.
	cmdArgs := append([]string{"run", "client/main.go"}, "list")
	cmd := exec.Command("go", cmdArgs...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("client command failed: %v", err)
	}

	var ids []string
	if err := json.Unmarshal(out, &ids); err != nil {
		t.Fatalf("failed to unmarshal list output: %v", err)
	}

	if len(ids) != 2 {
		t.Errorf("expected 2 jobs in the list, got %d", len(ids))
	}

	found1, found2 := false, false
	for _, id := range ids {
		if id == jobID1 {
			found1 = true
		}
		if id == jobID2 {
			found2 = true
		}
	}
	if !found1 || !found2 {
		t.Errorf("did not find all job IDs in list reply")
	}

	// 3. Clean up by releasing the jobs
	runClient(t, "release", jobID1)
	runClient(t, "release", jobID2)
}

func TestIntegrationReleaseAll(t *testing.T) {
	// 1. Start a mix of jobs
	runClient(t, "background", `echo "finished"`)
	bgReply2 := runClient(t, "background", `sleep 2`)
	jobID2, _ := bgReply2["job_id"].(string)

	// 2. Wait for the first job to finish
	time.Sleep(100 * time.Millisecond)

	// 3. Release all finished jobs
	releaseReply := runClient(t, "release-all")
	if count, ok := releaseReply["released_count"].(float64); !ok || count != 1 {
		t.Errorf("expected to release 1 job, got %v", releaseReply)
	}

	// 4. Verify that the running job still exists and the finished one is gone
	cmdArgs := append([]string{"run", "client/main.go"}, "list")
	cmd := exec.Command("go", cmdArgs...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("client command failed: %v", err)
	}
	var ids []string
	if err := json.Unmarshal(out, &ids); err != nil {
		t.Fatalf("failed to unmarshal list output: %v", err)
	}

	if len(ids) != 1 {
		t.Fatalf("expected 1 job to remain, got %d", len(ids))
	}
	if ids[0] != jobID2 {
		t.Errorf("expected remaining job to be %s, got %s", jobID2, ids[0])
	}

	// 5. Clean up the remaining job
	runClient(t, "release", jobID2)
}

func resetClient(t *testing.T) {
	t.Helper()
	cmd := exec.Command("go", "run", "client/main.go", "reset")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to reset server state: %v", err)
	}
}

func TestIntegrationStatistics(t *testing.T) {
	// 1. Get the initial statistics
	initialStats := runClient(t, "statistics")
	initialCount := initialStats["total_count"].(float64)

	// 2. Run a few commands to generate stats
	runClient(t, "run", "sleep 0.1")
	runClient(t, "background", "sleep 0.2")
	time.Sleep(300 * time.Millisecond) // Ensure background job finishes

	// 3. Get the final statistics
	finalStats := runClient(t, "statistics")

	// 4. Verify that the count has increased
	newCount := finalStats["total_count"].(float64)
	if newCount <= initialCount {
		t.Errorf("expected total_count to increase, but it did not")
	}
}
