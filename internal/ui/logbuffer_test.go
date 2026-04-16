package ui_test

import (
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ruptor-dev/cli/internal/ui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLogBuffer_WriteSplitsOnNewline asserts the io.Writer contract:
// each '\n' becomes a discrete entry in the ring; trailing partial
// lines are held until the next newline.
func TestLogBuffer_WriteSplitsOnNewline(t *testing.T) {
	b := ui.NewLogBuffer(8, 0)

	_, err := b.Write([]byte("alpha\nbeta\ngamma"))
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "beta"}, b.Lines(0),
		"only complete lines should be readable; 'gamma' waits for the next newline")

	_, err = b.Write([]byte("\n"))
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "beta", "gamma"}, b.Lines(0))
}

// TestLogBuffer_WriteAcrossCalls covers the realistic zerolog pattern
// where a single log event arrives across multiple Write calls.
func TestLogBuffer_WriteAcrossCalls(t *testing.T) {
	b := ui.NewLogBuffer(8, 0)
	for _, chunk := range []string{"INF ", "proxy ", "started", "\n"} {
		_, err := b.Write([]byte(chunk))
		require.NoError(t, err)
	}
	assert.Equal(t, []string{"INF proxy started"}, b.Lines(0))
}

// TestLogBuffer_RingWrapsAtCapacity asserts the ring drops the oldest
// entries once capacity is exceeded.
func TestLogBuffer_RingWrapsAtCapacity(t *testing.T) {
	b := ui.NewLogBuffer(3, 0)
	for i := 0; i < 5; i++ {
		_, err := b.Write([]byte(fmt.Sprintf("line%d\n", i)))
		require.NoError(t, err)
	}
	assert.Equal(t, []string{"line2", "line3", "line4"}, b.Lines(0),
		"the three most recent entries should survive; older entries must be dropped")
}

// TestLogBuffer_LinesNCapped exercises the n-cap behaviour: caller
// asks for fewer than the buffer holds.
func TestLogBuffer_LinesNCapped(t *testing.T) {
	b := ui.NewLogBuffer(8, 0)
	for i := 0; i < 6; i++ {
		_, err := fmt.Fprintf(b, "line%d\n", i)
		require.NoError(t, err)
	}

	assert.Equal(t, []string{"line5"}, b.Lines(1), "n=1 returns the single most recent line")
	assert.Equal(t, []string{"line3", "line4", "line5"}, b.Lines(3))
	assert.Len(t, b.Lines(0), 6, "n<=0 returns the entire buffer")
	assert.Len(t, b.Lines(100), 6, "n>size collapses to the buffer's actual size")
}

// TestLogBuffer_TruncatesLongLines asserts the maxLineLen contract:
// lines exceeding the cap are byte-truncated with a trailing ellipsis
// so the TUI render width stays bounded.
func TestLogBuffer_TruncatesLongLines(t *testing.T) {
	b := ui.NewLogBuffer(2, 10)
	_, err := b.Write([]byte(strings.Repeat("x", 50) + "\n"))
	require.NoError(t, err)

	got := b.Lines(1)
	require.Len(t, got, 1)
	assert.Len(t, got[0], 10, "truncated line must respect the cap including the ellipsis byte width")
	assert.True(t, strings.HasSuffix(got[0], "…"), "truncation must signal with the ellipsis rune")
}

// TestLogBuffer_FlushPromotesPartial confirms shutdown-time flush
// behaviour: the operator does not lose the last partial line.
func TestLogBuffer_FlushPromotesPartial(t *testing.T) {
	b := ui.NewLogBuffer(4, 0)
	_, err := b.Write([]byte("partial-no-newline"))
	require.NoError(t, err)

	assert.Empty(t, b.Lines(0), "no newline yet — partial must not be visible")

	b.Flush()
	assert.Equal(t, []string{"partial-no-newline"}, b.Lines(0))
}

// TestLogBuffer_ConcurrentWriteRead exercises the mutex under realistic
// pressure: many writers + a reader polling like the TUI tick loop.
// Verifies (a) -race cleanliness, (b) reader never panics or returns
// malformed data, (c) every committed line is visible at the end.
func TestLogBuffer_ConcurrentWriteRead(t *testing.T) {
	const writers = 8
	const perWriter = 200
	capacity := writers * perWriter
	b := ui.NewLogBuffer(capacity, 0)

	var writeWG sync.WaitGroup
	writeWG.Add(writers)
	for w := 0; w < writers; w++ {
		w := w
		go func() {
			defer writeWG.Done()
			for i := 0; i < perWriter; i++ {
				_, err := fmt.Fprintf(b, "writer=%d seq=%d\n", w, i)
				if err != nil {
					t.Errorf("write failed: %v", err)
					return
				}
			}
		}()
	}

	// Reader runs until the writers signal done. Any race on mutable
	// state surfaces here under `go test -race`.
	var writesFinished atomic.Bool
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for !writesFinished.Load() {
			_ = b.Lines(50)
			runtime.Gosched()
		}
	}()

	writeWG.Wait()
	writesFinished.Store(true)
	<-readerDone

	final := b.Lines(0)
	assert.Len(t, final, writers*perWriter, "every committed line must be present once writers are done")
}

// Compile-time guard: LogBuffer satisfies both interfaces it claims.
var (
	_ io.Writer    = (*ui.LogBuffer)(nil)
	_ ui.LogReader = (*ui.LogBuffer)(nil)
)
