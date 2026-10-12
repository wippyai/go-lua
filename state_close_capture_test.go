// SPDX-License-Identifier: MPL-2.0

package lua

import "testing"

func TestStateClosePreservesSuspendedCaptures(t *testing.T) {
	for _, pooled := range []bool{true, false} {
		name := "pooled"
		if !pooled {
			name = "unpooled"
		}
		t.Run(name, func(t *testing.T) {
			parent := NewState()
			defer parent.Close()
			fn, err := parent.LoadString(`
local value = 42
local other = "kept"
coroutine.yield(function() return value, other end, function(v) value = v end)
`)
			if err != nil {
				t.Fatal(err)
			}
			thread := parent.NewThreadWithContext(nil)
			status, values, err := parent.Resume(thread, fn)
			if err != nil || status != ResumeYield || len(values) != 2 {
				thread.Close()
				t.Fatalf("resume: status=%v values=%v err=%v", status, values, err)
			}
			read := values[0].(*LFunction)
			write := values[1].(*LFunction)
			if !pooled {
				for i := 0; i < thread.Options.RegistrySize+thread.Options.RegistryGrowStep+1; i++ {
					thread.Push(LNumber(i))
				}
				if cap(thread.reg.array) <= thread.Options.RegistrySize+thread.Options.RegistryGrowStep {
					thread.Close()
					t.Fatal("fixture did not exceed pooling threshold")
				}
			}
			if err := parent.CallByParam(P{Fn: read, NRet: 2, Protect: true}); err != nil {
				thread.Close()
				t.Fatal(err)
			}
			if !numberEquals(parent.Get(-2), 42) || parent.Get(-1) != LString("kept") {
				thread.Close()
				t.Fatalf("fixture changed captures before close: %v %v", parent.Get(-2), parent.Get(-1))
			}
			parent.Pop(2)
			thread.Close()
			for _, uv := range read.Upvalues {
				if !uv.IsClosed() || uv.reg != nil {
					t.Fatal("closed state retained an open capture or its register storage")
				}
			}
			if err := parent.CallByParam(P{Fn: read, NRet: 2, Protect: true}); err != nil {
				t.Fatal(err)
			}
			if !numberEquals(parent.Get(-2), 42) || parent.Get(-1) != LString("kept") {
				t.Fatalf("captures after close: %v %v", parent.Get(-2), parent.Get(-1))
			}
			parent.Pop(2)
			if err := parent.CallByParam(P{Fn: write, NRet: 0, Protect: true}, LNumber(77)); err != nil {
				t.Fatal(err)
			}
			if err := parent.CallByParam(P{Fn: read, NRet: 2, Protect: true}); err != nil {
				t.Fatal(err)
			}
			if !numberEquals(parent.Get(-2), 77) || parent.Get(-1) != LString("kept") {
				t.Fatal("closed captures lost their shared value")
			}
			parent.Pop(2)
		})
	}
}

func TestClosedUpvalueDropsRegisterStorage(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(42))
	uv := &Upvalue{reg: L.reg, index: L.reg.top - 1}
	uv.Close()
	if uv.reg != nil || uv.Value() != LNumber(42) {
		t.Fatal("close retained registers or lost its value")
	}
	uv.SetValue(LNumber(77))
	uv.Close()
	if uv.Value() != LNumber(77) {
		t.Fatal("closing an already closed upvalue changed its value")
	}
}
