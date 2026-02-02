package types

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/hooks"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/query/core"
)

func TestTypeDef_SimpleRecord(t *testing.T) {
	c := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign())
	sess := c.Check(`
		type Point = {x: number, y: number}
		local p: Point = {x = 10, y = 20}
	`, "test.lua")

	if sess == nil {
		t.Fatal("Check returned nil")
	}
	for _, d := range sess.Diagnostics {
		t.Errorf("unexpected diagnostic: %s", d.Message)
	}
}

func TestTypeDef_UsedBeforeDefinition(t *testing.T) {
	c := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign())
	sess := c.Check(`
		local p: Point = {x = 10, y = 20}
		type Point = {x: number, y: number}
	`, "test.lua")

	if sess == nil {
		t.Fatal("Check returned nil")
	}
	// Should have error - Point used before defined
	if len(sess.Diagnostics) == 0 {
		t.Error("expected diagnostic for undefined type")
	}
}

func TestTypeDef_ReferencesAnother(t *testing.T) {
	c := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign())
	sess := c.Check(`
		type Point = {x: number, y: number}
		type MaybePoint = Point?
		local p: MaybePoint = {x = 1, y = 2}
		local q: MaybePoint = nil
	`, "test.lua")

	if sess == nil {
		t.Fatal("Check returned nil")
	}
	for _, d := range sess.Diagnostics {
		t.Errorf("unexpected diagnostic: %s", d.Message)
	}
}

func TestTypeDef_Union(t *testing.T) {
	c := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign())
	sess := c.Check(`
		type StringOrNumber = string | number
		local a: StringOrNumber = "hello"
		local b: StringOrNumber = 42
	`, "test.lua")

	if sess == nil {
		t.Fatal("Check returned nil")
	}
	for _, d := range sess.Diagnostics {
		t.Errorf("unexpected diagnostic: %s", d.Message)
	}
}

func TestTypeDef_UnionMismatch(t *testing.T) {
	c := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign())
	sess := c.Check(`
		type StringOrNumber = string | number
		local a: StringOrNumber = true
	`, "test.lua")

	if sess == nil {
		t.Fatal("Check returned nil")
	}
	if len(sess.Diagnostics) == 0 {
		t.Error("expected diagnostic for type mismatch")
	}
}

func TestTypeDef_Array(t *testing.T) {
	c := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign())
	sess := c.Check(`
		type Numbers = {number}
		local arr: Numbers = {1, 2, 3}
	`, "test.lua")

	if sess == nil {
		t.Fatal("Check returned nil")
	}
	for _, d := range sess.Diagnostics {
		t.Errorf("unexpected diagnostic: %s", d.Message)
	}
}

func TestTypeDef_NestedRecord(t *testing.T) {
	c := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign())
	sess := c.Check(`
		type Point = {x: number, y: number}
		type Line = {start: Point, finish: Point}
		local line: Line = {
			start = {x = 0, y = 0},
			finish = {x = 10, y = 10}
		}
	`, "test.lua")

	if sess == nil {
		t.Fatal("Check returned nil")
	}
	for _, d := range sess.Diagnostics {
		t.Errorf("unexpected diagnostic: %s", d.Message)
	}
}

func TestTypeDef_InsideIfBlock(t *testing.T) {
	c := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign())
	sess := c.Check(`
		if true then
			type LocalPoint = {x: number, y: number}
			local p: LocalPoint = {x = 1, y = 2}
		end
	`, "test.lua")

	if sess == nil {
		t.Fatal("Check returned nil")
	}
	for _, d := range sess.Diagnostics {
		t.Errorf("unexpected diagnostic: %s", d.Message)
	}
}

func TestTypeDef_NotVisibleOutsideBlock(t *testing.T) {
	c := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign())
	sess := c.Check(`
		if true then
			type LocalPoint = {x: number, y: number}
		end
		local p: LocalPoint = {x = 1, y = 2}
	`, "test.lua")

	if sess == nil {
		t.Fatal("Check returned nil")
	}
	// Should have error - LocalPoint not visible outside if block
	if len(sess.Diagnostics) == 0 {
		t.Error("expected diagnostic for type not in scope")
	}
}

func TestTypeDef_Shadowing(t *testing.T) {
	c := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign())
	sess := c.Check(`
		type Value = number
		local a: Value = 10
		if true then
			type Value = string
			local b: Value = "hello"
		end
		local c: Value = 20
	`, "test.lua")

	if sess == nil {
		t.Fatal("Check returned nil")
	}
	for _, d := range sess.Diagnostics {
		t.Errorf("unexpected diagnostic: %s", d.Message)
	}
}

func TestTypeDef_Multiple(t *testing.T) {
	c := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign())
	sess := c.Check(`
		type Name = string
		type Age = number
		type Person = {name: Name, age: Age}
		local p: Person = {name = "Alice", age = 30}
	`, "test.lua")

	if sess == nil {
		t.Fatal("Check returned nil")
	}
	for _, d := range sess.Diagnostics {
		t.Errorf("unexpected diagnostic: %s", d.Message)
	}
}

