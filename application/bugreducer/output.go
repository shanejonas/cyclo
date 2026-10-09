package bugreducer

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

const previewLimit = 16 * 1024

// checkerOutput retains a bounded tail, including while a checker is running.
type checkerOutput struct {
	mu   sync.Mutex
	tail []byte
}

func (o *checkerOutput) Write(data []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	size := len(data)
	incoming := data[max(size-previewLimit, 0):]
	// Keep only the old suffix that fits beside the incoming suffix.
	retained := min(len(o.tail), previewLimit-len(incoming))
	tail := o.tail[len(o.tail)-retained:]
	buffer := o.tail[:copy(o.tail, tail)]
	o.tail = append(buffer, incoming...)
	return size, nil
}

func (o *checkerOutput) reset() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.tail = o.tail[:0]
}

func (o *checkerOutput) snapshot() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return string(o.tail)
}

// Own the pipe so command.Wait can return as soon as the checker exits, even
// when a child inherits stdout. The caller cleans up the process group before
// draining the pipe. exec.Cmd's built-in copier would wait for that child first.
func (o *checkerOutput) capture(command *exec.Cmd) (func() error, error) {
	if o == nil {
		return func() error { return nil }, nil
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	command.Stdout, command.Stderr = writer, writer
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(o, reader)
		done <- err
	}()
	return func() error { return drainOutput(reader, writer, done) }, nil
}

func drainOutput(reader, writer *os.File, done <-chan error) error {
	defer reader.Close()
	closeErr := writer.Close()
	// Detached descendants are outside process-group cleanup. Bound their drain.
	err := reader.SetReadDeadline(time.Now().Add(time.Second))
	if err != nil {
		_ = reader.Close()
		return errors.Join(closeErr, err, <-done)
	}
	copyErr := <-done
	if errors.Is(copyErr, os.ErrDeadlineExceeded) {
		copyErr = nil
	}
	return errors.Join(closeErr, copyErr)
}
