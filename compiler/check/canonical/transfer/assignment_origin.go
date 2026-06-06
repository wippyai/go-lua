package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/typ"
)

// AssignmentProvenanceEffect is the transfer reducer payload for path facts
// introduced by a local assignment. A plain alias assignment (`b = a`) can carry
// both value-origin identity for later index-write readback and key-presence
// proofs where the assigned value is later used as the dynamic key.
type AssignmentProvenanceEffect struct {
	TargetPath constraint.Path
	SourcePath constraint.Path
	Value      product.AbstractValue
}

// ArrayElementKeyProvenanceEffect is the transfer reducer payload for a local
// assignment from an indexed array element. If the source array is proven to hold
// keys for one or more tables, the assigned target is a key for those tables.
type ArrayElementKeyProvenanceEffect struct {
	TargetPath constraint.Path
	ArrayPath  constraint.Path
	Value      product.AbstractValue
}

func (t *Transfer) assignmentProvenanceEffect(
	target cfg.AssignTarget,
	src ast.Expr,
	val product.AbstractValue,
) (AssignmentProvenanceEffect, bool) {
	return t.assignmentProvenanceEffectWithSourceSymbol(target, src, 0, val)
}

func (t *Transfer) assignmentProvenanceEffectWithSourceSymbol(
	target cfg.AssignTarget,
	src ast.Expr,
	sourceSym cfg.SymbolID,
	val product.AbstractValue,
) (AssignmentProvenanceEffect, bool) {
	if src == nil {
		return AssignmentProvenanceEffect{}, false
	}
	targetPath, ok := t.exactStaticAliasTargetPath(target)
	if !ok || targetPath.Symbol == 0 || targetPath.IsEmpty() {
		return AssignmentProvenanceEffect{}, false
	}
	sourcePath, ok := t.staticPathOfExpr(src)
	if !ok || sourcePath.Symbol == 0 || sourcePath.IsEmpty() {
		sourcePath, ok = staticPathOfExprWithRootSymbol(src, sourceSym)
		if !ok || sourcePath.Symbol == 0 || sourcePath.IsEmpty() {
			return AssignmentProvenanceEffect{}, false
		}
	}
	return AssignmentProvenanceEffect{
		TargetPath: targetPath,
		SourcePath: sourcePath,
		Value:      val,
	}, true
}

func (t *Transfer) arrayElementKeyProvenanceEffect(
	target cfg.AssignTarget,
	src ast.Expr,
	val product.AbstractValue,
) (ArrayElementKeyProvenanceEffect, bool) {
	targetPath, ok := t.exactStaticAliasTargetPath(target)
	if !ok || targetPath.Symbol == 0 || targetPath.IsEmpty() {
		return ArrayElementKeyProvenanceEffect{}, false
	}
	attr, ok := src.(*ast.AttrGetExpr)
	if !ok || attr == nil || attr.Key == nil {
		return ArrayElementKeyProvenanceEffect{}, false
	}
	arrayPath, ok := t.containerExprPath(attr.Object)
	if !ok || arrayPath.IsEmpty() {
		return ArrayElementKeyProvenanceEffect{}, false
	}
	return ArrayElementKeyProvenanceEffect{
		TargetPath: targetPath,
		ArrayPath:  arrayPath,
		Value:      val,
	}, true
}

