package body

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// ObjectLiteralEntryProof is the body-owned proof projection for one lowered
// object-literal entry checked against a contextual field contract.
type ObjectLiteralEntryProof struct {
	Value            product.Value
	HasValue         bool
	Type             typ.Type
	HasType          bool
	UntrustedTop     bool
	ExplicitTop      bool
	Admissible       bool
	Assignable       bool
	ProvenMismatch   bool
	RuntimeValidated bool
}

// ObjectLiteralEntryProofAt resolves the value/type/evidence bundle for one
// lowered object-literal entry. Readmodels use the result for presentation only;
// body owns the value projection and subtype proof decisions.
func (r *Result) ObjectLiteralEntryProofAt(point cfg.Point, entry factflow.ObjectEntryView, expected typ.Type) ObjectLiteralEntryProof {
	value, valueOK := r.objectLiteralEntryValue(point, entry)
	t, typeOK := r.objectLiteralEntryType(value, valueOK)
	proof := ObjectLiteralEntryProof{
		Value:            value,
		HasValue:         valueOK,
		Type:             t,
		HasType:          typeOK,
		UntrustedTop:     valueOK && r.ValueHasUntrustedTopOrigin(value),
		ExplicitTop:      valueOK && r.ValueHasExplicitTopOrigin(value),
		RuntimeValidated: valueOK && r.ValueHasRuntimeValidationProof(value),
	}
	if valueOK {
		proof.Admissible = r.ValueProofAdmissible(value, expected)
		proof.ProvenMismatch = r.ValueWitnessProvenMismatch(value, expected)
	}
	if typeOK && t != nil {
		proof.Assignable = r.IsSubtype(t, expected)
		if !proof.ProvenMismatch && !proof.UntrustedTop {
			proof.ProvenMismatch = r.objectLiteralTypeProvenMismatch(t, expected)
		}
	}
	return proof
}

// ObjectLiteralShapeTypeAt builds the presentation type for a lowered object
// literal from solved entry values. It is a body projection because the entry
// values and their proof-adjusted types belong to solved state.
func (r *Result) ObjectLiteralShapeTypeAt(point cfg.Point, literal factflow.ObjectLiteralView) (typ.Type, bool) {
	builder := typetable.NewConstructorBuilder()
	seen := false
	literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		constructorPath, ok := luatypeprojection.ConstructorPathFromSegments(entry.SuffixSegmentsView())
		if !ok {
			return true
		}
		value, valueOK := r.objectLiteralEntryValue(point, entry)
		t, ok := r.objectLiteralEntryType(value, valueOK)
		if !ok || t == nil {
			return true
		}
		if !builder.Add(constructorPath, t) {
			seen = false
			return false
		}
		seen = true
		return true
	})
	if !seen {
		return nil, false
	}
	return builder.Build()
}

// ObjectLiteralMissingRequired reports a required field absent from a lowered
// object literal under expected.
func ObjectLiteralMissingRequired(literal factflow.ObjectLiteralView, expected typ.Type) (string, bool) {
	return luatypeprojection.MissingRequiredRecordField(expected, func(name string) bool {
		has := false
		literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
			if field, ok := segment.DirectFieldName(entry.SuffixSegmentsView()); ok && field == name {
				has = true
				return false
			}
			return true
		})
		return has
	})
}

func (r *Result) objectLiteralEntryValue(point cfg.Point, entry factflow.ObjectEntryView) (product.Value, bool) {
	if r == nil {
		return product.Value{}, false
	}
	source := entry.Source()
	if value, ok := r.SourceValueBeforeBoundary(point, source); ok {
		return value, true
	}
	return r.SourceValueForExplanationAtBoundary(point, source)
}

func (r *Result) objectLiteralEntryType(value product.Value, valueOK bool) (typ.Type, bool) {
	if !valueOK {
		return nil, false
	}
	if r != nil {
		if t, ok := luasourcevalue.ObjectLiteralEntryType(r.Registry(), r.typeValues, value); ok {
			return t, true
		}
		if !r.ValueHasUntrustedTopOrigin(value) {
			return r.ValueTypeWithPresence(value)
		}
		if projected, ok := r.ValueTypeWithPresence(value); ok && projected != nil {
			return projected, true
		}
	}
	return typ.Unknown, true
}

func (r *Result) objectLiteralTypeProvenMismatch(actual, expected typ.Type) bool {
	if actual == nil || expected == nil || typ.IsAny(actual) || typ.IsUnknown(actual) || typ.IsNever(actual) {
		return false
	}
	return !r.IsSubtype(actual, expected)
}
