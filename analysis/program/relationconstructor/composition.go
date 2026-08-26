package relationconstructor

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
)

// Composition is the pre-seal declaration filter for one construction: the
// exact rule identities admitted into the declaration that construction
// seals.
//
// It is a value, not a callback. A caller states the composition once and the
// constructor reads it, so nothing downstream can widen the admitted set and
// no rule is admitted by the order a catalog happens to be walked in.
type Composition struct {
	rules []schema.Key
	total bool
}

// Everything admits every rule the catalog declares. It is the composition a
// whole-program construction states, and it is distinct from a named set that
// happens to list every rule: Everything stays total as the catalog grows.
func Everything() Composition { return Composition{total: true} }

// NewComposition admits exactly the named rules. An empty, duplicated, or
// unavailable name is refused here rather than becoming a silently smaller
// composition.
func NewComposition(rules ...schema.Key) (Composition, bool) {
	if len(rules) == 0 {
		return Composition{}, false
	}
	admitted := make([]schema.Key, 0, len(rules))
	for _, key := range rules {
		if !key.Available() {
			return Composition{}, false
		}
		admitted = append(admitted, key)
	}
	sort.Slice(admitted, func(left, right int) bool { return admitted[left] < admitted[right] })
	for index := 1; index < len(admitted); index++ {
		if admitted[index-1] == admitted[index] {
			return Composition{}, false
		}
	}
	return Composition{rules: admitted}, true
}

// Available reports whether this composition admits anything.
func (composition Composition) Available() bool {
	return composition.total || len(composition.rules) > 0
}

// Total reports whether this composition admits every declared rule.
func (composition Composition) Total() bool { return composition.total }

// Names returns the admitted rule identities in sorted order. A total
// composition names none: it is defined by the catalog, not by a list.
func (composition Composition) Names() []schema.Key {
	if composition.total || len(composition.rules) == 0 {
		return nil
	}
	return append([]schema.Key(nil), composition.rules...)
}

// Admits reports whether one rule identity participates in this construction.
func (composition Composition) Admits(key schema.Key) bool {
	if !composition.Available() || !key.Available() {
		return false
	}
	if composition.total {
		return true
	}
	index := sort.Search(len(composition.rules), func(index int) bool { return composition.rules[index] >= key })
	return index < len(composition.rules) && composition.rules[index] == key
}

// Selection is one admitted rule and the catalog ordinal it keeps.
//
// The ordinal is carried rather than recomputed. A rule's role is its
// declaration position in the catalog, and the relation input bundle states
// one row per dense catalog ordinal, so an admitted rule must answer under the
// position the catalog declared it at. Renumbering the survivors of a filter
// would silently move every rule after an excluded one onto another rule's
// identity.
type Selection struct {
	Ordinal int
	Spec    rule.Spec
}

// Select applies this composition to one declared rule catalog.
//
// Selected rules keep catalog order and their original ordinals. Absent names
// every rule this composition admits that the catalog does not declare, in
// sorted order: a composition that does not fit its catalog is reported with
// the exact names rather than quietly producing a smaller program. The third
// result reports whether the catalog itself is well formed, so an unavailable
// composition and a catalog with an unavailable or duplicated rule key are
// refusals, not empty selections.
func (composition Composition) Select(specs []rule.Spec) (selected []Selection, absent []schema.Key, ok bool) {
	if !composition.Available() {
		return nil, nil, false
	}
	declared := make(map[schema.Key]struct{}, len(specs))
	selected = make([]Selection, 0, len(specs))
	for ordinal, spec := range specs {
		if !spec.Key.Available() {
			return nil, nil, false
		}
		if _, duplicate := declared[spec.Key]; duplicate {
			return nil, nil, false
		}
		declared[spec.Key] = struct{}{}
		if composition.Admits(spec.Key) {
			selected = append(selected, Selection{Ordinal: ordinal, Spec: spec})
		}
	}
	for _, key := range composition.rules {
		if _, present := declared[key]; !present {
			absent = append(absent, key)
		}
	}
	return selected, absent, true
}
