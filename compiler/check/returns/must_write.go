package returns

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
)

type fieldWriteSite struct {
	Target cfg.SymbolID
	Key    api.FieldWriteKey
}

// MustFieldWrites finds fields left present by direct assignments on every
// path from the function entry to its exit. Several write sites can jointly
// make a field definite; a later nil assignment removes it again.
func MustFieldWrites(graph *cfg.Graph) map[cfg.SymbolID]map[api.FieldWriteKey]bool {
	if graph == nil {
		return nil
	}
	// Each event records whether the field is present after the assignment.
	sites := make(map[fieldWriteSite]map[cfg.Point]bool)
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		for i, target := range info.Targets {
			key, ok := fieldWriteKey(target)
			if !ok {
				continue
			}
			site := fieldWriteSite{Target: target.BaseSymbol, Key: key}
			if sites[site] == nil {
				sites[site] = make(map[cfg.Point]bool)
			}
			present := true
			if i < len(info.Sources) {
				_, removesField := info.Sources[i].(*ast.NilExpr)
				present = !removesField
			}
			sites[site][p] = present
		}
	})
	var must map[cfg.SymbolID]map[api.FieldWriteKey]bool
	for site, events := range sites {
		if mayExitWithoutField(graph, events) {
			continue
		}
		if must == nil {
			must = make(map[cfg.SymbolID]map[api.FieldWriteKey]bool)
		}
		if must[site.Target] == nil {
			must[site.Target] = make(map[api.FieldWriteKey]bool)
		}
		must[site.Target][site.Key] = true
	}
	return must
}

func fieldWriteKey(target cfg.AssignTarget) (api.FieldWriteKey, bool) {
	if target.BaseSymbol == 0 {
		return api.FieldWriteKey{}, false
	}
	switch target.Kind {
	case cfg.TargetField:
		if len(target.FieldPath) == 0 {
			return api.FieldWriteKey{}, false
		}
		segments := make([]constraint.Segment, 0, len(target.FieldPath)-1)
		for _, field := range target.FieldPath[:len(target.FieldPath)-1] {
			segments = append(segments, constraint.Segment{Kind: constraint.SegmentField, Name: field})
		}
		return api.NewFieldWriteKey(segments, target.FieldPath[len(target.FieldPath)-1]), true
	case cfg.TargetIndex:
		if _, direct := target.Base.(*ast.IdentExpr); !direct {
			return api.FieldWriteKey{}, false
		}
		if key, ok := target.Key.(*ast.StringExpr); ok && key.Value != "" {
			return api.FieldWriteKey{Field: key.Value}, true
		}
	}
	return api.FieldWriteKey{}, false
}

func mayExitWithoutField(graph *cfg.Graph, events map[cfg.Point]bool) bool {
	type state struct {
		point   cfg.Point
		present bool
	}
	seen := make(map[state]bool)
	queue := []state{{point: graph.Entry()}}
	for head := 0; head < len(queue); head++ {
		current := queue[head]
		if present, writes := events[current.point]; writes {
			current.present = present
		}
		if seen[current] {
			continue
		}
		if current.point == graph.Exit() && !current.present {
			return true
		}
		seen[current] = true
		for _, next := range graph.SuccessorsReadOnly(current.point) {
			queue = append(queue, state{point: next, present: current.present})
		}
	}
	return false
}
