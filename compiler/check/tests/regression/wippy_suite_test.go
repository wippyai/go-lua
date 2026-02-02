package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// TestTypeAliasEquivalence tests that type aliases are equivalent to their underlying types.
// False positive: assigning a value to a type alias of the same underlying type fails.
func TestTypeAliasEquivalence(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "type alias is equivalent to underlying type",
			Code: `
				type UserID = string
				local id: UserID = "user-123"
				local s: string = id
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "function accepting type alias works with underlying type",
			Code: `
				type Amount = number
				local function process(a: Amount): number
					return a * 2
				end
				local result = process(100)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "nested type alias chain resolves correctly",
			Code: `
				type ID = string
				type UserID = ID
				type AdminID = UserID
				local id: AdminID = "admin-123"
				local s: string = id
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "type alias in record field",
			Code: `
				type Name = string
				type Person = {name: Name, age: number}
				local p: Person = {name = "Alice", age = 30}
				local n: string = p.name
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "type alias in function return",
			Code: `
				type Result = {ok: boolean, data: any}
				local function fetch(): Result
					return {ok = true, data = "hello"}
				end
				local r = fetch()
				local ok: boolean = r.ok
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "type alias with optional",
			Code: `
				type MaybeID = string?
				local id: MaybeID = "123"
				local id2: MaybeID = nil
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestNilNarrowing tests that nil checks properly narrow optional types.
// False positive: accessing field after nil check still produces nil-related errors.
func TestNilNarrowing(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "nil check narrows optional to non-nil",
			Code: `
				local function process(x: string?): string
					if x ~= nil then
						return x
					end
					return "default"
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "nil check in condition narrows for then block",
			Code: `
				local function get_length(s: string?): number
					if s ~= nil then
						return #s
					end
					return 0
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "nil check narrows record field",
			Code: `
				type Config = {name: string, port?: number}
				local function get_port(c: Config): number
					if c.port ~= nil then
						return c.port
					end
					return 8080
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "early return on nil narrows rest of function",
			Code: `
				local function require_value(x: string?): string
					if x == nil then
						return "missing"
					end
					return x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "nil check with and operator",
			Code: `
				local function safe_concat(a: string?, b: string): string
					if a ~= nil then
						return a .. b
					end
					return b
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestGenericInstantiation tests that generic types instantiate correctly.
// False positive: using a method on an instantiated generic type fails.
func TestGenericInstantiation(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "generic function instantiates with concrete type",
			Code: `
				local function first<T>(arr: {T}): T?
					return arr[1]
				end
				local n = first({1, 2, 3})
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "generic type alias instantiates correctly",
			Code: `
				type Box<T> = {value: T}
				local b: Box<number> = {value = 42}
				local n: number = b.value
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "nested generic instantiation",
			Code: `
				type Wrapper<T> = {inner: T}
				type DoubleWrap<T> = Wrapper<Wrapper<T>>
				local dw: DoubleWrap<string> = {inner = {inner = "hello"}}
				local s: string = dw.inner.inner
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "generic function with multiple type params",
			Code: `
				local function map<T, U>(arr: {T}, fn: (T) -> U): {U}
					local result: {U} = {}
					for i, v in ipairs(arr) do
						result[i] = fn(v)
					end
					return result
				end
				local nums = map({"a", "bb", "ccc"}, function(s: string): number
					return #s
				end)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "generic record with method",
			Code: `
				type Container<T> = {
					value: T,
					get: (self: Container<T>) -> T
				}
				local c: Container<string> = {
					value = "hello",
					get = function(self: Container<string>): string
						return self.value
					end
				}
				local s: string = c:get()
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestModuleGenericInstantiation tests generics from module manifests.
// False positive: using generic type from module fails to instantiate.
func TestModuleGenericInstantiation(t *testing.T) {
	// Create a module with generic types
	m := io.NewManifest("container")

	// Box<T> = {value: T, unwrap: (self) -> T}
	boxElem := typ.NewTypeParam("T", nil)
	boxType := typ.NewInterface("Box", []typ.Method{
		{
			Name: "unwrap",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(boxElem).
				Build(),
		},
	})
	boxGeneric := typ.NewGeneric("Box", []*typ.TypeParam{boxElem}, boxType)
	m.DefineType("Box", boxGeneric)

	// Result<T, E> = {ok: true, value: T} | {ok: false, error: E}
	resultOK := typ.NewTypeParam("T", nil)
	resultErr := typ.NewTypeParam("E", nil)
	resultType := typ.NewUnion(
		typ.NewRecord().Field("ok", typ.True).Field("value", resultOK).Build(),
		typ.NewRecord().Field("ok", typ.False).Field("error", resultErr).Build(),
	)
	resultGeneric := typ.NewGeneric("Result", []*typ.TypeParam{resultOK, resultErr}, resultType)
	m.DefineType("Result", resultGeneric)

	// Module export with wrap function
	moduleType := typ.NewRecord().
		Field("wrap", typ.Func().
			TypeParam("T", nil).
			Param("value", typ.NewTypeParam("T", nil)).
			Returns(typ.Instantiate(boxGeneric, typ.NewTypeParam("T", nil))).
			Build()).
		Build()
	m.SetExport(moduleType)

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name: "instantiate generic from module",
			code: `
				local b: Box<number> = {unwrap = function(self): number return 42 end}
				local n: number = b:unwrap()
			`,
			wantError: false,
		},
		{
			name: "use module function returning generic",
			code: `
				local b = container.wrap(42)
				local n = b:unwrap()
			`,
			wantError: false,
		},
		{
			name: "result type pattern matching",
			code: `
				local r: Result<number, string> = {ok = true, value = 42}
				if r.ok then
					local n: number = r.value
				end
			`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib(), testutil.WithManifest("container", m))
			if result.HasError() != tt.wantError {
				t.Errorf("wantError=%v, gotError=%v, errors=%v",
					tt.wantError, result.HasError(), testutil.ErrorMessages(result.Diagnostics))
			}
		})
	}
}

