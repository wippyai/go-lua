package checktest

import "testing"

func TestIntegerScalarOperatorsKeepIntegerPrecision(t *testing.T) {
	result := Check(`
local i: integer = 3
local idiv: integer = i // 2
local imod: integer = i % 2
local inc: integer = i + 1
return idiv + imod + inc
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("integer-preserving scalar operators emitted diagnostics: %#v", result.Diagnostics)
	}
}

func TestIntegerFieldIncrementKeepsIntegerPrecision(t *testing.T) {
	result := Check(`
type Saga = { version: integer }
local saga: Saga = { version = 1 }
saga.version = saga.version + 1
local version: integer = saga.version
return version
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("integer field increment emitted diagnostics: %#v", result.Diagnostics)
	}
}

func TestLoopCounterIncrementKeepsIntegerPrecision(t *testing.T) {
	result := Check(`
local function take(n: integer): ()
end

local function count_fields(values: {[string]: string}): ()
    local count = 0
    for _ in pairs(values) do
        count = count + 1
    end
    take(count)
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("loop counter emitted diagnostics: %#v", result.Diagnostics)
	}
}

func TestNumericFloorDivisionDoesNotLaunderToInteger(t *testing.T) {
	result := Check(`
local n: number = 2.5
local bad: integer = n // 2
return bad
`)
	if len(result.Diagnostics) == 0 {
		t.Fatal("numeric floor division assigned to integer was accepted")
	}
}
