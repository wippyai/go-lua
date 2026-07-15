package transformer

import (
	"fmt"
	"sort"

	engineobservation "github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// ObserverCallTemplate is one owner-local guarded lexical invocation. The
// packed term tuple is correlated: values and paths use Shape order and must
// never be joined independently. Callee annotations are deliberately absent.
type ObserverCallTemplate struct {
	owner      lexicalidentity.StableLexicalBodyID
	occurrence engineobservation.Occurrence
	point      cfg.Point
	target     DirectCallTarget
	guard      Guard
	values     []ValueTerm
	paths      []PathTerm
}

func (t ObserverCallTemplate) Owner() lexicalidentity.StableLexicalBodyID { return t.owner }
func (t ObserverCallTemplate) Occurrence() engineobservation.Occurrence   { return t.occurrence }
func (t ObserverCallTemplate) Point() (cfg.Point, bool) {
	return t.point, t.occurrence.Valid() && t.occurrence.Kind == engineobservation.CallInvocation
}
func (t ObserverCallTemplate) Target() DirectCallTarget { return t.target }
func (t ObserverCallTemplate) Guard() Guard             { return t.guard }
func (t ObserverCallTemplate) Shape() Shape             { return t.target.Shape }
func (t ObserverCallTemplate) ValueTerms() []ValueTerm  { return append([]ValueTerm(nil), t.values...) }
func (t ObserverCallTemplate) PathTerms() []PathTerm    { return append([]PathTerm(nil), t.paths...) }

// ObserverCallTemplates returns an immutable snapshot of owner-local call
// templates. Nested callee templates are never unfolded into this relation.
func (r Relation) ObserverCallTemplates() []ObserverCallTemplate {
	out := make([]ObserverCallTemplate, len(r.annotations.calls))
	for i, template := range r.annotations.calls {
		out[i] = cloneObserverCallTemplate(template)
	}
	return out
}

func cloneObserverCallTemplate(in ObserverCallTemplate) ObserverCallTemplate {
	in.values = append([]ValueTerm(nil), in.values...)
	in.paths = append([]PathTerm(nil), in.paths...)
	return in
}

func (t ObserverCallTemplate) valid(arena *Arena, caller Shape) bool {
	if arena == nil || t.owner == (lexicalidentity.StableLexicalBodyID{}) ||
		!t.occurrence.Valid() || t.occurrence.Kind != engineobservation.CallInvocation ||
		t.target.Cell == (CellRef{}) || len(t.values) != t.target.Shape.ValueCount() || len(t.paths) != len(t.values) ||
		!arena.validGuard(t.guard, caller) {
		return false
	}
	for i, value := range t.values {
		if !arena.validValue(value, caller, make(map[ValueTerm]bool)) {
			return false
		}
		if t.paths[i] != 0 && !arena.validPath(t.paths[i], caller) {
			return false
		}
	}
	return true
}

func equalObserverCallTemplate(left, right ObserverCallTemplate) bool {
	if left.owner != right.owner || left.occurrence != right.occurrence || left.point != right.point ||
		left.target != right.target || left.guard != right.guard || len(left.values) != len(right.values) || len(left.paths) != len(right.paths) {
		return false
	}
	for i := range left.values {
		if left.values[i] != right.values[i] || left.paths[i] != right.paths[i] {
			return false
		}
	}
	return true
}

func (t ObserverCallTemplate) canonical(arena *Arena) string {
	out := fmt.Sprintf("%x:%v:%d:%d:%d:%v:%s", t.owner, t.occurrence, t.point, t.target.Cell.Function, t.target.Cell.Slot, t.target.Shape, arena.canonicalGuard(t.guard))
	for i, value := range t.values {
		path := "-"
		if t.paths[i] != 0 {
			path = arena.canonicalPath(t.paths[i])
		}
		out += ":" + arena.canonicalValue(value) + ":" + path
	}
	return out
}

func recordObserverCallTemplate(in []ObserverCallTemplate, next ObserverCallTemplate) []ObserverCallTemplate {
	for _, prior := range in {
		if equalObserverCallTemplate(prior, next) {
			return in
		}
	}
	return append(in, cloneObserverCallTemplate(next))
}

func unionObserverCallTemplates(arena *Arena, sets ...[]ObserverCallTemplate) []ObserverCallTemplate {
	var out []ObserverCallTemplate
	for _, set := range sets {
		for _, template := range set {
			out = recordObserverCallTemplate(out, template)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].canonical(arena) < out[j].canonical(arena) })
	return out
}

func equalObserverCallTemplates(left, right []ObserverCallTemplate) bool {
	if len(left) != len(right) {
		return false
	}
	used := make([]bool, len(right))
	for _, item := range left {
		found := false
		for i := range right {
			if !used[i] && equalObserverCallTemplate(item, right[i]) {
				used[i], found = true, true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
