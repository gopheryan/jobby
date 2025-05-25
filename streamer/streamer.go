package streamer

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

// "Writes can be serialized with respect to other reads and writes. If a read() of file data can be proven (by any means)
// to occur after a write() of the data, it must reflect that write(), even if the calls are made by different processes"
// 		- https://pubs.opengroup.org/onlinepubs/009695399/functions/write.html
// Means we can indeed have many readers concurrently read a file single writer.
// To avoid readers missing data, mainly around the time the file is closed, our reader just needs to make once
// last effort to read from the file *after* the file is closed (the writer has exited)

// LiveFileStreamer provides a ReadCloser interface around
// a file that has at most a single active writer
// Reads data from the file as the writer writes to it.
// Returns EOF once the writer closes the file and the last data
// from the file is read.
// Closing the provided watcher prematurely also results in graceful termination (with EOF error)
type LiveFileStreamer struct {
	// The file to 'LiveStream' from
	File *os.File

	// A FileWriteWatcher that is watching the associated
	// File handle
	WriteWatcher *FileWriteWatcher

	// Directs the Read method to either try reading from
	// the file, or wait for a write event from the watcher.
	// True when the last read to the file reached EOF
	// False if we think there's still data in the file
	useWatch bool

	// Captures any unexpected error
	// short circuits all subsequent calls to 'Read'
	done error
	// manage close behavior
	closeOnce sync.Once
}

func NewLiveFileStreamer(file *os.File, W *FileWriteWatcher) *LiveFileStreamer {
	return &LiveFileStreamer{
		WriteWatcher: W,
		File:         file,
	}
}

func (l *LiveFileStreamer) fileRead(p []byte) (int, error) {
	count, err := l.File.Read(p)
	if err != nil {
		// Read returns 0, io.EOF, so we need not check count
		// or deal with data in 'p' on EOF
		if errors.Is(err, io.EOF) {
			// We're caught up
			l.useWatch = true
		} else {
			l.done = err
			return count, l.done
		}
	}
	return count, nil
}

func (l *LiveFileStreamer) Read(p []byte) (int, error) {
	if l.done != nil {
		return 0, l.done
	}

	if !l.useWatch {
		return l.fileRead(p)
	}

	_, ok := <-l.WriteWatcher.Events()
	if !ok {
		// Closure of the watch indicates that no more
		// writes will occur. BUT, there may stil be data left in the file
		count, err := l.fileRead(p)
		if count == 0 && err == nil && l.useWatch {
			// This is our end condition. The watcher is closed,
			// and the file is exhausted
			l.done = io.EOF // Short circuit next time
			// If the watcher encountered an error, inform the reader
			if l.WriteWatcher.Error() != nil {
				l.done = fmt.Errorf("watcher encountered unexpected error: %w", l.WriteWatcher.Error())
			}
			return 0, l.done
		}
		return count, err
	}
	return l.fileRead(p)

}

func (l *LiveFileStreamer) Close() error {
	var err error
	l.closeOnce.Do(func() {
		err = errors.Join(
			l.WriteWatcher.Close(),
			l.File.Close(),
		)
		// Drain events channel as per our contract
		// with the WriteWatcher
		for range l.WriteWatcher.Events() {

		}
	})
	return err
}
