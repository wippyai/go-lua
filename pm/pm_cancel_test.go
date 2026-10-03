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
