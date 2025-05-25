package streamer

import (
	"errors"
	"fmt"
	"io"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Helps us use a function as an io.Reader
type readFunc func([]byte) (int, error)

func (r readFunc) Read(p []byte) (int, error) {
	return r(p)
}

// FileWriteWatcher watches a file for write events
// The Events channel receives a message for each write event encountered
// If FileWriteWatcher closes the events channel encounters an IN_WRITE_CLOSE
// or if the caller closes the watch
// Users are expected to read the Events channel until it is closed
type FileWriteWatcher struct {
	watchfd   int
	watchDesc int
	events    chan struct{}
	err       error
}

// Close/Stop the FileWriteWatcher
// Caller *must* drain the Events channel
func (w *FileWriteWatcher) Close() error {
	return errors.Join(
		unix.Close(w.watchDesc),
		unix.Close(w.watchfd),
	)
}

// Set after Events channel is closed
// Communicates any errors encounted by the FileWriteWatcher
func (w *FileWriteWatcher) Error() error {
	return w.err
}

// Watcher sends empty messages through the channel whenever a write
// event is received. Channel is closed when the file closes or watcher is closed
func (w *FileWriteWatcher) Events() chan struct{} {
	return w.events
}

// Create a new FileWriteWatcher on the file
// Path must point to an existing, regular file
// FileWriteWatcher will watch the file until it receives a close event from a writer
// or the caller invokes the watcher's 'Close' method.
func NewWatcher(path string) (*FileWriteWatcher, error) {
	fd, err := unix.InotifyInit()
	if err != nil {
		return nil, err
	}

	// Watch for writes and close
	wd, err := unix.InotifyAddWatch(fd, path, unix.IN_CLOSE_WRITE|unix.IN_MODIFY)
	if err != nil {
		// There isn't much we can do if this fails, and it's an internal detail
		// that doesn't mean much to the caller
		// I *could* log it, I suppose
		_ = unix.Close(fd)
		return nil, err
	}

	// Close over the watch file descriptor
	// with a function that we can use as an io.Reader
	f := func(data []byte) (int, error) {
		return unix.Read(fd, data)
	}

	newWatcher := &FileWriteWatcher{
		watchfd:   fd,
		watchDesc: wd,
		events:    make(chan struct{}),
	}

	// Read from the watch until we encounter an error, or observe a "IN_CLOSE_WRITE" event
	go func() {
		defer close(newWatcher.events)
		data := make([]byte, unix.SizeofInotifyEvent)
		var readError error
		for {
			// When the watch is closed, we will break out of this read call
			// with an EBADF
			_, readError = io.ReadFull(readFunc(f), data)
			if readError != nil {
				if errors.Is(readError, syscall.EBADF) {
					// Intentional/graceful close
					readError = nil
				} else {
					readError = fmt.Errorf("error reading from watch: %w", readError)
				}
				break
			} else {
				inEvent := (*(*unix.InotifyEvent)(unsafe.Pointer(&data[0])))
				if inEvent.Mask&unix.IN_MODIFY > 0 {
					// Happy path. We got a write event on the file!
					newWatcher.events <- struct{}{}
				} else {
					// Exit on IN_CLOSE_WRITe
					// log an error if we encountered any other event type
					if inEvent.Mask&unix.IN_CLOSE_WRITE == 0 {
						readError = fmt.Errorf("unexpected event returned from watch '%d'", inEvent.Mask)
					}
					break
				}
			}
		}
		// Communicate any errors received by the watcher
		newWatcher.err = readError
	}()

	return newWatcher, nil
}
