package engine_test

import (
	"context"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
	"github.com/wippyai/go-lua/analysis/check/lint"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

// diagnosticOnLine reports whether any published diagnostic starts on line.
func diagnosticOnLine(result engine.Result, line int) bool {
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Span.StartLine == line {
			return true
		}
	}
	return false
}

// TestPathEqualIntersectionNarrowsDiscriminatedArm proves that an equality
// between a union member path and a typed peer selects, on its true edge, the
// arms whose member can equal the peer. The then arm reads the arm whose value
// is a number; the else arm is not narrowed and keeps its incompatible member.
func TestPathEqualIntersectionNarrowsDiscriminatedArm(t *testing.T) {
	result, err := engine.Check(`
type ChanInt = {tag: "int"}
type ChanStr = {tag: "str"}
type Sel = {channel: ChanInt, value: number} | {channel: ChanStr, value: string}
local function f(a: ChanInt, sel: Sel)
    if sel.channel == a then
        local good: number = sel.value
    else
        local bad: number = sel.value
    end
end`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if diagnosticOnLine(result, 7) {
		t.Fatalf("path-equal narrowed arm was refuted on its proven type: %#v", result.PublishedDiagnostics)
	}
	if !diagnosticOnLine(result, 9) {
		t.Fatalf("un-narrowed else arm was silently accepted: %#v", result.PublishedDiagnostics)
	}
}

// TestPathEqualIntersectionProvesNarrowedValue proves the true edge yields the
// concrete arm rather than a lenient join: assigning the narrowed member to the
// wrong scalar is a proven refutation, so the narrowing produced number, not a
// still-unknown value.
func TestPathEqualIntersectionProvesNarrowedValue(t *testing.T) {
	result, err := engine.Check(`
type ChanInt = {tag: "int"}
type ChanStr = {tag: "str"}
type Sel = {channel: ChanInt, value: number} | {channel: ChanStr, value: string}
local function f(a: ChanInt, sel: Sel)
    if sel.channel == a then
        local wrong: string = sel.value
    end
end`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !diagnosticOnLine(result, 7) {
		t.Fatalf("path-equal narrowing did not prove the concrete arm value: %#v", result.PublishedDiagnostics)
	}
}

// TestPathEqualIntersectionKeepsBothArmsWhenPeerOverlapsAll proves the
// narrowing is fail-closed: when the peer overlaps every arm, no arm is refuted
// and the member keeps its full union, so the incompatible assignment still
// fails on the true edge.
func TestPathEqualIntersectionKeepsBothArmsWhenPeerOverlapsAll(t *testing.T) {
	result, err := engine.Check(`
type ChanInt = {tag: "int"}
type ChanStr = {tag: "str"}
type Sel = {channel: ChanInt, value: number} | {channel: ChanStr, value: string}
local function f(a: ChanInt | ChanStr, sel: Sel)
    if sel.channel == a then
        local bad: number = sel.value
    end
end`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !diagnosticOnLine(result, 7) {
		t.Fatalf("path-equal over-narrowed when the peer overlaps every arm: %#v", result.PublishedDiagnostics)
	}
}

// typePredicateProject checks a two-module program whose entry validates a value
// with an imported `T:is(value)` witness, returning the entry diagnostics.
func typePredicateProject(t *testing.T, main string) []diagnostic.Diagnostic {
	t.Helper()
	errsrc := `type AppError = {code: string, message: string, retryable: boolean}
local M = {}
M.AppError = AppError
return M`
	result, err := lint.CheckProject(context.Background(), lint.ProjectInput{
		Entries: []lint.Entry{
			{Path: "errors.lua", ModulePath: "errors", Source: errsrc},
			{Path: "main.lua", ModulePath: "main", Imports: []string{"errors"}, Source: main},
		},
		Targets: []string{"main"},
	})
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	return result.Diagnostics
}

// TestNilErrorTypePredicateNarrowsValidatedValue proves the nil polarity of the
// `T:is(value)` witness: on the true edge of an `err == nil` guard the value
// slot holds the checked type, so reading its declared members is proven.
func TestNilErrorTypePredicateNarrowsValidatedValue(t *testing.T) {
	diagnostics := typePredicateProject(t, `local errors = require("errors")
local raw = {code = "A", message = "b", retryable = false}
local validated, type_err = errors.AppError:is(raw)
if type_err == nil then
    local code: string = validated.code
    local message: string = validated.message
end`)
	for _, diagnostic := range diagnostics {
		if diagnostic.Position.Line == 5 || diagnostic.Position.Line == 6 {
			t.Fatalf("nil-error type predicate did not narrow the validated value: %#v", diagnostics)
		}
	}
}

// TestNilErrorTypePredicateProvesConcreteType proves the narrowed value is the
// concrete checked type rather than a still-opaque one: assigning its string
// member to a number target is a proven mismatch, not an unproven claim.
func TestNilErrorTypePredicateProvesConcreteType(t *testing.T) {
	diagnostics := typePredicateProject(t, `local errors = require("errors")
local raw = {code = "A", message = "b", retryable = false}
local validated, type_err = errors.AppError:is(raw)
if type_err == nil then
    local wrong: number = validated.code
end`)
	for _, diagnostic := range diagnostics {
		if diagnostic.Position.Line == 5 && diagnostic.Code == "type.assignment" && strings.Contains(diagnostic.Message, "string, not number") {
			return
		}
	}
	t.Fatalf("nil-error type predicate did not prove the concrete member type: %#v", diagnostics)
}

// TestTypePredicateValueOpaqueWithoutGuard proves the narrowing is gated on the
// error guard: without the `err == nil` edge the value slot stays opaque, so a
// member claim on it cannot be proven.
func TestTypePredicateValueOpaqueWithoutGuard(t *testing.T) {
	diagnostics := typePredicateProject(t, `local errors = require("errors")
local raw = {code = "A", message = "b", retryable = false}
local validated, type_err = errors.AppError:is(raw)
local code: string = validated.code`)
	for _, diagnostic := range diagnostics {
		if diagnostic.Position.Line == 4 && diagnostic.Code == "lint.claim.unproven" {
			return
		}
	}
	t.Fatalf("type predicate value was narrowed without its guard: %#v", diagnostics)
}
