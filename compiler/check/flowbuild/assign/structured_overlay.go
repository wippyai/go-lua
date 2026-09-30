package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	cfganalysis "github.com/wippyai/go-lua/compiler/cfg/analysis"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/overlaymut"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

type structuredWrite struct {
	point     cfg.Point
	versionID int
	segments  []constraint.Segment
	source    ast.Expr
}

// indexStructuredWrites collects static field/index writes keyed by base
// symbol, including field function definitions (function T.f / T:f).
func indexStructuredWrites(graph *cfg.Graph) map[cfg.SymbolID][]structuredWrite {
	result := make(map[cfg.SymbolID][]structuredWrite)
	if graph == nil {
		return result
	}

	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		for i, target := range info.Targets {
			write, sym, ok := structuredWriteForTarget(graph, p, info.SourceAt(i), target)
			if !ok {
				continue
			}
			result[sym] = append(result[sym], write)
		}
	})
	graph.EachFuncDef(func(p cfg.Point, info *cfg.FuncDefInfo) {
		write, sym, ok := structuredWriteForFuncDef(graph, p, info)
		if !ok {
			return
		}
		result[sym] = append(result[sym], write)
	})

	return result
}

func structuredWriteForFuncDef(graph *cfg.Graph, p cfg.Point, info *cfg.FuncDefInfo) (structuredWrite, cfg.SymbolID, bool) {
	if info == nil || info.FuncExpr == nil || (info.TargetKind != cfg.FuncDefField && info.TargetKind != cfg.FuncDefMethod) {
		return structuredWrite{}, 0, false
	}
	sym := info.TargetPath.Symbol
	segments := info.TargetPath.Segments
	if sym == 0 || len(segments) == 0 {
		return structuredWrite{}, 0, false
	}
	for _, seg := range segments {
		if seg.Kind != constraint.SegmentField || seg.Name == "" {
			return structuredWrite{}, 0, false
		}
	}
	version := graph.VisibleVersion(p, sym)
	if version.ID == 0 {
		return structuredWrite{}, 0, false
	}
	return structuredWrite{
		point:     p,
		versionID: version.ID,
		segments:  segments,
		source:    info.FuncExpr,
	}, sym, true
}

// enrichStructuredOverlayAtPoint applies dominating visible field writes for the
// current symbol version into a point-specific identifier overlay.
func enrichStructuredOverlayAtPoint(
	graph *cfg.Graph,
	idom map[cfg.Point]cfg.Point,
	writes map[cfg.SymbolID][]structuredWrite,
	p cfg.Point,
	overlay api.SpecTypes,
	resolveSym func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	synth func(ast.Expr, cfg.Point) typ.Type,
) api.SpecTypes {
	if graph == nil || len(writes) == 0 {
		return overlay
	}

	out := overlay
	copied := false
	for sym, symWrites := range writes {
		if sym == 0 || len(symWrites) == 0 {
			continue
		}

		baseType, ok := out[sym]
		if !ok && resolveSym != nil {
			baseType, ok = resolveSym(p, sym)
		}

		merged := mergeVisibleStructuredWrites(graph, idom, symWrites, sym, p, baseType, synth)
		if merged == nil || (ok && typ.TypeEquals(merged, baseType)) {
			continue
		}

		if !copied {
			if len(overlay) == 0 {
				out = make(api.SpecTypes, 1)
			} else {
				out = make(api.SpecTypes, len(overlay)+1)
				for k, v := range overlay {
					out[k] = v
				}
			}
			copied = true
		}
		out[sym] = merged
	}

	return out
}

func structuredWriteForTarget(graph *cfg.Graph, p cfg.Point, source ast.Expr, target cfg.AssignTarget) (structuredWrite, cfg.SymbolID, bool) {
	if graph == nil || target.BaseSymbol == 0 {
		return structuredWrite{}, 0, false
	}

	var segments []constraint.Segment
	switch target.Kind {
	case cfg.TargetField:
		if len(target.FieldPath) == 0 {
			return structuredWrite{}, 0, false
		}
		segments = make([]constraint.Segment, len(target.FieldPath))
		for i, field := range target.FieldPath {
			if field == "" {
				return structuredWrite{}, 0, false
			}
			segments[i] = constraint.Segment{Kind: constraint.SegmentField, Name: field}
		}
	case cfg.TargetIndex:
		switch key := target.Key.(type) {
		case *ast.StringExpr:
			if key.Value == "" {
				return structuredWrite{}, 0, false
			}
			segments = []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: key.Value}}
		case *ast.NumberExpr:
			segments = []constraint.Segment{{Kind: constraint.SegmentIndexInt}}
		default:
			return structuredWrite{}, 0, false
		}
	default:
		return structuredWrite{}, 0, false
	}

	version := graph.VisibleVersion(p, target.BaseSymbol)
	if version.ID == 0 {
		return structuredWrite{}, 0, false
	}

	return structuredWrite{
		point:     p,
		versionID: version.ID,
		segments:  segments,
		source:    source,
	}, target.BaseSymbol, true
}

func mergeVisibleStructuredWrites(
	graph *cfg.Graph,
	idom map[cfg.Point]cfg.Point,
	writes []structuredWrite,
	sym cfg.SymbolID,
	at cfg.Point,
	baseType typ.Type,
	synth func(ast.Expr, cfg.Point) typ.Type,
) typ.Type {
	if graph == nil || sym == 0 || len(writes) == 0 {
		return baseType
	}

	currentVersion := graph.VisibleVersion(at, sym)
	if currentVersion.ID == 0 {
		return baseType
	}

	current := baseType
	for _, write := range writes {
		if write.versionID != currentVersion.ID {
			continue
		}
		if write.point == at || !cfganalysis.StrictlyDominates(idom, write.point, at) {
			continue
		}

		valueType := typ.Unknown
		if write.source != nil && synth != nil {
			if resolved := synth(write.source, write.point); resolved != nil {
				valueType = resolved
			}
		}
		current = overlaymut.EditAtPath(current, write.segments, func(typ.Type) typ.Type {
			return valueType
		}, overlaymut.OverwritePathEdit)
	}

	return current
}
