package main

import (
	"io"
	"log"
	"testing"
	"time"
)

// setup is a helper function to reset the state of the jobs map before each test.
func setup(t *testing.T) {
	t.Helper()
	jobs = make(map[string]*BackgroundJob)
	jobCounter = 0
	logger = log.New(io.Discard, "", 0)
}

// TestRun contains unit tests for the Run method.
func TestRun(t *testing.T) {
	setup(t)
	shellRunner := new(ShellRunner)

	t.Run("successful command", func(t *testing.T) {
		reply := make(map[string]interface{})
		err := shellRunner.Run(`echo "hello world"`, &reply)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if reply["stdout"] != "hello world\n" {
			t.Errorf("expected stdout to be 'hello world\\n', got %q", reply["stdout"])
		}
		if reply["stderr"] != "" {
			t.Errorf("expected stderr to be empty, got %q", reply["stderr"])
		}
		if reply["exit_code"] != 0 {
			t.Errorf("expected exit code to be 0, got %v", reply["exit_code"])
		}
	})

	t.Run("command with stderr", func(t *testing.T) {
		reply := make(map[string]interface{})
		err := shellRunner.Run(`>&2 echo "error"`, &reply)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if reply["stdout"] != "" {
			t.Errorf("expected stdout to be empty, got %q", reply["stdout"])
		}
		if reply["stderr"] != "error\n" {
			t.Errorf("expected stderr to be 'error\\n', got %q", reply["stderr"])
		}
		if reply["exit_code"] != 0 {
			t.Errorf("expected exit code to be 0, got %v", reply["exit_code"])
		}
	})

	t.Run("command with non-zero exit code", func(t *testing.T) {
		reply := make(map[string]interface{})
		err := shellRunner.Run(`exit 123`, &reply)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if reply["exit_code"] != 123 {
			t.Errorf("expected exit code to be 123, got %v", reply["exit_code"])
		}
	})
}

// TestBackground contains unit tests for the Background method.
func TestBackground(t *testing.T) {
	setup(t)
	shellRunner := new(ShellRunner)
	var id string
	err := shellRunner.Background(`sleep 0.1; echo "done"`, &id)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if id != "00000001" {
		t.Fatalf("expected a job id '00000001', got %s", id)
	}

	// Allow time for the command to start
	time.Sleep(10 * time.Millisecond)

	mutex.Lock()
	job, ok := jobs[id]
	mutex.Unlock()

	if !ok {
		t.Fatalf("job with id %s not found in jobs map", id)
	}
	if job.Status != "running" {
		t.Errorf("expected job status to be 'running', got %s", job.Status)
	}

	// Wait for the job to finish
	time.Sleep(200 * time.Millisecond)

	mutex.Lock()
	if job.Status != "exited" {
		t.Errorf("expected job status to be 'exited', got %s", job.Status)
	}
	if job.ExitCode != 0 {
		t.Errorf("expected exit code to be 0, got %d", job.ExitCode)
	}
	if job.Stdout.String() != "done\n" {
		t.Errorf("expected stdout to be 'done\\n', got %q", job.Stdout.String())
	}
	mutex.Unlock()
}

// TestStatus contains unit tests for the Status method.
func TestStatus(t *testing.T) {
	setup(t)
	shellRunner := new(ShellRunner)
	command := "sleep 0.2"
	var id string
	err := shellRunner.Background(command, &id)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	reply := make(map[string]interface{})
	err = shellRunner.Status(id, &reply)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if reply["status"] != "running" {
		t.Errorf("expected status to be 'running', got %s", reply["status"])
	}
	if reply["command"] != command {
		t.Errorf("expected command to be %q, got %q", command, reply["command"])
	}
	if _, ok := reply["start_time"]; !ok {
		t.Error("expected status reply to have 'start_time'")
	}
	if _, ok := reply["duration_seconds"]; !ok {
		t.Error("expected status reply to have 'duration_seconds'")
	}

	time.Sleep(300 * time.Millisecond)

	err = shellRunner.Status(id, &reply)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if reply["status"] != "exited" {
		t.Errorf("expected status to be 'exited', got %s", reply["status"])
	}
	if duration, ok := reply["duration_seconds"].(float64); !ok || duration < 0.2 {
		t.Errorf("expected duration to be at least 0.2, got %v", duration)
	}
}

