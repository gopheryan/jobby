package streamer_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gopheryan/jobby/streamer"

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

	testStreamer := streamer.NewLiveFileStreamer(resources.ReadHandle, resources.Watcher)

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

	// Closing the watcher should result in the streamer
	// eventually returning io.EOF
	resources.WriteHandle.Close()
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

	testStreamer := streamer.NewLiveFileStreamer(resources.ReadHandle, resources.Watcher)
	// Closing should not return an error
	assert.NoError(t, testStreamer.Close())

	// Read after close *should* return an error
	readBuf := make([]byte, len(initialData))
	_, err = io.ReadFull(testStreamer, readBuf)
	assert.Error(t, err)
}

// Validate unexpected close of read handle returns an error
func TestUnexpectedFileClose(t *testing.T) {
	resources, err := NewTestResources(t.TempDir())
	require.NoError(t, err, "Failed to create test resources")
	defer resources.Cleanup()

	initialData := []byte("how now brown cow")
	resources.WriteHandle.Write(initialData)

	testStreamer := streamer.NewLiveFileStreamer(resources.ReadHandle, resources.Watcher)
	_ = resources.ReadHandle.Close()

	// Next read should error out
	readBuf := make([]byte, len(initialData))
	_, err = io.ReadFull(testStreamer, readBuf)
	assert.Error(t, err)

	// Should also return an error because the read handle
	// close unexpectedly
	assert.Error(t, testStreamer.Close())
}

type TestResources struct {
	WriteHandle *os.File
	ReadHandle  *os.File
	Watcher     *streamer.FileWriteWatcher
}

func (t *TestResources) Cleanup() {
	_ = t.WriteHandle.Close()
	_ = t.ReadHandle.Close()
	_ = t.Watcher.Close()
}

func NewTestResources(dir string) (TestResources, error) {
	var allErrs error
	testFile, err := os.OpenFile(filepath.Join(dir, "test"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	allErrs = errors.Join(allErrs, err)

	watcher, err := streamer.NewWatcher(testFile.Name()) //fsnotify.NewWatcher()
	allErrs = errors.Join(allErrs, err)

	var readHandle *os.File
	if testFile != nil {
		readHandle, err = os.Open(testFile.Name())
		allErrs = errors.Join(allErrs, err)
	}

	return TestResources{
		WriteHandle: testFile,
		ReadHandle:  readHandle,
		Watcher:     watcher,
	}, allErrs
}
