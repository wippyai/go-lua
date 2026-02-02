package io

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNewManifest(t *testing.T) {
	m := NewManifest("test/path")
	if m.Path != "test/path" {
		t.Errorf("Path = %q, want %q", m.Path, "test/path")
	}
	if m.Types == nil {
		t.Error("Types should be initialized")
	}
	if m.Summaries == nil {
		t.Error("Summaries should be initialized")
	}
	if m.Globals == nil {
		t.Error("Globals should be initialized")
	}
}

func TestNewSummary(t *testing.T) {
	params := []typ.Type{typ.String, typ.Number}
	returns := []typ.Type{typ.Boolean}
	s := NewSummary(params, returns)

	if len(s.Params) != 2 {
		t.Errorf("Params len = %d, want 2", len(s.Params))
	}
	if len(s.Returns) != 1 {
		t.Errorf("Returns len = %d, want 1", len(s.Returns))
	}
	if s.ReturnsParam != -1 {
		t.Errorf("ReturnsParam = %d, want -1", s.ReturnsParam)
	}
}

func TestManifest_DefineType(t *testing.T) {
	m := NewManifest("test")
	m.DefineType("MyType", typ.String)

	got, ok := m.LookupType("MyType")
	if !ok {
		t.Fatal("LookupType should find defined type")
	}
	if got != typ.String {
		t.Errorf("got %v, want string", got)
	}
}

func TestManifest_DefineSummary(t *testing.T) {
	m := NewManifest("test")
	s := NewSummary(nil, nil)
	m.DefineSummary("myFunc", s)

	got, ok := m.LookupSummary("myFunc")
	if !ok {
		t.Fatal("LookupSummary should find defined summary")
	}
	if got != s {
		t.Error("got different summary")
	}
}

func TestManifest_SetExport_Basic(t *testing.T) {
	m := NewManifest("test")
	m.SetExport(typ.String)
	if m.Export != typ.String {
		t.Error("Export should be set")
	}
}

func TestManifest_AddGlobal(t *testing.T) {
	m := NewManifest("test")
	m.AddGlobal("print", typ.Any)
	if m.Globals["print"] != typ.Any {
		t.Error("Global should be set")
	}
}

func TestManifest_AllTypes(t *testing.T) {
	m := NewManifest("test")
	m.DefineType("A", typ.String)
	m.DefineType("B", typ.Number)

	all := m.AllTypes()
	if len(all) != 2 {
		t.Errorf("AllTypes len = %d, want 2", len(all))
	}
}

func TestManifest_AllTypes_Nil(t *testing.T) {
	var m *Manifest
	all := m.AllTypes()
	if all != nil {
		t.Error("nil manifest AllTypes should return nil")
	}
}

func TestManifest_AllSummaries(t *testing.T) {
	m := NewManifest("test")
	m.DefineSummary("a", NewSummary(nil, nil))
	m.DefineSummary("b", NewSummary(nil, nil))

	all := m.AllSummaries()
	if len(all) != 2 {
		t.Errorf("AllSummaries len = %d, want 2", len(all))
	}
}

func TestManifest_AllGlobals(t *testing.T) {
	m := NewManifest("test")
	m.AddGlobal("x", typ.String)

	all := m.AllGlobals()
	if len(all) != 1 {
		t.Errorf("AllGlobals len = %d, want 1", len(all))
	}
}

func TestFunctionSummary_Clone(t *testing.T) {
	s := &FunctionSummary{
		Params:       []typ.Type{typ.String},
		Returns:      []typ.Type{typ.Boolean},
		ReturnsParam: 0,
	}
	clone := s.Clone()
	if clone == s {
		t.Error("Clone should return different instance")
	}
	if len(clone.Params) != 1 {
		t.Error("Clone should copy params")
	}
	if len(clone.Returns) != 1 {
		t.Error("Clone should copy returns")
	}
}

func TestFunctionSummary_Clone_Full(t *testing.T) {
	s := &FunctionSummary{
		Params:       []typ.Type{typ.String, typ.Number},
		Returns:      []typ.Type{typ.Boolean},
		ReturnsParam: 1,
		ParamEscapes: []bool{true, false},
	}
	clone := s.Clone()
	if len(clone.ParamEscapes) != 2 {
		t.Errorf("ParamEscapes len = %d, want 2", len(clone.ParamEscapes))
	}
	if !clone.ParamEscapes[0] || clone.ParamEscapes[1] {
		t.Error("ParamEscapes values not preserved")
	}
}

func TestFunctionSummary_Clone_Nil(t *testing.T) {
	var s *FunctionSummary
	clone := s.Clone()
	if clone != nil {
		t.Error("nil summary Clone should return nil")
	}
}

func TestFunctionSummary_Clone_WithConditions(t *testing.T) {
	s := &FunctionSummary{
		Params:  []typ.Type{typ.String},
		Returns: []typ.Type{typ.Boolean},
		Requires: constraint.FromConstraints(
			constraint.NotNil{Path: constraint.Path{Root: "$0"}},
		),
		Ensures: constraint.FromConstraints(
			constraint.Truthy{Path: constraint.Path{Root: "result"}},
		),
	}
	clone := s.Clone()
	if !clone.Requires.HasConstraints() {
		t.Error("Requires should be cloned")
	}
	if !clone.Ensures.HasConstraints() {
		t.Error("Ensures should be cloned")
	}
}

func TestManifest_Encode_Decode(t *testing.T) {
	m := NewManifest("test/module")
	m.DefineType("MyType", typ.String)
	m.SetExport(typ.Number)

	data, err := m.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decoded, err := DecodeManifest(data)
	if err != nil {
		t.Fatalf("DecodeManifest failed: %v", err)
	}
	if decoded.Path != "test/module" {
		t.Errorf("Path = %q, want %q", decoded.Path, "test/module")
	}
}

func TestDecodeManifest_InvalidMagic(t *testing.T) {
	_, err := DecodeManifest([]byte{0, 0, 0, 0})
	if !errors.Is(err, ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", err)
	}
}

func TestApplyFunctionSummary_Nil(t *testing.T) {
	fn := typ.Func().Build()
	result := ApplyFunctionSummary(fn, nil)
	if result != fn {
		t.Error("nil summary should return original function")
	}
}

func TestApplyFunctionSummary_EmptySummary(t *testing.T) {
	fn := typ.Func().Param("x", typ.String).Returns(typ.Number).Build()
	summary := NewSummary(nil, nil)
	result := ApplyFunctionSummary(fn, summary)
	if result != fn {
		t.Error("empty summary should return original function")
	}
}

func TestApplyFunctionSummary_WithOptionalAndVariadic(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.String).
		OptParam("y", typ.Number).
		Variadic(typ.Any).
		Returns(typ.Boolean).
		Build()
	summary := NewSummary([]typ.Type{typ.String, typ.Number}, []typ.Type{typ.Boolean})
	summary.Ensures = constraint.FromConstraints(constraint.NotNil{Path: constraint.Path{Root: "result"}})

	result := ApplyFunctionSummary(fn, summary)
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if len(result.Params) != 2 {
		t.Errorf("expected 2 params, got %d", len(result.Params))
	}
	if result.Variadic == nil {
		t.Error("variadic should be preserved")
	}
	if !result.Params[1].Optional {
		t.Error("second param should be optional")
	}

	result = ApplyFunctionSummary(nil, &FunctionSummary{})
	if result != nil {
		t.Error("nil function should return nil")
	}
}

func TestManifest_EnrichedExport_Empty(t *testing.T) {
	m := NewManifest("test")
	if m.EnrichedExport() != nil {
		t.Error("manifest with no export should return nil")
	}
}
