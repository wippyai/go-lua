package lua

import (
	"testing"
)

func TestWrappedChildErrorTearsDownChild(t *testing.T) {
	L := NewState()
	defer L.Close()
	var child *LState
	L.SetGlobal("note", L.NewFunction(func(L *LState) int {
		child = L
		return 0
	}))
	fn, err := L.LoadString(`
local w = coroutine.wrap(function() note() error("boom") end)
local ok = pcall(w)
return ok`)
	if err != nil {
		t.Fatal(err)
	}
	L.Push(fn)
	if err := L.PCall(0, 1, nil); err != nil {
		t.Fatal(err)
	}
	if child == nil || !child.Dead {
		t.Fatal("failed wrapped child is not dead")
	}
	if child.Parent != nil {
		t.Fatal("failed wrapped child keeps its resumer")
	}
	if L.G.CurrentThread != L {
		t.Fatal("current thread is not restored after a wrapped child failed")
	}
}
