package transformer

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// ObservationKind names the first immutable diagnostic-evidence tranche.
// Kinds are deliberately closed: an unrepresented observation family rejects
// whole-function authority rather than disappearing from diagnostics.
type ObservationKind uint8

const (
	ObservationInvalid ObservationKind = iota
	ObservationAssignment
	ObservationCallArgument
	ObservationCallResult
)

// ObservationTerm is guarded evidence captured at the exact CFG point where a
// lexical binding becomes visible. Guard is local to the observation; using an
// exit-row guard would incorrectly condition earlier evidence on later paths.
type ObservationTerm struct {
	Owner    CellRef
	Route    uint64
	Kind     ObservationKind
	Point    cfg.Point
	Anchor   factflow.ExprRef
	Guard    Guard
	Symbol   symbol.ID
	Slot     uint32
	Actual   ValueTerm
	Expected ValueTerm
}

func (o ObservationTerm) valid(arena *Arena, shape Shape) bool {
	return o.Kind > ObservationInvalid && o.Kind <= ObservationCallResult && o.Point >= 0 &&
		(o.Symbol != 0 || o.Kind == ObservationCallArgument) && arena.validGuard(o.Guard, shape) && arena.validValue(o.Actual, shape, make(map[ValueTerm]bool)) &&
		(o.Expected == 0 || arena.validValue(o.Expected, shape, make(map[ValueTerm]bool)))
}

func (o ObservationTerm) canonical(arena *Arena) string {
	expected := "-"
	if o.Expected != 0 {
		expected = arena.canonicalValue(o.Expected)
	}
	return fmt.Sprintf("%d.%d.%x:%d:%d:%d:%s:%d:%d:%s:%s", o.Owner.Function, o.Owner.Slot, o.Route, o.Kind, o.Point, o.Anchor, arena.canonicalGuard(o.Guard), o.Symbol, o.Slot, arena.canonicalValue(o.Actual), expected)
}

// Observation is one specialized immutable evidence cell.
type Observation struct {
	Owner       CellRef
	Route       uint64
	Kind        ObservationKind
	Point       cfg.Point
	Anchor      factflow.ExprRef
	Symbol      symbol.ID
	Slot        uint32
	Actual      product.Value
	Expected    product.Value
	HasExpected bool
}

// ObservationProjection is transient specialization output. It is not a
// cache/publication artifact: schema, codec, universe, and keyspace contracts
// belong to the later atomic diagnostics artifact. Before publication, Owner
// must become lexicalidentity.StableLexicalBodyID (CellRef is run-local), the
// ExprRef-or-point anchor must become a tagged durable occurrence, and abnormal
// terminal/cyclic observation rows must be projected independently of returns.
type ObservationProjection struct{ items []Observation }

func (a ObservationProjection) Items() []Observation { return append([]Observation(nil), a.items...) }

func canonicalizeObservations(reg *axis.Registry, in []Observation) ObservationProjection {
	sort.Slice(in, func(i, j int) bool {
		if in[i].Owner != in[j].Owner {
			if in[i].Owner.Function != in[j].Owner.Function {
				return in[i].Owner.Function < in[j].Owner.Function
			}
			return in[i].Owner.Slot < in[j].Owner.Slot
		}
		if in[i].Route != in[j].Route {
			return in[i].Route < in[j].Route
		}
		if in[i].Point != in[j].Point {
			return in[i].Point < in[j].Point
		}
		if in[i].Kind != in[j].Kind {
			return in[i].Kind < in[j].Kind
		}
		if in[i].Anchor != in[j].Anchor {
			return in[i].Anchor < in[j].Anchor
		}
		if in[i].Slot != in[j].Slot {
			return in[i].Slot < in[j].Slot
		}
		if in[i].Symbol != in[j].Symbol {
			return in[i].Symbol < in[j].Symbol
		}
		if left, right := product.Hash(reg, in[i].Actual), product.Hash(reg, in[j].Actual); left != right {
			return left < right
		}
		if in[i].HasExpected != in[j].HasExpected {
			return !in[i].HasExpected
		}
		return product.Hash(reg, in[i].Expected) < product.Hash(reg, in[j].Expected)
	})
	out := in[:0]
	for _, item := range in {
		last := len(out) - 1
		if last >= 0 && sameObservationOccurrence(out[last], item) && out[last].HasExpected == item.HasExpected &&
			product.Equal(reg, out[last].Actual, item.Actual) && (!item.HasExpected || product.Equal(reg, out[last].Expected, item.Expected)) {
			continue
		}
		out = append(out, item)
	}
	return ObservationProjection{items: out}
}

func sameObservationOccurrence(left, right Observation) bool {
	return left.Owner == right.Owner && left.Route == right.Route && left.Point == right.Point && left.Kind == right.Kind && left.Anchor == right.Anchor && left.Symbol == right.Symbol && left.Slot == right.Slot
}

func recordObservationTerm(in []ObservationTerm, next ObservationTerm) []ObservationTerm {
	for _, prior := range in {
		if prior == next {
			return in
		}
	}
	return append(in, next)
}

func routeObservationTerm(term ObservationTerm, point cfg.Point) ObservationTerm {
	term.Route = (term.Route ^ uint64(point+1)) * 1099511628211
	return term
}
