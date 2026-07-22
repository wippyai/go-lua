package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// formalValue is the scalar codomain carried by formal Values fibers.  The
// concrete arm is deliberately stored inline: the overwhelmingly common
// concrete path remains the existing product.Value representation, without an
// interface box or a heap allocation.  The symbolic arm retains one owned,
// interned binding-term terminal until the entry-specialization boundary
// resolves it.
type formalValue struct {
	ground       product.Value
	symbolicLeaf decisionLeaf
	isSymbolic   bool
}

func formalGroundValue(value product.Value) formalValue { return formalValue{ground: value} }

func formalSymbolicValue(leaf decisionLeaf) formalValue {
	return formalValue{symbolicLeaf: leaf, isSymbolic: true}
}

func (v formalValue) concrete() (product.Value, bool) {
	return v.ground, !v.isSymbolic
}

func (v formalValue) validFor(authority *formalComponentTerminalAuthority) bool {
	if authority == nil {
		return false
	}
	if !v.isSymbolic {
		return product.BelongsToRegistry(authority.product.Registry(), v.ground)
	}
	// Values has no correlated path payload.  The symbolic arm names its
	// authority-owned, interned binding terminal directly: no reconstruction of
	// a ValueTerm join (and therefore no allocation) is needed on a read.
	terminal, err := authority.terminal(v.symbolicLeaf)
	if err != nil || terminal.kind != formalComponentBindings || len(terminal.bindings) == 0 {
		return false
	}
	for _, binding := range terminal.bindings {
		if binding.pathPresent || binding.apply.present() || !binding.validForAuthority(authority) {
			return false
		}
	}
	return true
}

// formalValuesFactor is the Values finite-map lift over formalValue.  It is
// intentionally transformer-local: concrete State values retain their exact,
// allocation-free product.Value map representation.
type formalValuesFactor struct {
	Top    bool
	Values map[FormalSlot]formalValue
}

// formalSymbolicValueSet is the interned symbolic representation used when a
// Values lattice operation joins concrete and symbolic alternatives. Concrete
// alternatives are reduced with the product law; binding alternatives retain
// their canonical, path-free symbolic set. This avoids extending a sealed
// Arena with post-freeze Constant/Join syntax.
type formalSymbolicValueSet struct {
	hasGround bool
	ground    product.Value
	bindings  []formalQualifiedBinding
}

func (s formalSymbolicValueSet) validFor(authority *formalComponentTerminalAuthority) bool {
	if authority == nil || s.hasGround && !product.BelongsToRegistry(authority.product.Registry(), s.ground) || len(s.bindings) == 0 {
		return false
	}
	for _, binding := range s.bindings {
		if binding.pathPresent || binding.apply.present() || !binding.validForAuthority(authority) {
			return false
		}
	}
	return true
}

func (s formalSymbolicValueSet) equal(registry *axis.Registry, other formalSymbolicValueSet) bool {
	return s.hasGround == other.hasGround && (!s.hasGround || product.Equal(registry, s.ground, other.ground)) &&
		formalQualifiedBindingsEqual(s.bindings, other.bindings)
}

// lessOrEq is the order of the formal Values sum. Ground alternatives retain
// the registered product order; symbolic alternatives are compared by their
// canonical spelling set, never by an entry-materialized value or term pointer.
func (s formalSymbolicValueSet) lessOrEq(registry *axis.Registry, other formalSymbolicValueSet) bool {
	if s.hasGround && (!other.hasGround || !product.LessOrEq(registry, s.ground, other.ground)) {
		return false
	}
	return formalQualifiedBindingsSubset(s.bindings, other.bindings)
}

func formalValuesFactorRelation(authority *formalComponentTerminalAuthority, left, right formalValuesFactor, order bool) (bool, error) {
	if authority == nil || left.Top && len(left.Values) != 0 || right.Top && len(right.Values) != 0 {
		return false, errFormalComponentMalformed
	}
	if left.Top || right.Top {
		if order {
			return right.Top, nil
		}
		return left.Top == right.Top, nil
	}
	registry := authority.product.Registry()
	bottom := product.Bottom(registry)
	seen := make(map[FormalSlot]struct{}, len(left.Values)+len(right.Values))
	for slot := range left.Values {
		seen[slot] = struct{}{}
	}
	for slot := range right.Values {
		seen[slot] = struct{}{}
	}
	for slot := range seen {
		l, present := left.Values[slot]
		if !present {
			l = formalGroundValue(bottom)
		}
		r, present := right.Values[slot]
		if !present {
			r = formalGroundValue(bottom)
		}
		leftSet, err := formalSymbolicValueSetFromFormalValue(authority, l)
		if err != nil {
			return false, err
		}
		rightSet, err := formalSymbolicValueSetFromFormalValue(authority, r)
		if err != nil {
			return false, err
		}
		if order {
			if !leftSet.lessOrEq(registry, rightSet) {
				return false, nil
			}
		} else if !leftSet.equal(registry, rightSet) {
			return false, nil
		}
	}
	return true, nil
}

func formalSymbolicValueSetFromFormalValue(authority *formalComponentTerminalAuthority, value formalValue) (formalSymbolicValueSet, error) {
	if ground, concrete := value.concrete(); concrete {
		return formalSymbolicValueSet{hasGround: true, ground: ground}, nil
	}
	return formalSymbolicValueSetFromLeaf(authority, value.symbolicLeaf)
}

