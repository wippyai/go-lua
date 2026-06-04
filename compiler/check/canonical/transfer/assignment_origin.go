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

func (t *Transfer) assignmentProvenanceEffect(
	target cfg.AssignTarget,
	src ast.Expr,
	val product.AbstractValue,
) (AssignmentProvenanceEffect, bool) {
	if target.Kind != cfg.TargetIdent || target.Symbol == 0 || src == nil {
		return AssignmentProvenanceEffect{}, false
	}
	sourcePath, ok := t.staticPathOfExpr(src)
	if !ok || sourcePath.Symbol == 0 || sourcePath.IsEmpty() {
		return AssignmentProvenanceEffect{}, false
	}
	return AssignmentProvenanceEffect{
		TargetPath: constraint.NewPath(target.Symbol, target.Name),
		SourcePath: sourcePath,
		Value:      val,
	}, true
}

func (t *Transfer) applyAssignmentProvenanceEffect(
	out *flow.PointState,
	effect AssignmentProvenanceEffect,
) bool {
	if out == nil || effect.TargetPath.IsEmpty() || effect.SourcePath.IsEmpty() {
		return false
	}
	changed := t.applyAssignmentAliasOrigin(out, effect)
	changed = t.copyAssignmentKeyPresence(out, effect.SourcePath, effect.TargetPath) || changed
	return changed
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
	sourceKey := flow.KeyPresencePathKey(sourcePath)
	targetKey := flow.KeyPresencePathKey(targetPath)
	if sourceKey == "" || targetKey == "" {
		return false
	}
	before := out.KeyPresence
	for _, entry := range out.KeyPresence.Entries() {
		if entry.Key != sourceKey {
			continue
		}
		out.KeyPresence = out.KeyPresence.With(entry.Table, targetKey)
	}
	for _, entry := range out.KeyPresence.ValueEntries() {
		if entry.Key != sourceKey {
			continue
		}
		out.KeyPresence = out.KeyPresence.WithValue(entry.Table, targetKey, entry.Value)
	}
	return !flow.KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
}
