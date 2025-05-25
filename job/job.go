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

func NewState(processExited bool, e *exec.ExitError) State {
	if processExited {
		// ExitCode() returns -1 if the process hasn't exited or was terminated by a signal
		// Well we know the process exited
		if e != nil && e.ExitCode() == -1 {
			status, ok := e.ProcessState.Sys().(syscall.WaitStatus)
			if ok && status.Signal() == syscall.SIGKILL {
				// I suppose we can't know for certain that it this program who sent
				// the SIGKILL, but it was *probably* us
				// Maybe the documentation around the "STOPPED" state should instead be
				// that this state means the program was forcefully terminated, rather than
				// terminated *by a user*
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
	sync.Mutex
	cmd           exec.Cmd
	processExited bool
	exitErr       *exec.ExitError

	stdoutPath string
	stderrPath string
}

func logFileClose(f *os.File) {
	if err := f.Close(); err != nil {
		if !errors.Is(err, os.ErrInvalid) {
			slog.Error("Failed to close file '%s': %w", f.Name(), err)
		}
	}
}

func NewJob(args JobArgs) (*Job, error) {
	c := exec.Cmd{
		Path: args.Command,
		Args: args.Args,
	}

	// Create our output files!
	stdoutFile, err := createOutputFile(args.StdoutPath)
	stderrFile, err2 := createOutputFile(args.StderrPath)
	if errors.Join(err, err2) != nil {
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
		cmd:        c,
		stdoutPath: args.StdoutPath,
		stderrPath: args.StderrPath,
	}

	// Now create a goroutine which will watch for the process to exit
	// it will atomically update the 'processExited' and 'exitErr' upon
	// process exit. Output files will be closed *after* releasing
	// the job lock
	go func() {
		defer logFileClose(stdoutFile)
		defer logFileClose(stderrFile)

		err = c.Wait()
		// Lock the job while we update the exit status
		newJob.Lock()
		defer newJob.Unlock()

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
	var exited bool
	j.Lock()
	exited = j.processExited
	exitErr = j.exitErr
	j.Unlock()

	var exitCode *int
	if exitErr != nil {
		tmp := exitErr.ExitCode()
		exitCode = &tmp
	}

	return Status{
		CurrentState: NewState(exited, exitErr),
		ReturnCode:   exitCode,
	}
}

func (j *Job) Stop() error {
	var err error
	j.Lock()
	if !j.processExited {
		err = j.cmd.Process.Kill()
	}
	j.Unlock()

	if err != nil {
		err = fmt.Errorf("failed to send kill signal to process: %w", err)
	}
	return err
}

func (j *Job) watchOutput(path string) (io.ReadCloser, error) {
	readHandle, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error opening output file '%s' for reading: %w", path, err)
	}

	watcher, err := streamer.NewWatcher(path)
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	j.Lock()
	if j.processExited {
		if err = watcher.Close(); err != nil {
			err = fmt.Errorf("error closing watcher: %w", err)
		}
	}
	j.Unlock()
	if err != nil {
		return nil, err
	}

	return streamer.NewLiveFileStreamer(readHandle, watcher), nil
}

func (j *Job) Stdout() (io.ReadCloser, error) {
	return j.watchOutput(j.stdoutPath)
}

func (j *Job) Stderr() (io.ReadCloser, error) {
	return j.watchOutput(j.stderrPath)
}