func formalValueFromLeaf(authority *formalComponentTerminalAuthority, leaf decisionLeaf) (formalValue, error) {
	if authority == nil {
		return formalValue{}, errFormalComponentForeignOwner
	}
	if leaf == 0 {
		return formalGroundValue(product.Bottom(authority.product.Registry())), nil
	}
	terminal, err := authority.terminal(leaf)
	if err != nil {
		return formalValue{}, err
	}
	switch terminal.kind {
	case formalComponentGroundValue:
		if !product.BelongsToRegistry(authority.product.Registry(), terminal.ground) {
			return formalValue{}, errFormalComponentMalformed
		}
		return formalGroundValue(terminal.ground), nil
	case formalComponentBindings:
		if len(terminal.bindings) == 0 {
			return formalValue{}, errFormalComponentMalformed
		}
		for _, binding := range terminal.bindings {
			if binding.pathPresent || binding.apply.present() || !binding.validForAuthority(authority) {
				return formalValue{}, errFormalComponentMalformed
			}
		}
		return formalSymbolicValue(leaf), nil
	case formalComponentSymbolicValue:
		if !terminal.symbolicValue.validFor(authority) {
			return formalValue{}, errFormalComponentMalformed
		}
		return formalSymbolicValue(leaf), nil
	default:
		return formalValue{}, errFormalComponentMalformed
	}
}

func formalSymbolicValueSetFromLeaf(authority *formalComponentTerminalAuthority, leaf decisionLeaf) (formalSymbolicValueSet, error) {
	if authority == nil {
		return formalSymbolicValueSet{}, errFormalComponentForeignOwner
	}
	if leaf == 0 {
		return formalSymbolicValueSet{hasGround: true, ground: product.Bottom(authority.product.Registry())}, nil
	}
	terminal, err := authority.terminal(leaf)
	if err != nil {
		return formalSymbolicValueSet{}, err
	}
	switch terminal.kind {
	case formalComponentGroundValue:
		return formalSymbolicValueSet{hasGround: true, ground: terminal.ground}, nil
	case formalComponentBindings:
		return formalSymbolicValueSet{bindings: terminal.bindings}, nil
	case formalComponentSymbolicValue:
		return terminal.symbolicValue, nil
	default:
		return formalSymbolicValueSet{}, errFormalComponentMalformed
	}
}

func (a *formalComponentTerminalAuthority) internSymbolicValueSet(value formalSymbolicValueSet) (decisionLeaf, error) {
	if !value.validFor(a) {
		return 0, errFormalComponentForeignOwner
	}
	fingerprint := formalComponentSetFingerprint(formalComponentSymbolicValue, func(index int) uint64 {
		if index == 0 && value.hasGround {
			return product.Hash(a.product.Registry(), value.ground)
		}
		bindingIndex := index
		if value.hasGround {
			bindingIndex--
		}
		return value.bindings[bindingIndex].fingerprint()
	}, len(value.bindings)+boolToInt(value.hasGround))
	bucket := formalComponentBucket{owner: a.body, kind: formalComponentSymbolicValue, width: uint32(len(value.bindings)), fingerprint: fingerprint}
	for _, leaf := range a.arena.buckets[bucket] {
		prior := a.arena.terminals[leaf].terminal
		if prior.kind == formalComponentSymbolicValue && prior.symbolicValue.equal(a.product.Registry(), value) {
			return leaf, nil
		}
	}
	value.bindings = append([]formalQualifiedBinding(nil), value.bindings...)
	return a.arena.append(a, formalComponentTerminal{kind: formalComponentSymbolicValue, symbolicValue: value, fingerprint: fingerprint}, bucket)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (a *formalComponentTerminalAuthority) joinFormalValueLeaves(left, right decisionLeaf) (decisionLeaf, error) {
	leftSet, err := formalSymbolicValueSetFromLeaf(a, left)
	if err != nil {
		return 0, err
	}
	rightSet, err := formalSymbolicValueSetFromLeaf(a, right)
	if err != nil {
		return 0, err
	}
	out := formalSymbolicValueSet{bindings: unionFormalQualifiedBindings(leftSet.bindings, rightSet.bindings)}
	if leftSet.hasGround && rightSet.hasGround {
		out.hasGround = true
		out.ground = product.Join(a.product.Registry(), leftSet.ground, rightSet.ground)
	} else if leftSet.hasGround {
		out.hasGround, out.ground = true, leftSet.ground
	} else if rightSet.hasGround {
		out.hasGround, out.ground = true, rightSet.ground
	}
	if len(out.bindings) == 0 {
		return a.internGroundValue(out.ground)
	}
	if !out.hasGround {
		return a.internBindings(out.bindings)
	}
	return a.internSymbolicValueSet(out)
}

func (a *formalComponentTerminalAuthority) internFormalValue(value formalValue) (decisionLeaf, error) {
	if !value.validFor(a) {
		return 0, errFormalComponentForeignOwner
	}
	if !value.isSymbolic {
		return a.internGroundValue(value.ground)
	}
	return value.symbolicLeaf, nil
}

func formalConcreteValuesFactor(authority *formalComponentTerminalAuthority, values formalValuesFactor) (map[FormalSlot]product.Value, error) {
	if authority == nil || values.Top && len(values.Values) != 0 {
		return nil, errFormalComponentMalformed
	}
	if values.Top || len(values.Values) == 0 {
		return nil, nil
	}
	out := make(map[FormalSlot]product.Value, len(values.Values))
	for slot, value := range values.Values {
		ground, exact := value.concrete()
		if !exact || !product.BelongsToRegistry(authority.product.Registry(), ground) {
			return nil, fmt.Errorf("%w: symbolic Values requires entry specialization", errFormalComponentMalformed)
		}
		out[slot] = ground
	}
	return out, nil
}
