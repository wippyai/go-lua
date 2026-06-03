package canonical_test

import (
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/kind"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"testing"
)

func TestCanonicalGenericInlineCallbackParamIsCallClosed(t *testing.T) {
	res := testutil.Check(`
		local function transform<T, U>(x: T, fn: (T) -> U): U
			return fn(x)
		end
		local n = transform("hello", function(s) return #s end)
		local len: integer = n
	`, testutil.WithStdlib())

	fn := findFunctionWithParamNames(t, res.Session.Results, "s")
	sym := singleSymbolNamed(t, fn.Graph, "s")
	got := fn.NarrowedTypeAt(fn.Graph.Entry(), constraint.NewPath(sym, "s"))
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("callback param entry type = %v, want string; root expected args=%v diagnostics=%v", got, res.Session.RootResult.CallExpectedArgs, testutil.ErrorMessages(res.Diagnostics))
	}
	if res.HasError() {
		t.Fatalf("expected clean check, got diagnostics: %v", testutil.ErrorMessages(res.Diagnostics))
	}
}

func TestCanonicalGenericInlineCallbackReturnInferredWithoutSink(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{
			name: "length return",
			code: `
				local function transform<T, U>(x: T, fn: (T) -> U): U
					return fn(x)
				end
				local n = transform("hello", function(s) return #s end)
			`,
		},
		{
			name: "arithmetic return",
			code: `
				local function apply<T, U>(x: T, fn: (T) -> U): U
					return fn(x)
				end
				local n = apply(10, function(num) return num + 1 end)
			`,
		},
		{
			name: "record field return",
			code: `
				local function apply<T, U>(x: T, fn: (T) -> U): U
					return fn(x)
				end
				local n = apply({count = 1}, function(r) return r.count end)
			`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := testutil.Check(tt.code, testutil.WithStdlib())
			got := rootSymbolTypeAtExit(t, res, "n")
			if !integerCompatibleType(got) {
				t.Fatalf("inferred n type = %v, want integer-compatible without typed sink; diagnostics=%v", got, testutil.ErrorMessages(res.Diagnostics))
			}
			if res.HasError() {
				t.Fatalf("expected clean check, got diagnostics: %v", testutil.ErrorMessages(res.Diagnostics))
			}
		})
	}
}

func TestCanonicalGenericKeysReturnIndexInferredWithoutSink(t *testing.T) {
	res := testutil.Check(`
		local function keys<K, V>(t: {[K]: V}): {K}
			local result: {K} = {}
			for k in pairs(t) do
				result[#result + 1] = k
			end
			return result
		end
		local data: {[string]: number} = {["a"] = 1, ["b"] = 2}
		local ks = keys(data)
		local first = ks[1]
	`, testutil.WithStdlib())

	got := rootSymbolTypeAtExit(t, res, "first")
	if !presentStringCompatibleType(got) {
		t.Fatalf("inferred first type = %v, want non-optional string-compatible without typed sink; diagnostics=%v", got, testutil.ErrorMessages(res.Diagnostics))
	}
	if res.HasError() {
		t.Fatalf("expected clean check, got diagnostics: %v", testutil.ErrorMessages(res.Diagnostics))
	}
}

func TestCanonicalGenericInstantiatedRecordMethodReturnWithoutSink(t *testing.T) {
	res := testutil.Check(`
		type Container<T> = {
			value: T,
			get: fun(self: self): T
		}

		local function make_container<T>(v: T): Container<T>
			return {
				value = v,
				get = function(self): T return self.value end
			}
		end

		local c = make_container("hello")
		local s = c:get()
	`, testutil.WithStdlib())

	got := rootSymbolTypeAtExit(t, res, "s")
	if !presentStringCompatibleType(got) {
		cType := rootSymbolTypeAtExit(t, res, "c")
		methodType, methodOK := querycore.Method(cType, "get")
		t.Fatalf("inferred s type = %v, want non-optional string-compatible without typed sink; c type=%v; method=%v/%v; diagnostics=%v", got, cType, methodType, methodOK, testutil.ErrorMessages(res.Diagnostics))
	}
	if res.HasError() {
		t.Fatalf("expected clean check, got diagnostics: %v", testutil.ErrorMessages(res.Diagnostics))
	}
}

