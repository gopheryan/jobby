package job_test

import (
	"bytes"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gopheryan/jobby/job"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const echoPathRelative = "../testdata/testprograms/echo"

func expectEchoOutput(stdout bool, count int) string {
	prefix := "stdout"
	if !stdout {
		prefix = "stderr"
	}

	bldr := strings.Builder{}
	for idx := range count {
		bldr.WriteString(prefix)
		bldr.WriteString(" ")
		bldr.WriteString(strconv.Itoa(idx + 1))
		bldr.WriteString("\n")
	}
	return bldr.String()
}

// First happy path test of the job!
func TestJob(t *testing.T) {
	dir := t.TempDir()
	j, err := job.NewJob(job.JobArgs{
		Command: echoPathRelative,
		// should take >=2.5 seconds to complete
		Args:       []string{"echo", "5"},
		StdoutPath: filepath.Join(dir, "file.stdout"),
		StderrPath: filepath.Join(dir, "file.sterr"),
	})
	assert.NoError(t, err)

	// Status should be running
	status := j.Status()
	assert.Equal(t, status.CurrentState, job.JobStatusRunning)

	sout, err := j.Stdout()
	require.NoError(t, err)
	defer sout.Close()

	serr, err := j.Stderr()
	require.NoError(t, err)
	defer sout.Close()

	stdoutData, err := io.ReadAll(sout)
	require.NoError(t, err)
	assert.Equal(t, expectEchoOutput(true, 5), string(stdoutData))

	// Note: the process has already exited by now, but stderr should still be readable
	// In fact, let's validate that assumption
	status = j.Status()
	assert.Equal(t, status.CurrentState, job.JobstatusComplete)

	stderrData, err := io.ReadAll(serr)
	require.NoError(t, err)
	assert.Equal(t, expectEchoOutput(false, 5), string(stderrData))
}

func TestJobBadOutputPaths(t *testing.T) {
	// path does not exist
	j, err := job.NewJob(job.JobArgs{
		Command: echoPathRelative,
		// should take >=2.5 seconds to complete
		Args:       []string{"echo", "5"},
		StdoutPath: "/var/a",
		StderrPath: "/bar/a",
	})
	assert.Error(t, err)
	assert.Nil(t, j)
}

func TestJobStop(t *testing.T) {
	dir := t.TempDir()
	j, err := job.NewJob(job.JobArgs{
		Command: echoPathRelative,
		// should take >=250 seconds to complete
		Args:       []string{"echo", "500"},
		StdoutPath: filepath.Join(dir, "file.stdout"),
		StderrPath: filepath.Join(dir, "file.sterr"),
	})
	require.NoError(t, err)
	assert.NoError(t, j.Stop(), "Failed to Stop job")

	// Wait a reasonable amount of time for the job to stop
	ticker := time.NewTicker(10 * time.Millisecond)
loop:
	for {
		select {
		case <-ticker.C:
			if j.Status().CurrentState == job.JobStatusStopped {
				break loop
			}
		case <-time.After(1 * time.Second):
			break loop
		}
	}
	assert.Equal(t, job.JobStatusStopped, j.Status().CurrentState)
}

func TestDetachAndReattach(t *testing.T) {
	// Attach to stdout, but then detach (close the reader)
	// shortly after
	dir := t.TempDir()
	j, err := job.NewJob(job.JobArgs{
		Command:    echoPathRelative,
		Args:       []string{"echo", "15"},
		StdoutPath: filepath.Join(dir, "file.stdout"),
		StderrPath: filepath.Join(dir, "file.sterr"),
	})
	require.NoError(t, err)
	sout, err := j.Stdout()
	require.NoError(t, err)

	// read a bit of output
	stdoutData := make([]byte, 8)
	_, err = io.ReadFull(sout, stdoutData)
	assert.NoError(t, err)
	assert.NoError(t, sout.Close())

	// Now do it again. We should get the same data
	sout2, err := j.Stdout()
	assert.NoError(t, err)

	secondStdoutdata := make([]byte, 8)
	_, err = io.ReadFull(sout2, secondStdoutdata)
	assert.NoError(t, err)

	// Subsequent attach and read should restart at beginning of stdout
	// meaning we shoud ahave the same data read into each buffer
	assert.True(t, bytes.Equal(stdoutData, secondStdoutdata))

	// Stop the job and wait for it to complete (by reading the rest of stdout)
	assert.NoError(t, j.Stop())
	_, err = io.ReadAll(sout2)
	assert.NoError(t, err)
	assert.Equal(t, job.JobStatusStopped, j.Status().CurrentState)
	require.NoError(t, sout2.Close())
}
