package ui

import (
	"bytes"
	"sync"
)

// LogReader is the read side the TUI consumes per tick.
type LogReader interface {
	// Lines returns the most recent N entries (oldest first). The
	// returned slice is a fresh copy; callers may retain it across
	// ticks without holding the buffer's lock.
	Lines(n int) []string
}

// LogBuffer is a thread-safe fixed-capacity ring buffer that
// satisfies io.Writer (so zerolog can target it via io.MultiWriter)
// and LogReader (so the TUI can tail it).
//
// Writes are split on '\n'; partial trailing lines are kept across
// Write calls and emitted once the next newline arrives. Lines
// exceeding maxLineLen are truncated with the trailing '…' rune so
// the TUI can render them without wrap surprises.
type LogBuffer struct {
	mu         sync.Mutex
	lines      []string // ring; len == capacity once warm
	head       int      // index of the next write position
	wrapped    bool     // true once head has wrapped past 0
	cap        int      // ring capacity
	maxLineLen int      // hard truncate at this byte length (0 = no cap)
	pending    bytes.Buffer
}

// NewLogBuffer returns a buffer that retains the last `capacity`
// lines. maxLineLen is a soft byte cap to keep render width bounded;
// pass 0 to disable truncation.
func NewLogBuffer(capacity, maxLineLen int) *LogBuffer {
	if capacity <= 0 {
		capacity = 1024
	}
	return &LogBuffer{
		lines:      make([]string, capacity),
		cap:        capacity,
		maxLineLen: maxLineLen,
	}
}

// Write absorbs raw bytes (typically from zerolog ConsoleWriter),
// splits on newlines, and stores complete lines in the ring. Always
// returns len(p), nil — io.Writer's "wrote everything" contract.
func (b *LogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, c := range p {
		if c == '\n' {
			b.flushPendingLocked()
			continue
		}
		b.pending.WriteByte(c)
	}
	return len(p), nil
}

// Flush forces any partial trailing line into the ring. Call before
// shutdown so the operator does not lose the last log line.
func (b *LogBuffer) Flush() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.flushPendingLocked()
}

// Lines returns up to n most recent lines (oldest first) as a fresh
// copy. n <= 0 returns the entire buffer in chronological order.
func (b *LogBuffer) Lines(n int) []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	size := b.sizeLocked()
	if size == 0 {
		return nil
	}
	if n <= 0 || n > size {
		n = size
	}

	out := make([]string, 0, n)
	// Oldest entry sits at b.head when wrapped, at 0 otherwise.
	start := 0
	if b.wrapped {
		start = b.head
	}
	// Walk forward `size` entries, then take the last n.
	skip := size - n
	for i := 0; i < size; i++ {
		idx := (start + i) % b.cap
		if i < skip {
			continue
		}
		out = append(out, b.lines[idx])
	}
	return out
}

// sizeLocked returns how many entries the ring currently holds. Caller
// must hold b.mu.
func (b *LogBuffer) sizeLocked() int {
	if b.wrapped {
		return b.cap
	}
	return b.head
}

// flushPendingLocked moves b.pending into the ring as a single line.
// Caller must hold b.mu.
func (b *LogBuffer) flushPendingLocked() {
	if b.pending.Len() == 0 {
		return
	}
	line := b.pending.String()
	b.pending.Reset()

	if b.maxLineLen > 0 && len(line) > b.maxLineLen {
		// Hard byte truncate; the TUI is responsible for ANSI-aware
		// width measurement when it renders. Trailing ellipsis tells
		// the operator the line was clipped.
		line = line[:b.maxLineLen-3] + "…"
	}

	b.lines[b.head] = line
	b.head++
	if b.head >= b.cap {
		b.head = 0
		b.wrapped = true
	}
}