func TestCanonicalGenericInlineCallbackNumberParamIsCallClosed(t *testing.T) {
	res := testutil.Check(`
		local function apply<T, U>(x: T, fn: (T) -> U): U
			return fn(x)
		end
		local n = apply(10, function(num) return num + 1 end)
		local out: integer = n
	`, testutil.WithStdlib())

	outer := findFunctionWithParamNames(t, res.Session.Results, "x", "fn")
	outerSym := singleSymbolNamed(t, outer.Graph, "x")
	outerGot := outer.NarrowedTypeAt(outer.Graph.Entry(), constraint.NewPath(outerSym, "x"))
	if !typ.TypeEquals(outerGot, typ.Integer) && !typ.TypeEquals(outerGot, typ.LiteralInt(10)) {
		t.Fatalf("outer generic param entry type = %v, want integer-compatible closed value; root expected args=%v diagnostics=%v", outerGot, res.Session.RootResult.CallExpectedArgs, testutil.ErrorMessages(res.Diagnostics))
	}

	fn := findFunctionWithParamNames(t, res.Session.Results, "num")
	sym := singleSymbolNamed(t, fn.Graph, "num")
	got := fn.NarrowedTypeAt(fn.Graph.Entry(), constraint.NewPath(sym, "num"))
	if !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("callback param entry type = %v, want integer; root expected args=%v diagnostics=%v", got, res.Session.RootResult.CallExpectedArgs, testutil.ErrorMessages(res.Diagnostics))
	}
	if res.HasError() {
		t.Fatalf("expected clean check, got diagnostics: %v", testutil.ErrorMessages(res.Diagnostics))
	}
}

func rootSymbolTypeAtExit(t *testing.T, res *testutil.Result, name string) typ.Type {
	t.Helper()
	if res == nil || res.Session == nil || res.Session.RootResult == nil || res.Session.RootResult.Graph == nil {
		t.Fatalf("missing root result while looking for %q", name)
	}
	root := res.Session.RootResult
	sym := singleSymbolNamed(t, root.Graph, name)
	got := root.NarrowedTypeAt(root.Graph.Exit(), constraint.NewPath(sym, name))
	if got == nil {
		return typ.Unknown
	}
	return got
}

func integerCompatibleType(t typ.Type) bool {
	if typ.TypeEquals(t, typ.Integer) {
		return true
	}
	if lit, ok := t.(*typ.Literal); ok && lit.Base == kind.Integer {
		return true
	}
	return false
}

func presentStringCompatibleType(t typ.Type) bool {
	if t == nil {
		return false
	}
	if _, optional := typ.SplitNilableFieldType(t); optional {
		return false
	}
	return subtype.IsSubtype(t, typ.String)
}

func TestCanonicalGenericInlineCallbackRecordParamIsCallClosed(t *testing.T) {
	res := testutil.Check(`
		local function apply<T, U>(x: T, fn: (T) -> U): U
			return fn(x)
		end
		local n = apply({count = 1}, function(r) return r.count end)
		local out: integer = n
	`, testutil.WithStdlib())

	fn := findFunctionWithParamNames(t, res.Session.Results, "r")
	sym := singleSymbolNamed(t, fn.Graph, "r")
	got := fn.NarrowedTypeAt(fn.Graph.Entry(), constraint.NewPath(sym, "r"))
	count, ok := querycore.Field(got, "count")
	if !ok || !typ.TypeEquals(count, typ.Integer) {
		t.Fatalf("callback param entry type = %v, want record count integer; root expected args=%v diagnostics=%v", got, res.Session.RootResult.CallExpectedArgs, testutil.ErrorMessages(res.Diagnostics))
	}
	if res.HasError() {
		t.Fatalf("expected clean check, got diagnostics: %v", testutil.ErrorMessages(res.Diagnostics))
	}
}
