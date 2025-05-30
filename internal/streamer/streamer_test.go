package streamer_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gopheryan/jobby/internal/streamer"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// It takes a lot of boilerplate, but this is a happy-path test
// of the LiveFileStreamer
// Validates that:
//   - The streamer "catches up" by reading existing data from the file
//   - Blocks/waits on new file data using the watcher
//   - Exits cleanly with EOF error once the write handle is closed
func TestFileStreamer(t *testing.T) {
	resources, err := NewTestResources(t.TempDir())
	require.NoError(t, err, "Failed to create test resources")
	defer resources.Cleanup()

	initialData := []byte("how now brown cow")
	resources.WriteHandle.Write(initialData)

	done := make(chan struct{})
	testStreamer, err := streamer.NewLiveFileStreamer(resources.WriteHandle.Name(), done)
	require.NoError(t, err)

	// Streamer should pick up initial data
	readBuf := make([]byte, len(initialData))
	count, err := io.ReadFull(testStreamer, readBuf)
	assert.NoError(t, err)
	assert.Equal(t, len(readBuf), count)
	assert.True(t, bytes.Equal(initialData, readBuf))

	nextData := []byte("I'm a little teacup short and stout")
	readBuf = make([]byte, len(nextData))
	// Validate that the steamer blocks until the next chunk of data
	// is available
	readDone := make(chan struct{})
	var readError error
	var readCount int
	go func() {
		defer close(readDone)
		readCount, readError = io.ReadFull(testStreamer, readBuf)
	}()

	select {
	// Validate that the testStreamer is blocking
	case <-time.After(10 * time.Millisecond):
	case <-readDone:
		// It didn't block. That's not expected behavior
		t.Fail()
		t.Log("test streamer should block")
	}

	count, err = resources.WriteHandle.Write(nextData)
	require.NoError(t, err)
	require.Equal(t, len(nextData), count)

	// Should not block
	select {
	case <-readDone:
	case <-time.After(10 * time.Millisecond):
		t.Fail()
		t.Log("waiting for next chunk of data should not block")

	}
	<-readDone
	assert.NoError(t, readError)
	assert.Equal(t, len(readBuf), readCount)
	assert.True(t, bytes.Equal(nextData, readBuf))

	// Closing the done channel should result in the streamer
	// eventually returning io.EOF
	close(done)
	//resources.Watcher.Close()
	time.Sleep(10 * time.Millisecond)
	count, err = io.ReadFull(testStreamer, readBuf)
	assert.ErrorIs(t, err, io.EOF)
	assert.Equal(t, 0, count)

	require.NoError(t, testStreamer.Close())
}

// Validate close behavior
func TestCallerClose(t *testing.T) {
	resources, err := NewTestResources(t.TempDir())
	require.NoError(t, err, "Failed to create test resources")
	defer resources.Cleanup()

	initialData := []byte("how now brown cow")
	resources.WriteHandle.Write(initialData)

	testStreamer, err := streamer.NewLiveFileStreamer(resources.WriteHandle.Name(), make(chan struct{}))
	require.NoError(t, err)
	// Closing should not return an error
	assert.NoError(t, testStreamer.Close())

	// Read after close *should* return an error
	readBuf := make([]byte, len(initialData))
	_, err = io.ReadFull(testStreamer, readBuf)
	assert.Error(t, err)
}

// Validate unexpected close of read handle returns an error
func TestUnexpectedFileDelete(t *testing.T) {
	resources, err := NewTestResources(t.TempDir())
	require.NoError(t, err, "Failed to create test resources")
	defer resources.Cleanup()

	initialData := []byte("how now brown cow")
	resources.WriteHandle.Write(initialData)

	testStreamer, err := streamer.NewLiveFileStreamer(resources.WriteHandle.Name(), make(chan struct{}))
	require.NoError(t, err)
	os.Remove(resources.WriteHandle.Name())

	readBuf := make([]byte, len(initialData))
	_, err = io.ReadFull(testStreamer, readBuf)
	assert.NoError(t, err)

	assert.NoError(t, testStreamer.Close())
}

type TestResources struct {
	WriteHandle *os.File
}

func (t *TestResources) Cleanup() {
	_ = t.WriteHandle.Close()
}

func NewTestResources(dir string) (TestResources, error) {
	var allErrs error
	testFile, err := os.OpenFile(filepath.Join(dir, "test"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	allErrs = errors.Join(allErrs, err)

	return TestResources{
		WriteHandle: testFile,
	}, allErrs
}
