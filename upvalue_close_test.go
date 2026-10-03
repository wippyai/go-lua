package lua

import (
	"runtime"
	"testing"
)

func TestClosedUpvalueDoesNotRetainState(t *testing.T) {
	L := NewState()
	defer L.Close()
	done := make(chan struct{})
	func() {
		fn, err := L.LoadString(`local keep = ... local x = 1 export = function() return x end return 0`)
		if err != nil {
			t.Fatal(err)
		}
		sentinel := L.NewTable()
		runtime.SetFinalizer(sentinel, func(*LTable) { close(done) })
		co, cancel := L.NewThread()
		defer cancel()
		if st, _, err := L.Resume(co, fn, sentinel); err != nil || st != ResumeOK {
			t.Fatalf("resume: %v %v", st, err)
		}
	}()
	if waitCollected(done) {
		return
	}
	t.Fatal("a closed upvalue keeps the finished thread's registers reachable")
}
