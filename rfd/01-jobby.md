## RFD 01 - Jobby

### What
A simple job scheduler consisting of three primary parts

* A reusable Go package which can be used to create/stop processes, view their status, and stream their output
* A service implementation that provides a gRPC interface around this package
* A command line utility that interacts with the API server
  
### Why
To hopefully join your team at Teleport!

## Details
### Job Package
The job will consist of a handful of exported types. The most important of which is the job itself.

 **Job** encapsulates the process that underpins a job, manages its lifecycle, and provides access to output and status information. Job object methods will be thread safe. Multiple goroutines may attach to stdout/stderr concurrently and at any time even after a job has completed. 
```
    type Job struct {// private}

    // Arguments for job creation
    type JobArgs struct{
        // Path or command to execute
        Command string
        // Arguments for command/process
        Args []string
        // File path where the job will write its stdout data
        // ex: /tmp/myjob_stdout
        StdoutPath string
        // File path where the job will write its stderr data
        // ex: /tmp/myjob_stderr
        StderrPath string
    }

    // Creates and starts a job
    func NewJob(args JobArgs) (*Job, error)

    // Stops the by sending a kill signal to the process
    func (j *Job) Stop() error
    
    // Retrieves current status of  job
    // Status struct not shown for brevity, but returns an enum (Running/Completed/Stopped)
    // and the exit code (if available) for the job
    func (j *Job) Status() Status

    // Returns a ReadCloser providing access to the job's standard output stream
    // Stream starts at the beginning of the process's stdout
    func (j *Job) Stdout() io.ReadCloser

    // Returns a ReadCloser providing access to the job's standard output stream
    // Stream starts at the beginning of the process's stderr
    func (j *Job) Stderr() io.ReadCloser
```

#### Note on streaming output
Stdout/Stderr will be available even after the job completes. Each stream will be persisted to a file (created by the job package at creation time) at a path specified by the caller. After the job exits, the lifecycle of these output files is up to the caller. They can delete them later or even just direct the job output to an ephemeral filesytem (tmpfs).

I've chosen to return a ReadCloser from these functions to provide caller with the opportunity to "quit/detach" rather than block indefinitely while waiting for output. This will hopefully provide simple and intuitive interface for callers and make the server implementation a breeze. This is probably the most difficult sub-problem of the project. For scenarios like this, I would much rather manage higher *internal complexity* to provide a simple interface, rather than define a complex interface that might shift complexity onto the caller. 

