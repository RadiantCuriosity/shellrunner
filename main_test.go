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
	if id == "" {
		t.Fatal("expected a job id, got empty string")
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
	var id string
	err := shellRunner.Background(`sleep 0.2`, &id)
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
	var id string
	err := shellRunner.Background(`echo "test output"; >&2 echo "test error"`, &id)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	time.Sleep(100 * time.Millisecond) // allow command to finish

	reply := make(map[string]interface{})
	err = shellRunner.Output(id, &reply)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if reply["stdout"] != "test output\n" {
		t.Errorf("expected stdout to be 'test output\\n', got %q", reply["stdout"])
	}
	if reply["stderr"] != "test error\n" {
		t.Errorf("expected stderr to be 'test error\\n', got %q", reply["stderr"])
	}
}
