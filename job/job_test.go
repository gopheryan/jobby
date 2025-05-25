package job_test

import (
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