// TestOutput contains unit tests for the Output method.
func TestOutput(t *testing.T) {
	setup(t)
	shellRunner := new(ShellRunner)

	t.Run("without release", func(t *testing.T) {
		var id string
		err := shellRunner.Background(`echo "test"`, &id)
		if err != nil {
			t.Fatalf("background failed: %v", err)
		}
		time.Sleep(100 * time.Millisecond) // allow command to finish

		reply := make(map[string]interface{})
		err = shellRunner.Output(OutputArgs{ID: id, Release: false}, &reply)
		if err != nil {
			t.Fatalf("output failed: %v", err)
		}

		if reply["stdout"] != "test\n" {
			t.Errorf("expected stdout 'test\\n', got %q", reply["stdout"])
		}

		// Verify the job still exists
		mutex.Lock()
		_, ok := jobs[id]
		mutex.Unlock()
		if !ok {
			t.Error("job was released when it should not have been")
		}
	})

		t.Run("with release", func(t *testing.T) {
		var id string
		err := shellRunner.Background(`echo "test"`, &id)
		if err != nil {
			t.Fatalf("background failed: %v", err)
		}
		time.Sleep(100 * time.Millisecond) // allow command to finish

		reply := make(map[string]interface{})
		err = shellRunner.Output(OutputArgs{ID: id, Release: true}, &reply)
		if err != nil {
			t.Fatalf("output failed: %v", err)
		}

		if reply["stdout"] != "test\n" {
			t.Errorf("expected stdout 'test\\n', got %q", reply["stdout"])
		}

		// Verify the job was released
		mutex.Lock()
		_, ok := jobs[id]
		mutex.Unlock()
		if ok {
			t.Error("job was not released when it should have been")
		}
	})
}

// TestRelease contains unit tests for the Release method.
func TestRelease(t *testing.T) {
	setup(t)
	shellRunner := new(ShellRunner)
	var id string
	err := shellRunner.Background(`sleep 1`, &id)
	if err != nil {
		t.Fatalf("background failed: %v", err)
	}

	// Verify the job exists before releasing
	mutex.Lock()
	_, ok := jobs[id]
	mutex.Unlock()
	if !ok {
		t.Fatal("job was not created successfully")
	}

	var released bool
	err = shellRunner.Release(id, &released)
	if err != nil {
		t.Fatalf("release failed: %v", err)
	}
	if !released {
		t.Error("expected release to return true")
	}

	// Verify the job was released
	mutex.Lock()
	_, ok = jobs[id]
	mutex.Unlock()
	if ok {
		t.Error("job was not released")
	}

	// Verify that releasing a non-existent job returns an error
	err = shellRunner.Release(id, &released)
	if err == nil {
		t.Error("expected an error when releasing a non-existent job, but got nil")
	}
}

// TestReleaseAll contains unit tests for the ReleaseAll method.
func TestReleaseAll(t *testing.T) {
	setup(t)
	shellRunner := new(ShellRunner)

	// Create a mix of finished and running jobs
	var finishedID1, finishedID2, runningID string
	shellRunner.Background("echo 'finished 1'", &finishedID1)
	shellRunner.Background("echo 'finished 2'", &finishedID2)
	shellRunner.Background("sleep 1", &runningID)

	time.Sleep(100 * time.Millisecond) // Allow finished jobs to complete

	var releasedCount int
	err := shellRunner.ReleaseAll(struct{}{}, &releasedCount)
	if err != nil {
		t.Fatalf("ReleaseAll failed: %v", err)
	}

	if releasedCount != 2 {
		t.Errorf("expected to release 2 jobs, but released %d", releasedCount)
	}

	// Verify that only the running job remains
	mutex.Lock()
	defer mutex.Unlock()
	if len(jobs) != 1 {
		t.Errorf("expected 1 job to remain, but found %d", len(jobs))
	}
	if _, ok := jobs[runningID]; !ok {
		t.Errorf("running job with id %s was released", runningID)
	}
}

// TestList contains unit tests for the List method.
func TestList(t *testing.T) {
	setup(t)
	shellRunner := new(ShellRunner)

	// 1. Test with no jobs
	var reply []string
	err := shellRunner.List(struct{}{}, &reply)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(reply) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(reply))
	}

	// 2. Test with a few jobs
	var id1, id2 string
	shellRunner.Background("sleep 1", &id1)
	shellRunner.Background("sleep 1", &id2)

	err = shellRunner.List(struct{}{}, &reply)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(reply) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(reply))
	}

	// Check if the returned IDs are correct
	found1, found2 := false, false
	for _, id := range reply {
		if id == id1 {
			found1 = true
		}
		if id == id2 {
			found2 = true
		}
	}
	if !found1 || !found2 {
		t.Errorf("did not find all job IDs in list reply")
	}
}
