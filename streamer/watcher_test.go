package streamer

// Use 'streamer' instead of 'streamer_test' package to do some white box testing

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestWatch(t *testing.T) {

	t.Run("bad-path", func(tt *testing.T) {
		_, err := NewWatcher("/notexists")
		assert.Error(tt, err)
	})

	t.Run("create-close", func(tt *testing.T) {
		tt.TempDir()
		file, err := os.CreateTemp(t.TempDir(), "")
		require.NoError(tt, err)
		defer file.Close()

		watcher, err := NewWatcher(file.Name())
		assert.NoError(tt, err)

		assert.NoError(tt, watcher.Close())
		<-watcher.events
		assert.NoError(tt, watcher.Error())
	})

	t.Run("read-error", func(tt *testing.T) {
		tt.TempDir()
		file, err := os.CreateTemp(t.TempDir(), "")
		require.NoError(tt, err)
		defer file.Close()

		badReader := func(_ int) (unix.InotifyEvent, error) {
			return unix.InotifyEvent{}, errors.New("unexpected error while reading watch!")
		}

		// Inject a reader that will return an error upon the first read
		// of the watch descriptor
		watcher, err := newWatcher(file.Name(), badReader)
		assert.NoError(tt, err)
		for range watcher.Events() {
		}

		assert.NoError(tt, watcher.Close())
		assert.Error(tt, watcher.Error())
	})

}