func (t *Transfer) exactStaticAliasTargetPath(target cfg.AssignTarget) (constraint.Path, bool) {
	if target.Expr != nil {
		return t.staticPathOfExpr(target.Expr)
	}
	switch target.Kind {
	case cfg.TargetIdent:
		if target.Symbol == 0 {
			return constraint.Path{}, false
		}
		return constraint.NewPath(target.Symbol, target.Name), true
	case cfg.TargetField:
		if target.BaseSymbol == 0 || len(target.FieldPath) == 0 {
			return constraint.Path{}, false
		}
		path := constraint.NewPath(target.BaseSymbol, target.BaseName)
		path.Segments = append(path.Segments, fieldSegments(target.FieldPath)...)
		return path, true
	case cfg.TargetIndex:
		if target.Base == nil {
			if target.BaseSymbol == 0 {
				return constraint.Path{}, false
			}
			path := constraint.NewPath(target.BaseSymbol, target.BaseName)
			path.Segments = append(path.Segments, fieldSegments(target.FieldPath)...)
			seg, ok := staticIndexSegment(target.Key)
			if !ok {
				return constraint.Path{}, false
			}
			path.Segments = append(path.Segments, seg)
			return path, true
		}
		path, ok := t.staticPathOfExpr(target.Base)
		if !ok || path.Symbol == 0 {
			return constraint.Path{}, false
		}
		seg, ok := staticIndexSegment(target.Key)
		if !ok {
			return constraint.Path{}, false
		}
		path.Segments = append(path.Segments, seg)
		return path, true
	default:
		return constraint.Path{}, false
	}
}

func (t *Transfer) applyAssignmentProvenanceEffect(
	out *flow.PointState,
	effect AssignmentProvenanceEffect,
) bool {
	if out == nil || effect.TargetPath.IsEmpty() || effect.SourcePath.IsEmpty() {
		return false
	}
	changed := t.applyPathAliasEffect(out, effect)
	changed = t.applyAssignmentAliasOrigin(out, effect) || changed
	changed = t.copyAssignmentKeyPresence(out, effect.SourcePath, effect.TargetPath) || changed
	changed = t.copyAssignmentIndexWriteAdmissions(out, effect.SourcePath, effect.TargetPath) || changed
	return changed
}

func (t *Transfer) applyArrayElementKeyProvenanceEffect(
	out *flow.PointState,
	effect ArrayElementKeyProvenanceEffect,
) bool {
	if out == nil || effect.TargetPath.IsEmpty() || effect.ArrayPath.IsEmpty() {
		return false
	}
	arrayAddr, arrayOK := flow.StableAddressOfPath(effect.ArrayPath)
	targetAddr, targetOK := flow.StableAddressOfPath(effect.TargetPath)
	if !arrayOK || !targetOK {
		return false
	}
	tables, presenceChanged := flow.ApplyKeyArrayElementKeyProof(out, flow.KeyArrayElementKeyProof{
		Array:     arrayAddr,
		TargetKey: targetAddr,
	})
	changed := false
	arrayKey := arrayAddr.Key()
	for _, table := range tables {
		for _, value := range out.KeyPresence.KeyArrayValues(arrayKey, table) {
			tablePath, ok := indexWritePathFromKey(table)
			if !ok || tablePath.IsEmpty() || value.IsZero() {
				continue
			}
			keyValue := effect.Value
			if keyValue.IsZero() {
				keyValue = product.FromType(typ.Unknown)
			}
			tableAddr, tableOK := flow.StableAddressOfPath(tablePath)
			keyAddr, keyOK := flow.StableAddressOfPath(effect.TargetPath)
			if !tableOK || !keyOK {
				continue
			}
			changed = flow.ApplyIndexWriteAdmissionProof(out, flow.IndexWriteAdmissionProof{
				Fact: flow.IndexWriteAdmissionAddressFact{
					Target:     tableAddr,
					KeyPath:    keyAddr,
					HasKeyPath: true,
					Key:        keyValue,
					Value:      value,
				},
			}) || changed
		}
	}
	changed = presenceChanged || changed
	changed = t.applyValueOriginEffect(out, ValueOriginEffect{
		ValuePath:  effect.TargetPath,
		SourcePath: effect.ArrayPath,
		Kind:       flow.ValueOriginIndexedIterator,
		VarIndex:   1,
	}) || changed
	return changed
}

