package lua

import (
	"context"
	"reflect"
	"testing"
	"unsafe"
)

func TestResetLStateClearsEveryReference(t *testing.T) {
	L := NewState()
	defer L.Close()
	th, cancel := L.NewThread()
	defer cancel()
	other, cancel2 := L.NewThread()
	defer cancel2()
	fn, err := L.LoadString(`local t = setmetatable({}, {__index = function(_, k) coroutine.yield(k) return k end}) return t.x`)
	if err != nil {
		t.Fatal(err)
	}
	if st, _, err := L.Resume(th, fn); err != nil || st != ResumeYield {
		t.Fatalf("setup yield: %v %v", st, err)
	}
	if len(th.frameExt) == 0 {
		t.Fatal("setup: expected frame extensions")
	}
	th.holding, other.heldBy = other, th
	th.yieldCallRB = 7
	th.Dead = true

	resetLState(th)

	switch {
	case th.G != nil, th.Parent != nil, th.Env != nil, th.currentFrame != nil, th.uvcache != nil:
		t.Fatal("pooled state retains a state reference")
	case th.frameExt != nil:
		t.Fatal("pooled state retains frame extensions")
	case th.holding != nil, th.heldBy != nil:
		t.Fatal("pooled state retains hold references")
	case th.ctx != nil, th.ctxDone != nil, th.ctxCancelFn != nil:
		t.Fatal("pooled state retains a context")
	case th.yieldState != yieldNone, th.yieldCallRB != 0:
		t.Fatal("pooled state retains yield state")
	}
	for i := range th.reg.array {
		if th.reg.array[i] != LNil && th.reg.array[i] != nil {
			t.Fatalf("pooled registry slot %d retains %v", i, th.reg.array[i])
		}
	}
}

func TestPooledStateFramesDoNotRetainClosures(t *testing.T) {
	L := NewState()
	defer L.Close()
	th, cancel := L.NewThread()
	defer cancel()
	fn, err := L.LoadString(`local t = setmetatable({}, {__index = function(_, k) coroutine.yield(k) return k end}) return t.x`)
	if err != nil {
		t.Fatal(err)
	}
	if st, _, err := L.Resume(th, fn); err != nil || st != ResumeYield {
		t.Fatalf("setup yield: %v %v", st, err)
	}
	depth := th.stack.Sp()
	if depth == 0 {
		t.Fatal("setup: no frames")
	}
	resetLState(th)
	for i := 0; i < depth; i++ {
		if f := th.stack.At(i); f.Fn != nil || f.GoFunc != nil {
			t.Fatalf("pooled frame %d retains a function", i)
		}
	}
}

const recurseSource = `local function f(n) if n == 0 then return 0 end return 1 + f(n - 1) end return f(...)`

func TestPooledStateHonorsCallStackSize(t *testing.T) {
	for _, minimize := range []bool{false, true} {
		NewState(Options{CallStackSize: 16, MinimizeStackMemory: minimize}).Close()
		L := NewState(Options{CallStackSize: 1024, MinimizeStackMemory: minimize})
		fn, err := L.LoadString(recurseSource)
		if err != nil {
			t.Fatal(err)
		}
		L.Push(fn)
		L.Push(LNumber(500))
		if err := L.PCall(1, 1, nil); err != nil {
			t.Fatalf("minimize=%v: %v", minimize, err)
		}
		L.Close()
	}
}

func TestPooledThreadHonorsCallStackSize(t *testing.T) {
	for _, minimize := range []bool{false, true} {
		L := NewState(Options{CallStackSize: 1024, MinimizeStackMemory: minimize})
		NewState(Options{CallStackSize: 16, MinimizeStackMemory: minimize}).Close()
		fn, err := L.LoadString(recurseSource)
		if err != nil {
			t.Fatal(err)
		}
		th, cancel := L.NewThread()
		st, ret, err := L.Resume(th, fn, LNumber(500))
		cancel()
		if err != nil || st != ResumeOK {
			t.Fatalf("minimize=%v: %v %v %v", minimize, st, ret, err)
		}
		L.Close()
	}
}