I plan to leverage inotify (by way of https://github.com/fsnotify/fsnotify) to implement the ReadCloser returned by the Stdout/Stderr methods. Calling these methods returns a reader backed by a goroutine which will *immediately* register a watch on the file *before reading*. It will then begin reading the file until it reaches the EOF. After encountering its first EOF, it will reading "write" events from the watch. Depending on the timing, some of these write events may be "stale". The file was appended to while we were catching up. We may have already read the data corresponding to these writes, and that's OK. 

From this point forward, the goroutine only attempts to read from the output file upon receiving a "write" event from the watch. In addition to blocking on the next "write" event from the watcher, the gorutine should also listen for a "done" channel managed by the job itself. In essence, the job is also backed by a goroutine whose sole job is to close this "done" channel when the process exits (or perhaps it watches for the output file to close). This serves as a signal that no more writes will happen to the file. The goroutine backing this reader will exit, and the reader will return `io.EOF` on the next call to `Read`.

Psuedo code:
```
    go func(done chan struct{}) {
        // read from the output file until EOF is reached
        for {
            select {
                case event, ok := <-watcher.Events:
                    // new data? Try reading from the file again!
                    // We may not get anything (stale event). No matter, keep trying
                case _ = <-done:
                    // we should actually try one last read here
                    // It could be that the process made one or more final writes
                    // to the file, then exited and closed the done channel
                    // there may even be notify events for thos writes, but
                    // go doesn't guarantee evaluation order of select statements
                case _= <-readerClosed:
                    // The user/caller called our "Close" method
                    // clean up and exit.
            }
        }
    }
```


#### Aside
At first I considered adding some sort of "Manager" or "JobStore" type for assigning job identifiers and handling job lookup, but this is out of scope for the package. It's unlikely that other potential consumers of this package would get any use out of it, so that logic should be implemented elsewhere.


### Server
A server implementation will provide a gRPC service interface to the job package. Authentication is managed via mTLS, so each client is expected to present a valid cert. In a nutshell the server will:

* Handle lookup and retrieval of jobs using a UUID assigned to each job
  * A simple KV store in-memory will be used for lookup (a map). It will not persist upon a restart. 
  * The server will asign each job a UUID as an identifier, as well as associate user and group information with each job. 
    * UUID is preferred as an identifier over the process's PID since PIDs will eventually wrap.
* Manage Authentication of users as well implement a simple access control policy for accessing jobs.
  * Each job will be owned by the user who created it. **Only this user will be allowed to view/manage the job.**
  * User will be determined by examining the **Distinguished Name** found in the client certificate.
  
  #### Note
  See Appendix A for protcol buffer definitions

  #### Note on streaming
  The server will use a server-streaming RPC to stream job output to clients. This will allow the server to quickly push output to the client without the need for polling. Clients can detach from the stream at any time by cancelling the request

#### Authentication 
Authentication will be handled via mTLS. I will provide simple automation (a Makefile) to generate a root/CA cert and key as well as client and server certs/keys. For ease of running this code, and because this is just an interview project, I will also check in sample certificates/keys that are ready to use.

The server will expect a `ca.crt`, `server.cert`, and `server.key` present in its working directory to load.

The server will require TLS 1.3; an easy decision considering we're implementing both the client and server.

### Command Line Interface
A command line utility will act as a client to the API server, wrapping the entirety of the gRPC interface. 

Example Usage:
```
    jobby start echo -a hello
    Job Started! Id: d19af777-cfa4-40a3-b835-244365b5a697

    jobby status d19af777-cfa4-40a3-b835-244365b5a697
    Status: COMPLETED, ExitCode: 0

    jobby output --type stdout
    hello
```

The `output` command will continuously stream/print output from long running processes. Users can cancel a running stream with "ctrl+c"

Similar to the server, the client will expect a `ca.cert`, `client.cert`, and `client.key` in the working directory to present to authenticate with the server.

### Appendix A
Sample gRPC service definition
```
syntax = "proto3";

service Jobby {
    rpc StartJob (StartJobRequest) returns StartJobResponse {}
    rpc StopJob (StopJobRequest) returns StopJobResponse {}
    rpc GetStatus (GetStatusRequest) returns GetStatusResponse{}
    // Server will close the send-stream once output is exhausted
    rpc GetJobOutput (GetJobOutputRequest) returns stream GetJobOutputResponse {}
}

message StartJobRequest {
    string command = 1;
    repeated string args = 2;
}

message StartJobResponse {
   bytes job_id = 1;
}

message StopJobRequest {
   bytes job_id = 1;
}

message StopJobResponse {
   // Intentionally empty
}

message GetStatusRequest {
    bytes job_id = 1;
}

enum Status {
    STATUS_UNSPECIFIED = 0;
    // Currently running
    STATUS_RUNNING = 1;
    // Stopped prematurely (due to user action)
    STATUS_STOPPED = 2;
    // Completed 
    STATUS_COMPLETE = 3;
}

message GetStatusResponse {
   Status current_status = 1;
   // available when status is "COMPLETE"
   optional int32 exit_code = 2;
}

enum OutputType {
    OUTPUT_TYPE_UNSPECIFIED = 0;
    OUTPUT_TYPE_STDOUT = 1;
    OUTPUT_TYPE_STDERR = 2;
}

message GetJobOutputRequest {
   bytes job_id = 1;
   OutputType type = 2;
}

message GetJobOutputResponse {
    // A chunk of output data from the job
   bytes data = 1;
}
```