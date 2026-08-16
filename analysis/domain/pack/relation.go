package pack

// relation is the schema-complete Pack interface for one Program body.
// It contains no Link or Program object: those source facts stay in Schema.
// A non-extreme Case is admitted only when it supplies exactly this sorted
// target vector, so a caller cannot construct a partial observation of a
// root's Pack relation.
type relation struct {
	owner   *algebra
	index   uint32
	targets []equationTarget
	sealed  bool
}

type equationTarget struct {
	kind  EquationKind
	index uint32
}

func (relation *relation) valid() bool {
	return relation != nil && relation.sealed && relation.owner != nil && relation.owner.valid() && relation.index != 0 && len(relation.targets) != 0
}

// sealRelation validates the complete target vector once and freezes a
// private copy. Hot Value/SourceResult authentication thereafter checks only
// immutable scalar fences; it never rescans the target denominator.
func sealRelation(owner *algebra, index uint32, targets []equationTarget) (*relation, bool) {
	if owner == nil || !owner.valid() || index == 0 || len(targets) == 0 {
		return nil, false
	}
	frozen := append([]equationTarget(nil), targets...)
	for position, target := range frozen {
		if target.kind != EquationScalar && target.kind != EquationPack || target.index == 0 {
			return nil, false
		}
		if position != 0 && compareTarget(target, frozen[position-1]) <= 0 {
			return nil, false
		}
	}
	relation := &relation{owner: owner, index: index, targets: frozen, sealed: true}
	if !relation.valid() {
		return nil, false
	}
	return relation, true
}

func compareTarget(left, right equationTarget) int {
	if left.kind < right.kind {
		return -1
	}
	if left.kind > right.kind {
		return 1
	}
	if left.index < right.index {
		return -1
	}
	if left.index > right.index {
		return 1
	}
	return 0
}

func (relation *relation) matches(equations []Equation) bool {
	if !relation.valid() || len(equations) != len(relation.targets) || len(equations) == 0 {
		return false
	}
	for index, equation := range equations {
		if !equation.valid() || equation.owner != relation.owner || compareTarget(equationTargetFor(equation), relation.targets[index]) != 0 {
			return false
		}
	}
	return true
}

func (relation *relation) hasTarget(kind EquationKind, index uint32) bool {
	if !relation.valid() || index == 0 {
		return false
	}
	target := equationTarget{kind: kind, index: index}
	// targets are sealed in canonical sorted order. Builder calls this once for
	// each emitted equation, so a linear scan would turn a wide complete Pack
	// relation into quadratic construction work on the Rule-row path.
	low, high := 0, len(relation.targets)
	for low < high {
		middle := low + (high-low)/2
		if compareTarget(relation.targets[middle], target) < 0 {
			low = middle + 1
		} else {
			high = middle
		}
	}
	return low < len(relation.targets) && compareTarget(relation.targets[low], target) == 0
}

func equationTargetFor(equation Equation) equationTarget {
	switch equation.kind {
	case EquationScalar:
		return equationTarget{kind: equation.kind, index: equation.endpoint.index}
	case EquationPack:
		return equationTarget{kind: equation.kind, index: equation.port.index}
	default:
		return equationTarget{}
	}
}
