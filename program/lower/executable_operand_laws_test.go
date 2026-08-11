package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

func requireExecutableOperand(t *testing.T, p *program.Program, owner, operand keyspace.Term) {
	t.Helper()
	flow := p.Flow()
	if operand == 0 || flow.Containment().Static(operand) {
		return
	}
	if !flow.Executable().Contains(operand) {
		t.Fatalf("Executable(%v) has non-executable operand %v", owner, operand)
	}
}

// This is the vertical law for the one Program runtime-admission authority.
// It deliberately combines a static typeof(require(...)) tree with live
// aggregates and constructors, so static syntax must stay absent while every
// may-evaluated runtime operand is closed into Executable.
func TestExecutableClosesRuntimeConstructorOperands(t *testing.T) {
	p := parseBindLower(t, `
type Snapshot = typeof(require("dependency"))
local api = require("dependency")
type Subject = api.Schema.User
local key = "item"
local value = -(api[key] + 1)
local result = value and api(key, { [key] = value, value })
return result
`)
	staticImport, staticOK := p.Module().ImportAt(0)
	liveImport, liveOK := p.Module().ImportAt(1)
	staticCall := staticImport.Call
	liveCall := liveImport.Call
	flow := p.Flow()
	if !staticOK || !flow.Containment().Static(staticCall) || flow.Executable().Contains(staticCall) {
		t.Fatalf("static require Call = %v/%v, want static and non-executable", staticCall, staticOK)
	}
	if !liveOK || flow.Containment().Static(liveCall) || !flow.Executable().Contains(liveCall) {
		t.Fatalf("live require Call = %v/%v, want executable runtime occurrence", liveCall, liveOK)
	}
	valuesView := flow.Authored().Values()
	for index := 0; index < valuesView.Count(); index++ {
		values, _ := valuesView.At(index)
		if !flow.Executable().Contains(values) {
			continue
		}
		count, _ := valuesView.Len(values)
		for member := 0; member < count; member++ {
			value, _ := valuesView.Member(values, member)
			requireExecutableOperand(t, p, values, value)
		}
		_, tail, _ := valuesView.Get(values)
		requireExecutableOperand(t, p, values, tail)
	}
	calls := flow.Authored().Calls()
	for index := 0; index < calls.Count(); index++ {
		call, _ := calls.At(index)
		if !flow.Executable().Contains(call) {
			continue
		}
		_, callee, _, actuals, _ := calls.Get(call)
		requireExecutableOperand(t, p, call, callee)
		requireExecutableOperand(t, p, call, actuals)
	}
	unaries := flow.Authored().Operators().Unaries()
	for index := 0; index < unaries.Count(); index++ {
		term, _ := unaries.At(index)
		if flow.Executable().Contains(term) {
			_, _, operand, _ := unaries.Get(term)
			requireExecutableOperand(t, p, term, operand)
		}
	}
	binaries := flow.Authored().Operators().Binaries()
	for index := 0; index < binaries.Count(); index++ {
		term, _ := binaries.At(index)
		if flow.Executable().Contains(term) {
			_, _, left, right, _ := binaries.Get(term)
			requireExecutableOperand(t, p, term, left)
			requireExecutableOperand(t, p, term, right)
		}
	}
	selects := flow.Authored().Operators().Selects()
	for index := 0; index < selects.Count(); index++ {
		term, _ := selects.At(index)
		if flow.Executable().Contains(term) {
			_, _, left, right, _ := selects.Get(term)
			requireExecutableOperand(t, p, term, left)
			requireExecutableOperand(t, p, term, right)
		}
	}
	tables := flow.Authored().Tables()
	for index := 0; index < tables.Count(); index++ {
		table, _ := tables.At(index)
		if !flow.Executable().Contains(table) {
			continue
		}
		count, _ := tables.FieldCount(table)
		for fieldIndex := 0; fieldIndex < count; fieldIndex++ {
			field, _ := tables.FieldAt(table, fieldIndex)
			requireExecutableOperand(t, p, table, field)
		}
	}
	fields := flow.Authored().Fields()
	for index := 0; index < fields.Count(); index++ {
		field, _ := fields.At(index)
		if !flow.Executable().Contains(field) {
			continue
		}
		_, key, values, fieldKind, _ := fields.Get(field)
		if fieldKind == kind.FieldKey || fieldKind == kind.FieldExact {
			requireExecutableOperand(t, p, field, key)
		}
		requireExecutableOperand(t, p, field, values)
	}
	typeOfs := p.Static().Operators().TypeOfs()
	for index := 0; index < typeOfs.Count(); index++ {
		typeOf, _ := typeOfs.At(index)
		if flow.Executable().Contains(typeOf) {
			t.Fatalf("static TypeOf %v became executable", typeOf)
		}
	}
}