// TestRegistryTypeLoss tests that registry.get preserves type information.
// False positive: registry.get returns unknown even when type is registered.
func TestRegistryTypeLoss(t *testing.T) {
	// Create a registry module manifest
	m := io.NewManifest("registry")

	// registry.get<T>(key: string): T?
	getFunc := typ.Func().
		TypeParam("T", nil).
		Param("key", typ.String).
		Returns(typ.NewUnion(typ.NewTypeParam("T", nil), typ.Nil)).
		Build()

	// registry.set<T>(key: string, value: T)
	setFunc := typ.Func().
		TypeParam("T", nil).
		Param("key", typ.String).
		Param("value", typ.NewTypeParam("T", nil)).
		Build()

	moduleType := typ.NewRecord().
		Field("get", getFunc).
		Field("set", setFunc).
		Build()
	m.SetExport(moduleType)

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name: "registry.get with explicit type annotation",
			code: `
				local val: string? = registry.get("key")
				if val ~= nil then
					local s: string = val
				end
			`,
			wantError: false,
		},
		{
			name: "registry round trip",
			code: `
				registry.set("count", 42)
				local n: number? = registry.get("count")
			`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib(), testutil.WithManifest("registry", m))
			if result.HasError() != tt.wantError {
				t.Errorf("wantError=%v, gotError=%v, errors=%v",
					tt.wantError, result.HasError(), testutil.ErrorMessages(result.Diagnostics))
			}
		})
	}
}

// TestCallbackTypePreservation tests that callback types are preserved through function calls.
// False positive: callback parameter type is lost when passed to higher-order function.
func TestCallbackTypePreservation(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "callback receives correct parameter type",
			Code: `
				type User = {name: string, age: number}
				local function process_user(u: User, callback: (User) -> nil)
					callback(u)
				end
				process_user({name = "Alice", age = 30}, function(u: User)
					local n: string = u.name
				end)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "async callback preserves return type",
			Code: `
				local function fetch(url: string, on_done: (string) -> nil)
					on_done("response")
				end
				fetch("http://example.com", function(data: string)
					local s: string = data
				end)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "nested callback preserves types",
			Code: `
				local function outer(f: (number) -> number)
					return f(10)
				end
				local result: number = outer(function(x: number): number
					return x * 2
				end)
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestMethodCallOnUnion tests method calls on union types after narrowing.
// False positive: method call fails even after narrowing union to single variant.
func TestMethodCallOnUnion(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "method call after type narrowing",
			Code: `
				type A = {kind: "a", get_a: (self: A) -> string}
				type B = {kind: "b", get_b: (self: B) -> number}
				type AB = A | B

				local function process(x: AB): string
					if x.kind == "a" then
						return x:get_a()
					end
					return tostring(x:get_b())
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "field access after boolean check",
			Code: `
				type Success = {ok: true, value: string}
				type Failure = {ok: false, error: string}
				type Result = Success | Failure

				local function get_value(r: Result): string
					if r.ok then
						return r.value
					end
					return r.error
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}
