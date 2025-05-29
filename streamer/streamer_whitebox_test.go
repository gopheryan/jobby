package streamer

import (
	"bufio"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadError(t *testing.T) {
	writeHandle, err := os.CreateTemp(t.TempDir(), "")
	require.NoError(t, err)
	defer writeHandle.Close()

	done := make(chan struct{})
	fs, err := NewLiveFileStreamer(writeHandle.Name(), done)
	require.NoError(t, err)

	// Close the read handle in the middle of reading!
	fs.file.Close()

	// Reading should fail with a non-EOF error
	_, err = io.ReadAll(fs)
	require.Error(t, err)
	require.False(t, errors.Is(err, io.EOF))
	// Subsequent call should return non-EOF as well
	_, err = io.ReadAll(fs)
	require.Error(t, err)
	require.NotErrorIs(t, err, io.EOF)

	// Calling read directly should return non-EOF error
	// We did not happily reach end of data, we instead
	// encountered an error
	count, err := fs.Read(make([]byte, 1))
	require.Error(t, err)
	require.NotErrorIs(t, err, io.EOF)
	require.Zero(t, count)

	// Should complain also about the read handle
	// having already been closed
	assert.Error(t, fs.Close())
}

func TestHappyRead(t *testing.T) {
	writeHandle, err := os.CreateTemp(t.TempDir(), "")
	require.NoError(t, err)
	defer writeHandle.Close()
	done := make(chan struct{})
	fs, err := NewLiveFileStreamer(writeHandle.Name(), done)
	require.NoError(t, err)

	helloData := []byte("hello!")
	_, err = writeHandle.Write(helloData)
	require.NoError(t, err)
	close(done)

	bufWriter := bufio.NewWriter(nil)
	// Copy works! It knows we're done because
	// our LiveFileStreamer returned an EOF
	count, err := io.Copy(bufWriter, fs)
	assert.NoError(t, err) // Copy receives EOF internally, but reports nil
	assert.Equal(t, len(helloData), int(count))

	// Read should return EOF
	readCount, err := fs.Read(make([]byte, len(helloData))) // Intentional throw-away byte slice
	assert.Equal(t, 0, readCount)
	assert.ErrorIs(t, err, io.EOF)

	assert.NoError(t, fs.Close())
}