func TestPooledThreadHonorsRegistryLimits(t *testing.T) {
	L := NewState(Options{RegistrySize: 256, RegistryMaxSize: 4096, RegistryGrowStep: 64})
	defer L.Close()
	NewState(Options{RegistrySize: 256, RegistryMaxSize: 1 << 20, RegistryGrowStep: 1024}).Close()
	th, cancel := L.NewThread()
	defer cancel()
	if th.reg.maxSize != 4096 || th.reg.growBy != 64 {
		t.Fatalf("thread registry limits %d/%d, want 4096/64", th.reg.maxSize, th.reg.growBy)
	}
}

func TestPooledStateRegistryRespectsMaxSize(t *testing.T) {
	NewState(Options{RegistrySize: 4096, RegistryMaxSize: 4096}).Close()
	L := NewState(Options{RegistrySize: 256, RegistryMaxSize: 512})
	defer L.Close()
	if n := cap(L.reg.array); n > 512 {
		t.Fatalf("registry holds %d slots, limit is 512", n)
	}
}

// Fields a pooled state keeps across reset: the registry and call stack are
// reused allocations cleared in place, mainLoop and Options hold no
// references, and stop stays set so a closed state reports IsClosed.
var pooledStateKeptFields = map[string]bool{
	"reg": true, "stack": true, "mainLoop": true, "Options": true, "stop": true,
}

// seedField sets a field of any kind to a non-zero value; fields whose kind
// the test cannot seed fail the test, so a new field must be handled here.
func seedField(t *testing.T, name string, f reflect.Value) {
	t.Helper()
	switch f.Kind() {
	case reflect.Ptr:
		f.Set(reflect.New(f.Type().Elem()))
	case reflect.Map:
		f.Set(reflect.MakeMap(f.Type()))
	case reflect.Slice:
		f.Set(reflect.MakeSlice(f.Type(), 1, 1))
	case reflect.Chan:
		f.Set(reflect.MakeChan(reflect.ChanOf(reflect.BothDir, f.Type().Elem()), 0).Convert(f.Type()))
	case reflect.Func:
		f.Set(reflect.MakeFunc(f.Type(), func([]reflect.Value) []reflect.Value {
			return make([]reflect.Value, f.Type().NumOut())
		}))
	case reflect.Interface:
		switch f.Type() {
		case reflect.TypeOf((*context.Context)(nil)).Elem():
			f.Set(reflect.ValueOf(context.Background()))
		default:
			t.Fatalf("field %s: no seed for interface %s", name, f.Type())
		}
	case reflect.Bool:
		f.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		f.SetInt(7)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		f.SetUint(7)
	default:
		t.Fatalf("field %s: no seed for kind %s", name, f.Kind())
	}
}

func TestResetLStateClearsEveryField(t *testing.T) {
	L := NewState()
	defer L.Close()
	th, cancel := L.NewThread()
	defer cancel()

	v := reflect.ValueOf(th).Elem()
	for i := 0; i < v.NumField(); i++ {
		name := v.Type().Field(i).Name
		if pooledStateKeptFields[name] {
			continue
		}
		f := reflect.NewAt(v.Field(i).Type(), unsafe.Pointer(v.Field(i).UnsafeAddr())).Elem()
		seedField(t, name, f)
	}
	// Holds must be consistent for releaseHold to tear them down.
	other, cancel2 := L.NewThread()
	defer cancel2()
	th.holding, th.heldBy = other, other
	other.heldBy, other.holding = th, th

	resetLState(th)

	defaultPanic := reflect.ValueOf(panicWithTraceback).Pointer()
	for i := 0; i < v.NumField(); i++ {
		name := v.Type().Field(i).Name
		if pooledStateKeptFields[name] {
			continue
		}
		f := v.Field(i)
		switch f.Kind() {
		case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Chan, reflect.Interface:
			if !f.IsNil() {
				t.Errorf("pooled state retains %s", name)
			}
		case reflect.Func:
			if name != "Panic" {
				if !f.IsNil() {
					t.Errorf("pooled state retains %s", name)
				}
			} else if f.IsNil() || f.Pointer() != defaultPanic {
				t.Errorf("pooled state keeps a custom Panic callback")
			}
		default:
			if !f.IsZero() {
				t.Errorf("pooled state retains %s = %v", name, f)
			}
		}
	}
}