func (t *Transfer) applyPathAliasEffect(out *flow.PointState, effect AssignmentProvenanceEffect) bool {
	if pathkey.PathRelated(effect.TargetPath, effect.SourcePath) {
		return false
	}
	targetAddr, ok := flow.StableAddressOfPath(effect.TargetPath)
	if !ok {
		return false
	}
	sourceAddr, ok := flow.StableAddressOfPath(effect.SourcePath)
	if !ok {
		return false
	}
	return flow.ApplyPathAliasProof(out, flow.PathAliasProof{
		Value:  targetAddr,
		Source: sourceAddr,
	})
}

func (t *Transfer) applyAssignmentAliasOrigin(out *flow.PointState, effect AssignmentProvenanceEffect) bool {
	if effect.Value.IsZero() || pathkey.PathRelated(effect.TargetPath, effect.SourcePath) {
		return false
	}
	valType := product.ProjectValueOrUnknown(effect.Value)
	if typ.IsAbsentOrUnknown(valType) || typ.IsAny(valType) {
		return false
	}
	return t.applyValueOriginEffect(out, ValueOriginEffect{
		ValuePath:  effect.TargetPath,
		SourcePath: effect.SourcePath,
		Kind:       flow.ValueOriginAssignmentAlias,
	})
}

func (t *Transfer) copyAssignmentKeyPresence(out *flow.PointState, sourcePath, targetPath constraint.Path) bool {
	sourceAddr, sourceOK := flow.StableAddressOfPath(sourcePath)
	targetAddr, targetOK := flow.StableAddressOfPath(targetPath)
	if !sourceOK || !targetOK {
		return false
	}
	return flow.ApplyKeyPresenceAliasProof(out, flow.KeyPresenceAliasProof{
		SourceKey: sourceAddr,
		TargetKey: targetAddr,
	})
}

func (t *Transfer) copyAssignmentIndexWriteAdmissions(out *flow.PointState, sourcePath, targetPath constraint.Path) bool {
	sourceAddr, sourceOK := flow.StableAddressOfPath(sourcePath)
	targetAddr, targetOK := flow.StableAddressOfPath(targetPath)
	if !sourceOK || !targetOK {
		return false
	}
	sourceKey := sourceAddr.Key()
	targetKey := targetAddr.Key()
	before := out.IndexWrites
	flow.ApplyIndexWriteKeyAliasProof(out, flow.IndexWriteKeyAliasProof{
		SourceKey: sourceAddr,
		TargetKey: targetAddr,
	})
	sourceValue, hasSourceValue := flow.PointFactsOf(*out).PathValue(sourcePath)
	if hasSourceValue && sourceValue.DefinitelyAbsent() {
		return !flow.IndexWriteAdmissionFactsDomain.Equal(before, out.IndexWrites)
	}
	if !hasSourceValue || sourceValue.IsZero() {
		sourceValue = product.FromType(typ.Unknown)
	}
	for _, entry := range out.KeyPresence.Entries() {
		if entry.Key != sourceKey {
			continue
		}
		tablePath, ok := indexWritePathFromKey(entry.Table)
		if !ok || tablePath.IsEmpty() {
			continue
		}
		tableValue, ok := flow.PointFactsOf(*out).PathValue(tablePath)
		if !ok || tableValue.IsZero() {
			continue
		}
		read, ok := product.RuntimeIndexOf(tableValue, sourceValue)
		if !ok || read.IsZero() {
			continue
		}
		present := product.NarrowPresent(read)
		if present.IsZero() || !flow.AdmissibleMapWriteProofValue(present) {
			continue
		}
		tableAddr, tableOK := flow.StableAddressFromKey(entry.Table)
		keyAddr, keyOK := flow.StableAddressFromKey(targetKey)
		if !tableOK || !keyOK {
			continue
		}
		flow.ApplyIndexWriteAdmissionProof(out, flow.IndexWriteAdmissionProof{
			Fact: flow.IndexWriteAdmissionAddressFact{
				Target:     tableAddr,
				KeyPath:    keyAddr,
				HasKeyPath: true,
				Key:        sourceValue,
				Value:      present,
			},
		})
	}
	return !flow.IndexWriteAdmissionFactsDomain.Equal(before, out.IndexWrites)
}
