package job

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/gopheryan/jobby/streamer"
)

// Current process state
type State string

const (
	// The process is currently running
	JobStatusRunning State = "RUNNING"
	// Not necessarily a 'success' (see exit code for that)
	// This state signals that the process is no longer running
	JobstatusComplete State = "COMPLETE"
	// Different from 'COMPLETE' in that this state
	// means that the user deliberately stopped the job
	JobStatusStopped State = "STOPPED"
)

func newState(processExited, userKilled bool, e *exec.ExitError) State {
	if processExited {
		// ExitCode() returns -1 if the process hasn't exited or was terminated by a signal
		// Well we know the process exited
		if e != nil && e.ExitCode() == -1 {
			status, ok := e.ProcessState.Sys().(syscall.WaitStatus)
			// We know the process has exited, and the 'userKilled' flag *may*
			// be set, but there's a chance that the process exited just as Stop()
			// was called. Validate that this process was killed by a kill signal
			if ok && status.Signal() == syscall.SIGKILL && userKilled {
				// The process has exited, it was killed by a kill signal,
				// and the job has a 'userKilled' flag indicating that the caller
				// asked us to stop a running job.
				return JobStatusStopped
			}
		}
		return JobstatusComplete
	}
	return JobStatusRunning
}

type Status struct {
	CurrentState State
	ReturnCode   *int
}

type JobArgs struct {
	Command    string
	Args       []string
	StdoutPath string
	StderrPath string
}

type Job struct {
	jobLock       sync.Mutex
	cmd           exec.Cmd
	processExited bool
	processDone   chan struct{}
	exitErr       *exec.ExitError
	userKilled    bool

	stdoutPath string
	stderrPath string
}

func logFileClose(f *os.File) {
	if err := f.Close(); err != nil {
		if !errors.Is(err, os.ErrInvalid) {
			slog.Error("Failed to close file", "error", err)
		}
	}
}

func New(args JobArgs) (*Job, error) {
	c := exec.Cmd{
		Path: args.Command,
		Args: args.Args,
	}

	// Create our output files!
	stdoutFile, err := createOutputFile(args.StdoutPath)
	stderrFile, err2 := createOutputFile(args.StderrPath)
	if err := errors.Join(err, err2); err != nil {
		logFileClose(stdoutFile)
		logFileClose(stderrFile)
		return nil, fmt.Errorf("error creating output file(s): %w", err)
	}

	c.Stdout = stdoutFile
	c.Stderr = stderrFile

	if err = c.Start(); err != nil {
		logFileClose(stdoutFile)
		logFileClose(stderrFile)
		return nil, fmt.Errorf("error starting process: %w", err)
	}

	newJob := &Job{
		cmd:         c,
		stdoutPath:  args.StdoutPath,
		stderrPath:  args.StderrPath,
		processDone: make(chan struct{}),
	}

	// Now create a goroutine which will watch for the process to exit
	// it will atomically update the 'processExited' and 'exitErr' upon
	// process exit. Output files will be closed *after* releasing
	// the job lock
	go func() {
		defer logFileClose(stdoutFile)
		defer logFileClose(stderrFile)

		err := c.Wait()
		// Lock the job while we update the exit status
		newJob.jobLock.Lock()
		// This will unlock *before* the output files close.
		// We may consider holding the lock until the files are
		// closed, but I don't believe we need that guarantee
		// Other methods can be assure that observing
		// 'processExited == true' means that the last write to
		// the output files have completed
		defer newJob.jobLock.Unlock()

		close(newJob.processDone)
		newJob.processExited = true
		newJob.exitErr, _ = err.(*exec.ExitError)
	}()

	return newJob, err
}

func createOutputFile(path string) (*os.File, error) {
	// We need to open a file for writing, create if not exists,
	// and truncate existing files
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	// current user process can read/write, group members can read
	return os.OpenFile(path, flags, 0640)
}

func (j *Job) Status() Status {
	var exitErr *exec.ExitError
	j.jobLock.Lock()

	exitErr = j.exitErr
	currentState := newState(j.processExited, j.userKilled, j.exitErr)
	var exitCode *int
	if exitErr != nil {
		tmp := exitErr.ExitCode()
		exitCode = &tmp
	}

	j.jobLock.Unlock()

	return Status{
		CurrentState: currentState,
		ReturnCode:   exitCode,
	}
}

func (j *Job) Stop() error {
	var err error
	j.jobLock.Lock()
	if !j.processExited {
		err = j.cmd.Process.Kill()
		if err == nil {
			// Track that a successful kill signal was
			// sent to a running process by the caller
			j.userKilled = true
		}
	}
	j.jobLock.Unlock()

	if err != nil {
		err = fmt.Errorf("failed to send kill signal to process: %w", err)
	}
	return err
}

func (j *Job) watchOutput(path string) (io.ReadCloser, error) {
	fileStreamer, err := streamer.NewLiveFileStreamer(path, j.processDone)
	if err != nil {
		return nil, fmt.Errorf("failed to create file streamer: %w", err)
	}
	return fileStreamer, nil
}

func (j *Job) Stdout() (io.ReadCloser, error) {
	return j.watchOutput(j.stdoutPath)
}

func (j *Job) Stderr() (io.ReadCloser, error) {
	return j.watchOutput(j.stderrPath)
}
