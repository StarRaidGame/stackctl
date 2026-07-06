package supervisor

import (
	"bytes"
	"sync"
)

// LogBuffer is a bounded, line-oriented ring buffer that implements io.Writer, so
// a child's stdout/stderr can be wired straight into it (cmd.Stdout = buf). It
// retains at most the last `max` complete lines; a trailing partial line (no
// newline yet) is held back until its newline arrives. Safe for concurrent use:
// the process's IO goroutines write while the UI reads via Lines.
type LogBuffer struct {
	mu      sync.Mutex
	lines   []string
	pending []byte
	max     int
}

// NewLogBuffer returns a buffer retaining the last max lines (default 500).
func NewLogBuffer(max int) *LogBuffer {
	if max <= 0 {
		max = 500
	}
	return &LogBuffer{max: max}
}

// Write splits p into lines on '\n', appending complete lines and buffering any
// trailing partial line for the next write. It never returns an error.
func (b *LogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pending = append(b.pending, p...)
	for {
		i := bytes.IndexByte(b.pending, '\n')
		if i < 0 {
			break
		}
		line := string(bytes.TrimRight(b.pending[:i], "\r"))
		b.pending = append(b.pending[:0], b.pending[i+1:]...) // consume, compact
		b.lines = append(b.lines, line)
		if len(b.lines) > b.max {
			b.lines = b.lines[len(b.lines)-b.max:]
		}
	}
	return len(p), nil
}

// Lines returns a snapshot copy of the retained complete lines.
func (b *LogBuffer) Lines() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(b.lines))
	copy(out, b.lines)
	return out
}
