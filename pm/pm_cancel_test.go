package pm

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestFind_PathologicalPatternHitsLimitQuickly(t *testing.T) {
	src := []byte(strings.Repeat("a", 4000))
	start := time.Now()
	_, err := Find(strings.Repeat("a*", 12)+"b", src, 0, 1)
	if err == nil || !strings.Contains(err.Error(), "limit exceeded") {
		t.Fatalf("error = %v, want a limit error", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("limit error took %v", d)
	}
}

func TestProgramFind_StopsOnCancellation(t *testing.T) {
	// Every start position runs about four thousand steps without backtracking
	// or scanning, and the search tries a hundred thousand positions.
	pattern := strings.Repeat("a", 4000) + "b"
	program, err := Compile(pattern)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte(strings.Repeat("a", 100000))
	done := make(chan struct{})
	time.AfterFunc(50*time.Millisecond, func() { close(done) })

	start := time.Now()
	_, err = program.WithDone(done).Find(src, 0, 1)
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("error = %v, want ErrCanceled", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("cancellation took %v", d)
	}
}

func TestProgramFindOne_StopsOnCancellation(t *testing.T) {
	program, err := Compile(strings.Repeat("a", 4000) + "b")
	if err != nil {
		t.Fatal(err)
	}
	src := []byte(strings.Repeat("a", 100000))
	done := make(chan struct{})
	close(done)
	if _, err := program.WithDone(done).FindOne(src, 0); !errors.Is(err, ErrCanceled) {
		t.Fatalf("error = %v, want ErrCanceled", err)
	}
}

type scanCountingClass struct {
	checks      int
	cancelAfter int
	done        chan struct{}
}

func (c *scanCountingClass) matches(int) bool {
	c.checks++
	if c.checks == c.cancelAfter {
		close(c.done)
	}
	return true
}

func TestScanRepeat_StopsBeforeExhaustingCanceledInput(t *testing.T) {
	done := make(chan struct{})
	cls := &scanCountingClass{cancelAfter: 10, done: done}
	v := &vm{src: make([]byte, 4*(cancelCheckMask+1)), done: done}
	_, err := v.scanRepeat(cls, 0)
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("error = %v, want ErrCanceled", err)
	}
	if cls.checks > cancelCheckMask+1 {
		t.Fatalf("scanned %d bytes after cancellation, want at most one check interval", cls.checks)
	}
}

func TestScanRepeat_StopsAtRemainingByteScanBudget(t *testing.T) {
	cls := &scanCountingClass{}
	v := &vm{src: make([]byte, 64), byteScans: MaxVMByteScans - 8}
	_, err := v.scanRepeat(cls, 0)
	if err == nil || !strings.Contains(err.Error(), "byte scan limit exceeded") {
		t.Fatalf("error = %v, want byte scan limit error", err)
	}
	// The byte at the boundary may be inspected to distinguish a matching
	// continuation from a terminating non-match, but the rest must stay unread.
	if cls.checks > 9 {
		t.Fatalf("scanned %d bytes with only 8 left in the budget", cls.checks)
	}
}

func TestScanRepeat_ExactBudgetStillAllowsTermination(t *testing.T) {
	for _, src := range []string{"aaaaaaaa", "aaaaaaaab"} {
		v := &vm{src: []byte(src), byteScans: MaxVMByteScans - 8}
		end, err := v.scanRepeat(&literalClass{char: 'a'}, 0)
		if err != nil || end != 8 || v.byteScans != MaxVMByteScans {
			t.Fatalf("input %q: end=%d scans=%d error=%v", src, end, v.byteScans, err)
		}
	}
}

func TestVMByteScanHelpers_ObserveCancellation(t *testing.T) {
	done := make(chan struct{})
	close(done)
	t.Run("balanced", func(t *testing.T) {
		v := &vm{src: []byte("(abc)"), done: done}
		_, _, err := v.matchBrace('(', ')', 0)
		if !errors.Is(err, ErrCanceled) {
			t.Fatalf("error = %v, want ErrCanceled", err)
		}
	})
	t.Run("backreference", func(t *testing.T) {
		md := newMatchData(4)
		defer md.release()
		md.setCapture(2, 0)
		md.setCapture(3, 3)
		v := &vm{src: []byte("abcabc"), matchData: md, done: done}
		_, _, err := v.matchBackref(1, 3)
		if !errors.Is(err, ErrCanceled) {
			t.Fatalf("error = %v, want ErrCanceled", err)
		}
	})
}
