package lua_test

import (
	"sync"
	"testing"

	lua "github.com/wippyai/go-lua"
	"github.com/wippyai/go-lua/compiler/bytecode"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/compiler/parse"
	typeio "github.com/wippyai/go-lua/types/io"
)

func argumentFunction(t *testing.T, source, name string, dumped bool) (*lua.LState, *lua.LFunction) {
	t.Helper()
	chunk, err := parse.ParseString(source, "arguments")
	if err != nil {
		t.Fatal(err)
	}
	session := testutil.NewChecker(testutil.WithStdlib()).CheckChunk(chunk, "arguments")
	defer session.Release()
	info, err := session.ExportManifest("arguments").Encode()
	if err != nil {
		t.Fatal(err)
	}
	proto, err := lua.CompileWithOptions(chunk, "arguments", lua.CompileOptions{TypeInfo: info})
	if err != nil {
		t.Fatal(err)
	}
	if dumped {
		data, err := bytecode.Dump(proto)
		if err != nil {
			t.Fatal(err)
		}
		proto, err = bytecode.Undump(data)
		if err != nil {
			t.Fatal(err)
		}
	}
	l := lua.NewState()
	t.Cleanup(l.Close)
	l.OpenLibs()
	if err := l.CallByParam(lua.P{Fn: l.LoadProto(proto), NRet: 1, Protect: true}); err != nil {
		t.Fatal(err)
	}
	value := l.Get(-1)
	if table, ok := value.(*lua.LTable); ok {
		value = table.RawGetString(name)
	}
	fn, ok := value.(*lua.LFunction)
	if !ok {
		t.Fatalf("expected function, got %v", value)
	}
	return l, fn
}

func TestArgumentContracts(t *testing.T) {
	cases := []struct {
		name, source, method string
		args                 []lua.LValue
		invalid              bool
	}{
		{"typed-valid", `return function(id: string) return id end`, "", []lua.LValue{lua.LString("ok")}, false},
		{"typed-invalid", `return function(id: string) return id end`, "", []lua.LValue{lua.LInteger(42)}, true},
		{"required-missing", `return function(id: string) return id end`, "", nil, true},
		{"optional-missing", `return function(id: string?) return id end`, "", nil, false},
		{"optional-invalid", `return function(id: string?) return id end`, "", []lua.LValue{lua.LInteger(42)}, true},
		{"mixed-untyped-omitted", `return function(id: string, value) return id end`, "", []lua.LValue{lua.LString("ok")}, false},
		{"untyped", `return function(id) return id end`, "", []lua.LValue{lua.LInteger(42)}, false},
		{"inferred-not-declared", `local f: fun(id: string): string = function(id) return id end; return f`, "", []lua.LValue{lua.LInteger(42)}, false},
		{"alias-invalid", `local function impl(id: string) return id end; return {run=impl}`, "run", []lua.LValue{lua.LInteger(42)}, true},
		{"same-line-second", `return {first=function(id: string) return id end, run=function(id: integer) return id end}`, "run", []lua.LValue{lua.LString("wrong")}, true},
		{"variadic-valid", `return function(...: string) return 1 end`, "", []lua.LValue{lua.LString("a"), lua.LString("b")}, false},
		{"variadic-invalid", `return function(...: string) return 1 end`, "", []lua.LValue{lua.LString("a"), lua.LInteger(1)}, true},
		{"extra-args-unchanged", `return function(id: string) return id end`, "", []lua.LValue{lua.LString("a"), lua.LInteger(1)}, false},
		{"nested-record", `type User = {name: string}; return function(user: User) return user.name end`, "", []lua.LValue{lua.CreateTable(0, 0)}, true},
		{"undefined-type", `return function(id: Missing) return id end`, "", []lua.LValue{lua.LInteger(42)}, true},
		{"generic-alias", `type Box<T> = {value: T}; return function(box: Box<string>) return box end`, "", []lua.LValue{lua.CreateTable(0, 0)}, true},
		{"recursive-alias", `type Node = {name: string, next: Node?}; return function(node: Node) return node end`, "", []lua.LValue{lua.CreateTable(0, 0)}, true},
		{"implicit-self", `local M = {}; function M:run(id: string) return id end; return M`, "run", []lua.LValue{lua.CreateTable(0, 0), lua.LInteger(42)}, true},
		{"raw-nil", `return function(id: string) return id end`, "", []lua.LValue{nil}, true},
		{"raw-nil-optional", `return function(id: string?) return id end`, "", []lua.LValue{nil}, false},
		{"constraint", `return function(score: number @min(0)) return score end`, "", []lua.LValue{lua.LInteger(-1)}, true},
	}
	for _, c := range cases {
		for _, dumped := range []bool{false, true} {
			t.Run(c.name+map[bool]string{false: "/source", true: "/bytecode"}[dumped], func(t *testing.T) {
				l, fn := argumentFunction(t, c.source, c.method, dumped)
				err := fn.Proto.CheckArguments(l, c.args)
				if (err != nil) != c.invalid {
					t.Fatalf("CheckArguments = %v, invalid=%v", err, c.invalid)
				}
			})
		}
	}
}

func TestArgumentContractsConcurrent(t *testing.T) {
	_, fn := argumentFunction(t, `return function(id: string) return id end`, "", false)
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l := lua.NewState()
			defer l.Close()
			for range 20 {
				if err := fn.Proto.CheckArguments(l, []lua.LValue{lua.LString("ok")}); err != nil {
					t.Error(err)
				}
				if err := fn.Proto.CheckArguments(l, []lua.LValue{lua.LInteger(42)}); err == nil {
					t.Error("accepted invalid argument")
				}
			}
		}()
	}
	wg.Wait()
}

func TestArgumentContractsCorruptMetadata(t *testing.T) {
	l := lua.NewState()
	defer l.Close()
	proto := &lua.FunctionProto{ArgumentInfo: []byte("invalid")}
	first := proto.CheckArguments(l, nil)
	second := proto.CheckArguments(l, nil)
	if first == nil || second == nil || first == second {
		t.Fatal("malformed metadata must fail closed with independent errors")
	}
}

func TestArgumentContractsKeepConstraintDetails(t *testing.T) {
	l, fn := argumentFunction(t, `return function(score: number @min(0)) return score end`, "", true)
	err, ok := fn.Proto.CheckArguments(l, []lua.LValue{lua.LInteger(-1)}).(*lua.Error)
	if !ok || err.Kind() != lua.Invalid || err.Details()["argument"] != 1 || err.Details()["constraint"] == nil {
		t.Fatalf("missing validation details: %v", err)
	}
}

func TestArgumentContractsCompileIndexNotRetained(t *testing.T) {
	_, fn := argumentFunction(t, `return function(id: string) return id end`, "", false)
	manifest, err := typeio.DecodeManifest(fn.Proto.TypeInfo)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.ArgumentContracts) != 0 || len(fn.Proto.ArgumentInfo) == 0 {
		t.Fatal("runtime proto must keep only its own contract, not the module index")
	}
}

func TestArgumentContractsDoNotChangeVMCalls(t *testing.T) {
	l, fn := argumentFunction(t, `return function(id: string) return id end`, "", false)
	if err := l.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, lua.LInteger(42)); err != nil {
		t.Fatal(err)
	}
	if l.Get(-1) != lua.LInteger(42) {
		t.Fatal("ordinary Lua call semantics changed")
	}
}
