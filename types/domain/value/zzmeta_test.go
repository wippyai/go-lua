package value

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// zzClassFull is a class observed with its full method surface.
func zzClassFull() *typ.Record {
	return typ.NewRecord().
		Field("type", typ.Func().Param("self", typ.Unknown).Returns(typ.Unknown).Build()).
		Field("all", typ.Func().Param("self", typ.Unknown).Returns(typ.Number).Build()).
		Build()
}

// zzClassEmpty is the same class observed early, before methods are attached.
func zzClassEmpty() *typ.Record {
	return typ.NewRecord().Build()
}

func TestZZInconsistentClassViewsMerge(t *testing.T) {
	// View A: methods live in the __index prototype (split view).
	proto := typ.NewRecord().
		Field("type", typ.Func().Param("self", typ.Unknown).Returns(typ.Unknown).Build()).
		Field("all", typ.Func().Param("self", typ.Unknown).Returns(typ.Number).Build()).
		Build()
	viewSplit := typ.NewRecord().Field(indexFieldName, proto).Build()
	// View B: methods live at the top (self view), __index points at unknown.
	viewSelf := typ.NewRecord().
		Field("type", typ.Func().Param("self", typ.Unknown).Returns(typ.Unknown).Build()).
		Field("all", typ.Func().Param("self", typ.Unknown).Returns(typ.Number).Build()).
		Field(indexFieldName, typ.Unknown).
		Build()

	for _, tc := range []struct {
		name           string
		existing, cand typ.Type
	}{
		{"split_then_self", viewSplit, viewSelf},
		{"self_then_split", viewSelf, viewSplit},
	} {
		merged := MergeForConvergence(tc.existing, tc.cand)
		t.Logf("[%s] merged=%s", tc.name, typ.FormatShort(merged))
	}
}

const indexFieldName = "__index"

func TestZZConvergenceMakesMethodsOptional(t *testing.T) {
	full := zzClassFull()
	empty := zzClassEmpty()

	for _, tc := range []struct {
		name             string
		existing, cand   typ.Type
	}{
		{"empty_then_full", empty, full},
		{"full_then_empty", full, empty},
	} {
		merged := MergeForConvergence(tc.existing, tc.cand)
		rec, ok := merged.(*typ.Record)
		if !ok {
			t.Errorf("[%s] merged is not a record: %s", tc.name, typ.FormatShort(merged))
			continue
		}
		var typeOpt, allPresent string
		for _, f := range rec.Fields {
			if f.Name == "type" {
				if f.Optional {
					typeOpt = "OPTIONAL"
				} else {
					typeOpt = "required"
				}
			}
		}
		if typeOpt == "" {
			typeOpt = "ABSENT"
		}
		_ = allPresent
		t.Logf("[%s] merged=%s  type field=%s", tc.name, typ.FormatShort(merged), typeOpt)
	}
}