func TestTypeDef_Function(t *testing.T) {
	c := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign())
	sess := c.Check(`
		type Callback = (x: number) -> string
		local cb: Callback = function(x: number): string
			return tostring(x)
		end
	`, "test.lua")

	if sess == nil {
		t.Fatal("Check returned nil")
	}
	for _, d := range sess.Diagnostics {
		t.Errorf("unexpected diagnostic: %s", d.Message)
	}
}

func TestTypeDef_Map(t *testing.T) {
	c := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign())
	sess := c.Check(`
		type StringMap = {[string]: number}
		local m: StringMap = {a = 1, b = 2}
	`, "test.lua")

	if sess == nil {
		t.Fatal("Check returned nil")
	}
	for _, d := range sess.Diagnostics {
		t.Errorf("unexpected diagnostic: %s", d.Message)
	}
}

func TestTypeDef_Optional(t *testing.T) {
	c := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign())
	sess := c.Check(`
		type MaybeNumber = number?
		local a: MaybeNumber = 10
		local b: MaybeNumber = nil
	`, "test.lua")

	if sess == nil {
		t.Fatal("Check returned nil")
	}
	for _, d := range sess.Diagnostics {
		t.Errorf("unexpected diagnostic: %s", d.Message)
	}
}

func TestTypeDef_RecordWithOptionalField(t *testing.T) {
	c := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign())
	sess := c.Check(`
		type Config = {name: string, port?: number}
		local c1: Config = {name = "server"}
		local c2: Config = {name = "server", port = 8080}
	`, "test.lua")

	if sess == nil {
		t.Fatal("Check returned nil")
	}
	for _, d := range sess.Diagnostics {
		t.Errorf("unexpected diagnostic: %s", d.Message)
	}
}

func TestTypeDef_WrongFieldType(t *testing.T) {
	c := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign())
	sess := c.Check(`
		type Point = {x: number, y: number}
		local p: Point = {x = "wrong", y = 20}
	`, "test.lua")

	if sess == nil {
		t.Fatal("Check returned nil")
	}
	if len(sess.Diagnostics) == 0 {
		t.Error("expected diagnostic for wrong field type")
	}
}

func TestTypeDef_MissingField(t *testing.T) {
	c := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign())
	sess := c.Check(`
		type Point = {x: number, y: number}
		local p: Point = {x = 10}
	`, "test.lua")

	if sess == nil {
		t.Fatal("Check returned nil")
	}
	if len(sess.Diagnostics) == 0 {
		t.Error("expected diagnostic for missing field")
	}
}

func TestTypeDef_InWhileLoop(t *testing.T) {
	c := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign())
	sess := c.Check(`
		local i = 0
		while i < 1 do
			type Counter = {value: number}
			local c: Counter = {value = i}
			i = i + 1
		end
	`, "test.lua")

	if sess == nil {
		t.Fatal("Check returned nil")
	}
	for _, d := range sess.Diagnostics {
		t.Errorf("unexpected diagnostic: %s", d.Message)
	}
}

func TestTypeDef_InForLoop(t *testing.T) {
	c := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign())
	sess := c.Check(`
		for i = 1, 3 do
			type Index = number
			local idx: Index = i
		end
	`, "test.lua")

	if sess == nil {
		t.Fatal("Check returned nil")
	}
	for _, d := range sess.Diagnostics {
		t.Errorf("unexpected diagnostic: %s", d.Message)
	}
}

func TestTypeDef_InDoBlock(t *testing.T) {
	c := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign())
	sess := c.Check(`
		do
			type Inner = {value: number}
			local x: Inner = {value = 42}
		end
	`, "test.lua")

	if sess == nil {
		t.Fatal("Check returned nil")
	}
	for _, d := range sess.Diagnostics {
		t.Errorf("unexpected diagnostic: %s", d.Message)
	}
}

func TestTypeDef_ChainedReferences(t *testing.T) {
	c := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign())
	sess := c.Check(`
		type A = number
		type B = A
		type C = B
		local x: C = 42
	`, "test.lua")

	if sess == nil {
		t.Fatal("Check returned nil")
	}
	for _, d := range sess.Diagnostics {
		t.Errorf("unexpected diagnostic: %s", d.Message)
	}
}

func TestTypeDef_InNestedFunction(t *testing.T) {
	c := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign())
	sess := c.Check(`
		local function outer()
			type LocalType = {x: number}
			local function inner()
				local v: LocalType = {x = 1}
			end
		end
	`, "test.lua")

	if sess == nil {
		t.Fatal("Check returned nil")
	}
	for _, d := range sess.Diagnostics {
		if !strings.Contains(d.Message, "LocalType") {
			t.Errorf("unexpected diagnostic: %s", d.Message)
		}
	}
}
