package lua

import (
	"strings"
	"testing"
	"time"
)

func TestPathologicalPatternRaisesLimitError(t *testing.T) {
	funcs := []string{
		`string.find(s, p)`,
		`string.match(s, p)`,
		`string.gsub(s, p, 'x')`,
		`string.gmatch(s, p)()`,
	}
	for _, call := range funcs {
		t.Run(call, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			fn, err := L.LoadString(`
local s = string.rep('a', 4000)
local p = ('a*'):rep(12) .. 'b'
return ` + call)
			if err != nil {
				t.Fatal(err)
			}
			th, cancel := L.NewThread()
			defer cancel()
			start := time.Now()
			_, _, err = L.Resume(th, fn)
			if err == nil || !strings.Contains(err.Error(), "pattern match") || !strings.Contains(err.Error(), "limit exceeded") {
				t.Fatalf("error = %v", err)
			}
			if d := time.Since(start); d > 5*time.Second {
				t.Fatalf("took %v", d)
			}
		})
	}
}

func TestLongPatternMatchObservesContextCancellation(t *testing.T) {
	L := NewState()
	defer L.Close()
	fn, err := L.LoadString(`
local s = string.rep('a', 100000)
local p = ('a'):rep(4000) .. 'b'
return string.find(s, p)`)
	if err != nil {
		t.Fatal(err)
	}
	th, cancel := L.NewThread()
	time.AfterFunc(50*time.Millisecond, cancel)
	start := time.Now()
	_, _, err = L.Resume(th, fn)
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("error = %v, want context cancellation", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("cancellation took %v", d)
	}
}
