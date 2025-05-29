package streamer

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Helps us use a function as an io.Reader
type readFunc func([]byte) (int, error)

func (r readFunc) Read(p []byte) (int, error) {
	return r(p)
}

type watchReader func(desc int) (unix.InotifyEvent, error)

// Reads an event from the watch file
// Returns either a valid inotify event or the raw read error
func defaultWatchReader(fd int) (unix.InotifyEvent, error) {
	data := make([]byte, unix.SizeofInotifyEvent)
	f := func(data []byte) (int, error) {
		return unix.Read(fd, data)
	}

	_, readError := io.ReadFull(readFunc(f), data)
	if readError != nil {
		return unix.InotifyEvent{}, readError
	} else {
		inEvent := (*(*unix.InotifyEvent)(unsafe.Pointer(&data[0])))
		return inEvent, nil
	}
}

// FileWriteWatcher watches a file for write events
// The Events channel receives a message for each write event encountered
// Users are expected to read the Events channel until it is closed
type FileWriteWatcher struct {
	watchfd   int
	watchDesc int
	events    chan struct{}
	err       error
	closeOnce *sync.Once
	closeSync chan struct{}
	readEvent watchReader
}

// Close/Stop the FileWriteWatcher
// Caller *must* drain the Events channel
func (w *FileWriteWatcher) Close() error {
	var err error
	w.closeOnce.Do(func() {
		// Close the watch first. Ths will force an IN_IGNORE event
		// out of the watch which will signal shutdown of the
		// goroutine managing reading
		_, e1 := unix.InotifyRmWatch(w.watchfd, uint32(w.watchDesc))
		// Don't let the goroutine exit until we call close
		close(w.closeSync)
		err = e1
	})
	return err
}

// Set after Events channel is closed
// Communicates any errors encounted by the FileWriteWatcher
func (w *FileWriteWatcher) Error() error {
	return w.err
}

// Watcher sends empty messages through the channel whenever a write
// event is received. Channel is closed when the watcher is closed
func (w *FileWriteWatcher) Events() chan struct{} {
	return w.events
}

// Create a new FileWriteWatcher on the file
// Path must point to an existing, regular file
// FileWriteWatcher will watch the file until the caller invokes the watcher's 'Close' method.
func NewWatcher(path string) (*FileWriteWatcher, error) {
	return newWatcher(path, defaultWatchReader)
}

// internally create new watcher with our own event reader function
// mainly implemented this way for testability
func newWatcher(path string, wr watchReader) (*FileWriteWatcher, error) {
	fd, err := unix.InotifyInit()
	if err != nil {
		return nil, err
	}

	// Watch for writes
	wd, err := unix.InotifyAddWatch(fd, path, unix.IN_MODIFY)
	if err != nil {
		// There isn't much we can do if this fails, and it's an internal detail
		// that doesn't mean much to the caller
		// I *could* log it, I suppose
		_ = unix.Close(fd)
		return nil, err
	}

	closeSync := make(chan struct{})

	newWatcher := &FileWriteWatcher{
		watchfd:   fd,
		watchDesc: wd,
		events:    make(chan struct{}),
		closeOnce: &sync.Once{},
		readEvent: wr,
		closeSync: closeSync,
	}

	// Read from the watch until we encounter an error
	go func() {
		defer close(newWatcher.events)
		var readError error
		for readError == nil {
			var inEvent unix.InotifyEvent
			inEvent, readError = newWatcher.readEvent(fd)
			if readError == nil {
				if inEvent.Mask&unix.IN_MODIFY > 0 {
					// Happy path. We got a write event on the file!
					newWatcher.events <- struct{}{}
				} else {
					// IN_IGNORED means the watch was closed
					// Any other event is unexpected
					if inEvent.Mask&unix.IN_IGNORED == 0 {
						readError = fmt.Errorf("unexpected event returned from watch '%d'", inEvent.Mask)
					}
					// Stop reading anyhow
					break
				}
			}
		}
		<-closeSync
		readError = errors.Join(readError, unix.Close(fd))
		// Communicate any errors received by the watcher
		newWatcher.err = readError
	}()

	return newWatcher, nil
}
