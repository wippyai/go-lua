package lua

import "testing"

func TestEscapedUpvalueSurvivesStateClose(t *testing.T) {
	L := NewState()
	defer L.Close()
	fn, err := L.LoadString(`
local x = 41
export = function() x = x + 1 return x end
coroutine.yield(1)
return 0`)
	if err != nil {
		t.Fatal(err)
	}
	co, cancel := L.NewThread()
	defer cancel()
	if st, _, err := L.Resume(co, fn); err != nil || st != ResumeYield {
		t.Fatalf("resume: %v %v", st, err)
	}
	export := L.GetGlobal("export").(*LFunction)
	co.Close()

	// A new user of the pooled state overwrites its registers.
	other, cancel2 := L.NewThread()
	defer cancel2()
	filler, err := L.LoadString(`local a, b, c, d, e = 900, 901, 902, 903, 904 coroutine.yield(a) return a`)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := L.Resume(other, filler); err != nil {
		t.Fatal(err)
	}

	L.Push(export)
	if err := L.PCall(0, 1, nil); err != nil {
		t.Fatal(err)
	}
	if got := L.Get(-1); got != LNumber(42) {
		t.Fatalf("escaped upvalue read %v, want 42", got)
	}
	L.SetTop(0)
}
