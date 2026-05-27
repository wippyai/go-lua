package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// A function returning a table.insert-built array whose nested extractor result
// carries an any-typed field must preserve that any field across the return
// boundary. A within-compilation caller reading the element field is exercising
// gradual any, not a degraded unknown, so the comparison type-checks.
//
// Non-loop variant: the array is built by a single table.insert.
func TestInterprocReturn_AnyFieldArrayElement_NoLoop_StaysPrecise(t *testing.T) {
	source := `
		local function extract(entry)
			local meta = entry.meta
			return {
				id = entry.id,
				order = meta.order or 0,
			}
		end

		local function find_all()
			local entries = {
				{ id = "a", meta = ({ order = 1 } :: any) },
			}
			local out = {}
			table.insert(out, extract(entries[1]))
			return out
		end

		local pages = find_all()
		local a = pages[1]
		local b = pages[2]
		local _ok: boolean = a.order < b.order
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected any-field array element to stay precise, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Loop variant: the array is built by table.insert inside a for loop. The
// loop-built element must preserve the same element field precision as the
// single-insert variant across the return boundary.
func TestInterprocReturn_AnyFieldArrayElement_Loop_StaysPrecise(t *testing.T) {
	source := `
		local function extract(entry)
			local meta = entry.meta
			return {
				id = entry.id,
				order = meta.order or 0,
			}
		end

		local function find_all()
			local entries = {
				{ id = "a", meta = ({ order = 1 } :: any) },
				{ id = "b", meta = ({ order = 2 } :: any) },
			}
			local out = {}
			for _, entry in ipairs(entries) do
				table.insert(out, extract(entry))
			end
			return out
		end

		local pages = find_all()
		local a = pages[1]
		local b = pages[2]
		local _ok: boolean = a.order < b.order
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected loop-built any-field array element to stay precise, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// A concrete (non-any) element field of a returned table.insert-built array must
// reach a within-compilation caller as that concrete type.
func TestInterprocReturn_ConcreteFieldArrayElement_StaysConcrete(t *testing.T) {
	source := `
		local function extract(index)
			return {
				id = "row",
				order = index,
			}
		end

		local function find_all()
			local out = {}
			for i = 1, 3 do
				table.insert(out, extract(i))
			end
			return out
		end

		local pages = find_all()
		local a = pages[1]
		local b = pages[2]
		local _ok: boolean = a.order < b.order
		local _id: string = a.id
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected concrete array element field to stay concrete, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// A union element type (record fields that differ across branches) of a returned
// table.insert-built array reaches the caller as a usable union, not unknown.
func TestInterprocReturn_UnionFieldArrayElement_StaysUsable(t *testing.T) {
	source := `
		local function extract(entry)
			local raw = entry.value
			return {
				value = raw or "default",
			}
		end

		local function collect()
			local items = {
				{ value = ("x" :: any) },
			}
			local out = {}
			for _, item in ipairs(items) do
				table.insert(out, extract(item))
			end
			return out
		end

		local rows = collect()
		local first = rows[1]
		local _len: integer = #tostring(first.value)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected union array element field to stay usable, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
