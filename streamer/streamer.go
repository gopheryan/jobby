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
	file *os.File

	// A FileWriteWatcher that is watching the associated
	// File handle
	writeWatcher *FileWriteWatcher

	// Indicates that the file will receive no more writes
	writerDone chan struct{}

	// manage close behavior
	closeOnce *sync.Once
}

func NewLiveFileStreamer(path string, writerDone chan struct{}) (*LiveFileStreamer, error) {
	readHandle, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error opening output file '%s' for reading: %w", path, err)
	}

	watcher, err := NewWatcher(path)
	if err != nil {
		_ = readHandle.Close()
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	return &LiveFileStreamer{file: readHandle, writeWatcher: watcher, writerDone: writerDone, closeOnce: &sync.Once{}}, nil
}

func (l *LiveFileStreamer) Read(p []byte) (int, error) {
	// Read returns 0, io.EOF, so we need not check count
	// or deal with data in 'p' on EOF
	for {
		count, err := l.file.Read(p)
		if err == nil || !errors.Is(err, io.EOF) {
			// return on success or file read errors
			return count, err
		}

		select {
		case _, ok := <-l.writeWatcher.Events():
			if !ok {
				if l.writeWatcher.Error() != nil {
					return 0, fmt.Errorf("watcher encountered unexpected error: %w", l.writeWatcher.Error())
				}
				return l.file.Read(p)
			}
		case <-l.writerDone:
			// Do not take this path again
			l.writerDone = nil
			// We must take care to drain the watcher channel
			err := l.writeWatcher.Close()
			if err != nil {
				// That's not good
				for range l.writeWatcher.Events() {
				}
				return 0, err
			}
		}
	}
}

// Safe for multiple calls, but subsequent
// calls are ineffectual and always return nil
func (l *LiveFileStreamer) Close() error {
	var err error
	l.closeOnce.Do(func() {
		err = errors.Join(
			l.writeWatcher.Close(),
			l.file.Close(),
		)
		// Drain events channel as per our contract
		// with the WriteWatcher
		for range l.writeWatcher.Events() {
		}
	})
	return err
}
