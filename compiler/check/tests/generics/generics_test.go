package generics

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestGenerics_BasicIdentity tests basic generic function identity.
func TestGenerics_BasicIdentity(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "identity function",
			Code: `
				local function identity<T>(x: T): T
					return x
				end
				local n: number = identity(42)
				local s: string = identity("hello")
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "identity wrong type fails",
			Code: `
				local function identity<T>(x: T): T
					return x
				end
				local n: number = identity("hello")
			`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestGenerics_MultipleTypeParams tests functions with multiple type parameters.
func TestGenerics_MultipleTypeParams(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "pair function",
			Code: `
				local function pair<A, B>(a: A, b: B): (A, B)
					return a, b
				end
				local n, s = pair(42, "hello")
				local x: number = n
				local y: string = s
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "swap function",
			Code: `
				local function swap<A, B>(a: A, b: B): (B, A)
					return b, a
				end
				local s, n = swap(42, "hello")
				local x: string = s
				local y: number = n
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestGenerics_GenericTypes tests generic type definitions.
func TestGenerics_GenericTypes(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "generic array type",
			Code: `
				type Array<T> = {[integer]: T}
				local arr: Array<number> = {1, 2, 3}
				local n: number? = arr[1]
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "generic optional type",
			Code: `
				type Maybe<T> = T | nil
				local m: Maybe<number> = 42
				local n: Maybe<number> = nil
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "generic record type",
			Code: `
				type Box<T> = {value: T}
				local b: Box<number> = {value = 42}
				local n: number = b.value
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "generic result type",
			Code: `
				type Result<T, E> = {ok: true, value: T} | {ok: false, error: E}
				local r: Result<number, string> = {ok = true, value = 42}
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestGenerics_Constraints tests generic type constraints.
func TestGenerics_Constraints(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "constraint on type param",
			Code: `
				type Printable = {tostring: (self: Printable) -> string}
				local function print_it<T: Printable>(x: T): string
					return x:tostring()
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "constraint violation at call site",
			Code: `
				type HasName = {name: string}
				local function wrap<T: HasName>(x: T): T
					return x
				end
				local n: number = wrap(42)
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "constraint satisfied at call site",
			Code: `
				type HasName = {name: string}
				local function wrap<T: HasName>(x: T): T
					return x
				end
				local r = wrap({name = "Alice"})
				local s: string = r.name
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestGenerics_Instantiation tests generic type instantiation.
func TestGenerics_Instantiation(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "inferred instantiation",
			Code: `
				local function identity<T>(x: T): T
					return x
				end
				local n: number = identity(42)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "nested generic instantiation",
			Code: `
				type Box<T> = {value: T}
				type DoubleBox<T> = Box<Box<T>>
				local db: DoubleBox<number> = {value = {value = 42}}
				local n: number = db.value.value
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestGenerics_MethodsOnGenericTypes tests methods on generic types.
func TestGenerics_MethodsOnGenericTypes(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "method returns type parameter",
			Code: `
				type Container<T> = {
					_value: T,
					get: (self: Container<T>) -> T
				}
				local c: Container<number> = {
					_value = 42,
					get = function(self: Container<number>): number
						return self._value
					end
				}
				local n: number = c:get()
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestBoundedArrayLiteral tests that array literal indexing is sound.
func TestBoundedArrayLiteral(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "literal index in bounds is non-optional",
			Code: `
				local arr = {1, 2, 3}
				local n: number = arr[1]
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "literal index at last element",
			Code: `
				local arr = {1, 2, 3}
				local n: number = arr[3]
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "literal index out of bounds is nil",
			Code: `
				local arr = {1, 2, 3}
				local n: nil = arr[4]
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "assigning out of bounds to non-optional fails",
			Code: `
				local arr = {1, 2, 3}
				local n: number = arr[4]
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "generic array type with literal index",
			Code: `
				type Array<T> = {[integer]: T}
				local arr: Array<number> = {1, 2, 3}
				local n: number? = arr[1]
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}
