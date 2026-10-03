package lua

import "testing"

func TestResumeRejectedThreadIsUntouched(t *testing.T) {
	L := NewState()
	defer L.Close()
	fn, err := L.LoadString(`return 1`)
	if err != nil {
		t.Fatal(err)
	}
	th, cancel := L.NewThread()
	defer cancel()
	if st, _, err := L.Resume(th, fn); err != nil || st != ResumeOK {
		t.Fatalf("first resume: %v %v", st, err)
	}
	if _, _, err := L.Resume(th, fn); err == nil {
		t.Fatal("resuming a dead thread succeeded")
	}
	if th.stack.Sp() != 0 {
		t.Fatalf("a rejected resume left %d frames on the dead thread", th.stack.Sp())
	}
}

func TestResumeClosedThreadIsRejected(t *testing.T) {
	L := NewState()
	defer L.Close()
	fn, err := L.LoadString(`return 1`)
	if err != nil {
		t.Fatal(err)
	}
	th, cancel := L.NewThread()
	cancel()
	th.Close()
	st, _, err := L.Resume(th, fn)
	if st != ResumeError || err == nil {
		t.Fatalf("resuming a closed thread: %v %v", st, err)
	}
	st, _, err = L.ResumeInto(th, fn, nil)
	if st != ResumeError || err == nil {
		t.Fatalf("resuming a closed thread into a buffer: %v %v", st, err)
	}
}
