package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// aliasPathType resolves the declared object type at a source path: the symbol's
// declared type for a bare path, or its structural projection for a sub-path.
func (l *lowerer) aliasPathType(p path.Path) (typ.Type, bool) {
	rootType, ok := l.symbolTypes[p.Symbol]
	if !ok {
		return nil, false
	}
	if len(p.Segments) == 0 {
		return rootType, true
	}
	projected, ok := luatypeprojection.ApplySegments(rootType, p.Segments)
	if !ok || projected == nil {
		return nil, false
	}
	return projected, true
}

func (l *lowerer) typeCastPostconditionRefinementFromWIR(point cfg.Point) (factflow.PostconditionRefinement, bool) {
	info, ok := l.directTypeCastCallFromWIR(point)
	if !ok {
		return factflow.PostconditionRefinement{}, false
	}
	return factflow.NewPostconditionRefinement(info.argPath, factflow.NewValueConstraint(l.untrustedTypeWitnessValue(info.target))), true
}

func (l *lowerer) typeCastCallResultValueFromWIR(point cfg.Point) (factflow.CallResultValue, bool) {
	info, ok := l.directTypeCastCallFromWIR(point)
	if !ok {
		return factflow.CallResultValue{}, false
	}
	return factflow.NewCallResultValue(0, l.typeIsProofValue(info.target)), true
}

type directTypeCastInfo struct {
	target  typ.Type
	argPath path.Path
}

func (l *lowerer) directTypeCastCallFromWIR(point cfg.Point) (directTypeCastInfo, bool) {
	t, ok := l.typeCastTargetFromWIR(point)
	if !ok {
		return directTypeCastInfo{}, false
	}
	argPath, ok := l.callArgumentPathFromWIR(point, 0)
	if !ok {
		return directTypeCastInfo{}, false
	}
	return directTypeCastInfo{target: t, argPath: argPath}, true
}

func (l *lowerer) typeCastTargetFromWIR(point cfg.Point) (typ.Type, bool) {
	calleePath, ok := l.callCalleePathFromWIR(point)
	if ok && l.bindings != nil && len(calleePath.Segments) == 0 {
		t, ok := primitiveRuntimeCastType(calleePath.Root)
		if ok && l.bindings.SymbolResolvesToGlobal(calleePath.Symbol, calleePath.Root) {
			return t, true
		}
	}
	inst, hasCall := l.wirCallInstruction(point)
	if !hasCall || inst.Call.Method != 0 || inst.Type == 0 {
		return nil, false
	}
	t := l.wir.Type(inst.Type)
	if t == nil {
		return nil, false
	}
	return t, true
}

func primitiveRuntimeCastType(name string) (typ.Type, bool) {
	switch name {
	case "boolean":
		return typ.Boolean, true
	case "number":
		return typ.Number, true
	case "integer":
		return typ.Integer, true
	case "string":
		return typ.String, true
	default:
		return nil, false
	}
}
