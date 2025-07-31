# shellrunner

`shellrunner` is a high-performance JSON-RPC server written in Go that provides a robust
interface for executing and managing shell commands. It listens on a local Unix socket,
offering both synchronous and asynchronous execution, job management, and statistics
tracking.

This project includes a command-line client in Go and a client library in Common Lisp for
interacting with the server.

## Features

- **Synchronous Execution**: Run a command and block until it completes, receiving the exit code, stdout, and stderr.
- **Asynchronous Execution**: Launch a command in the background and receive a unique job ID for later interaction.
- **Job Management**: List all running and completed jobs, query their status, and retrieve their output.
- **Resource Control**: Release finished jobs individually, or all at once, to free up memory.
- **Persistent History**: Optionally keep the results of synchronous commands for later inspection.
- **Performance Statistics**: Track the total number of executed commands, their average duration, and the maximum duration.
- **Simple Job IDs**: Uses simple, sequential integer IDs for easy reference.
- **Optional Logging**: Enable detailed logging via a command-line flag or an environment variable.

## Installation & Building

To build the `shellrunner` server, you need a Go environment (version 1.18 or later is recommended).

```sh
# Clone the repository (if you haven't already)
git clone <repository-url>
cd shellrunner

# Build the server binary
go build
```

This will create a `shellrunner` executable in the project directory.

## Usage

### Starting the Server

To start the server, simply run the compiled binary:

```sh
./shellrunner
```

The server will start and listen on a Unix socket at `/tmp/shellrunner.sock`.

#### Logging

You can enable logging to stdout using either the `-logging` flag or the `SHELLRUNNER_LOGGING` environment variable.

```sh
# Using the flag
./shellrunner -logging

# Using the environment variable
SHELLRUNNER_LOGGING=true ./shellrunner
```

### JSON-RPC API

The server exposes a set of methods that can be called via JSON-RPC 2.0.

- **`ShellRunner.Run`**: Executes a command synchronously.
  - **Params**: `{"command": "<command>", "keep": <bool>}`
  - **Result**: `{"stdout": "...", "stderr": "...", "exit_code": 0, "job_id": "..."}` (job_id is only present if `keep` is true)

- **`ShellRunner.Background`**: Executes a command asynchronously.
  - **Params**: `"<command>"`
  - **Result**: `"<job_id>"`

- **`ShellRunner.Status`**: Retrieves the status of a job.
  - **Params**: `"<job_id>"`
  - **Result**: `{"command": "...", "status": "...", "start_time": "...", "duration_seconds": 0.0}`

- **`ShellRunner.Output`**: Retrieves the output of a job.
  - **Params**: `{"id": "<job_id>", "release": <bool>}`
  - **Result**: `{"stdout": "...", "stderr": "..."}`

- **`ShellRunner.Release`**: Releases a job's resources.
  - **Params**: `"<job_id>"`
  - **Result**: `true`

- **`ShellRunner.ReleaseAll`**: Releases all finished jobs.
  - **Params**: `{}`
  - **Result**: `<released_count>`

- **`ShellRunner.List`**: Lists all jobs.
  - **Params**: `{}`
  - **Result**: `[{"id": "1", "status": "running"}, ...]`

- **`ShellRunner.Statistics`**: Retrieves server statistics.
  - **Params**: `{}`
  - **Result**: `{"total_count": 0, "average_duration_seconds": 0.0, "max_duration_seconds": 0.0}`

## Go Client

A command-line client is provided in the `client/` directory.

### Usage

```sh
go run client/main.go <method> [args...]
```

**Available Methods:**

- `run <command> [--keep]`: Executes a command synchronously.
- `background <command>`: Starts a background job.
- `status <job_id>`: Checks a job's status.
- `output <job_id> [--release]`: Retrieves a job's output.
- `release <job_id>`: Releases a job.
- `release-all`: Releases all finished jobs.
- `list`: Lists all jobs.
- `statistics`: Shows server statistics.

### Examples

```sh
# Run a command and keep its results for later
go run client/main.go run "ls -la" --keep

# Start a background job
go run client/main.go background "sleep 5 && echo 'done'"

# List all jobs
go run client/main.go list

# Get the status of job "1"
go run client/main.go status "1"
```

## Development

### Testing

The project includes a comprehensive suite of unit and integration tests. To run them, use the following command:

```sh
go test -v ./...
```

## License

This project is licensed under the MIT License.
