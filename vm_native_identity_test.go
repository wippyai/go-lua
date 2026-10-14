package lua

import (
	"runtime"
	"testing"
	"time"
)

func TestReviewNativeFunctionTableKey(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGlobal("native", LGoFunc(func(*LState) int { return 0 }))
	if err := L.DoString(`local t = {}; t[native] = 42; assert(t[native] == 42); t[native] = nil; assert(next(t) == nil)`); err != nil {
		t.Fatal(err)
	}
}

func TestReviewNativeComparisonMetamethod(t *testing.T) {
	for _, op := range []string{"==", "<", "<="} {
		t.Run(op, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			L.SetGlobal("compare", LGoFunc(func(L *LState) int { L.Push(LTrue); return 1 }))
			if err := L.DoString(`local mt = {__eq=compare, __lt=compare, __le=compare}; local a=setmetatable({},mt); local b=setmetatable({},mt); assert(a ` + op + ` b)`); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReviewDoubleClosePoolIsolation(t *testing.T) {
	L := NewState()
	L.Close()
	L.Close()
	if L.stop != 1 {
		t.Fatal("Close admitted duplicate disposal")
	}
	a := NewState()
	b := NewState()
	if a == b {
		a.Close()
		t.Fatal("two live states share the same pooled LState after double Close")
	}
	defer a.Close()
	defer b.Close()
	a.SetGlobal("owner", LString("a"))
	b.SetGlobal("owner", LString("b"))
	if a.GetGlobal("owner") != LString("a") {
		t.Fatal("globals crossed state boundary")
	}
}

func nativeKeyOne(L *LState) int { L.Push(LNumber(1)); return 1 }
func nativeKeyTwo(L *LState) int { L.Push(LNumber(2)); return 1 }

func TestNativeFunctionKeysMutation(t *testing.T) {
	L := NewState()
	defer L.Close()
	one, two := LGoFunc(nativeKeyOne), LGoFunc(nativeKeyTwo)
	L.SetGlobal("one", one)
	L.SetGlobal("two", two)
	if err := L.DoString(`
  local a,b = {},{}
  a[one],a[two] = 11,22
  b[one] = 33
  assert(one ~= two and rawequal(one,one))
  assert(a[one] == 11 and a[two] == 22 and b[one] == 33)
  local seen = 0
  for k,v in pairs(a) do
   assert(type(k) == "function")
   assert((rawequal(k,one) and k()==1 and v==11) or (rawequal(k,two) and k()==2 and v==22))
   a[k]=nil
   seen=seen+1
  end
  assert(seen==2 and next(a)==nil)
  a[one]=44
  assert(a[one]==44 and b[one]==33)
 `); err != nil {
		t.Fatal(err)
	}
	table := L.NewTable()
	table.RawSetH(one, LTrue)
	table.RawSetH(two, LFalse)
	table.RawSetH(one, LNumber(7))
	if len(table.Dict) != 2 || table.RawGetH(one) != LNumber(7) {
		t.Fatal("overwrite changed key identity")
	}
	visits := 0
	table.ForEach(func(key, value LValue) {
		if _, ok := key.(LGoFunc); !ok {
			t.Fatalf("iteration exposed %T", key)
		}
		if table.RawGet(key) != value {
			t.Fatal("iteration key cannot look up its value")
		}
		visits++
	})
	if visits != 2 {
		t.Fatalf("visits=%d", visits)
	}
	// A Go consumer copying the exported dictionary must preserve lookup.
	copy := L.NewTable()
	copy.Dict = make(map[LValue]LValue)
	for key, value := range table.Dict {
		copy.Dict[key] = value
	}
	if copy.RawGet(one) != LNumber(7) {
		t.Fatal("copied dictionary lost native key")
	}
}

func TestNativeComparisonMetamethodYield(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGlobal("compare", LGoFunc(func(L *LState) int { return L.Yield(LString("comparison")) }))
	for _, op := range []string{"==", "<", "<="} {
		if err := L.DoString(`
   local mt={__eq=compare,__lt=compare,__le=compare}
   local a,b=setmetatable({},mt),setmetatable({},mt)
   local co=coroutine.create(function() return a ` + op + ` b end)
   local ok,value=coroutine.resume(co)
   assert(ok and value=="comparison")
   ok,value=coroutine.resume(co,true)
   assert(ok and value==true and coroutine.status(co)=="dead")
  `); err != nil {
			t.Fatalf("%s: %v", op, err)
		}
	}
}

func TestNativeComparisonDifferentMetamethods(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGlobal("one", LGoFunc(nativeKeyOne))
	L.SetGlobal("two", LGoFunc(nativeKeyTwo))
	if err := L.DoString(`
  local a=setmetatable({}, {__eq=one,__lt=one})
  local b=setmetatable({}, {__eq=two,__lt=two})
  assert(not (a==b))
  local ok=pcall(function() return a<b end)
  assert(not ok)
 `); err != nil {
		t.Fatal(err)
	}
}

func TestCloseTwiceWithoutPooling(t *testing.T) {
	L := NewState(Options{RegistrySize: 128, RegistryGrowStep: 128, RegistryMaxSize: 4096})
	L.SetTop(2048)
	L.Close()
	if L.reg != nil {
		t.Fatal("test did not exercise non-pooled close")
	}
	L.Close()
	if !L.IsClosed() {
		t.Fatal("state not closed")
	}
}

func TestDeletedNativeKeyReleasesCaptures(t *testing.T) {
	collected := make(chan struct{}, 1)
	table := CreateTable(0, 0)
	func() {
		object := &LUserData{Value: make([]byte, 1024)}
		runtime.SetFinalizer(object, func(*LUserData) { collected <- struct{}{} })
		key := LGoFunc(func(L *LState) int { L.Push(object); return 1 })
		table.RawSetH(key, LTrue)
		table.Next(LNil)
		table.RawSetH(key, LNil)
	}()
	defer runtime.KeepAlive(table)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		runtime.GC()
		select {
		case <-collected:
			return
		case <-time.After(10 * time.Millisecond):
		}
	}
	t.Fatal("deleted native function key retains its captured object")
}
