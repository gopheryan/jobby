package streamer

import (
	"bufio"
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
	//os.OpenFile(filepath.Join(t.TempDir(),
	done := make(chan struct{})
	fs, err := NewLiveFileStreamer(writeHandle.Name(), done)
	require.NoError(t, err)

	// Close the read handle in the middle of reading!
	fs.file.Close()

	// Reading should fail with a non-EOF error
	_, err = io.ReadAll(fs)
	require.Error(t, err)
	// Subsequent call should return non-EOF as well
	_, err = io.ReadAll(fs)
	require.Error(t, err)
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
	assert.NoError(t, err)
	assert.Equal(t, len(helloData), int(count))

	// Read should return EOF
	readCount, err := fs.Read(make([]byte, len(helloData))) // Intentional throw-away byte slice
	assert.Equal(t, 0, readCount)
	assert.ErrorIs(t, err, io.EOF)

	assert.NoError(t, fs.Close())
}
