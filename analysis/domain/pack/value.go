package pack

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/static"
)

// Value is one immutable normalized relation for a single Pack schema. It is
// Bottom, a finite antichain of exact/open cases, or the one binder-free
// all-unknown case. It carries neither an engine slot nor a persisted class or
// substitution map; algebra is only the local sealed schema fence.
type Value struct {
	owner    *algebra
	relation *relation
	cases    []Case
	bottom   bool
	top      bool
	sealed   bool
	hash     uint64
	rank     [4]uint64
}

func bottomValue(owner *algebra) Value {
	return finishValue(Value{owner: owner, bottom: true})
}

func topValue(owner *algebra) Value {
	return finishValue(Value{owner: owner, top: true})
}

func valueFromCases(relation *relation, cases []Case) (Value, bool) {
	if !relation.valid() {
		return Value{}, false
	}
	owner := relation.owner
	if len(cases) == 0 {
		return bottomValue(owner), true
	}
	copyOf := append([]Case(nil), cases...)
	for _, value := range copyOf {
		if !value.valid() || value.owner != owner || (!value.top && value.relation != relation) {
			return Value{}, false
		}
		if value.top {
			return topValue(owner), true
		}
	}
	sort.Slice(copyOf, func(left, right int) bool { return compareCase(copyOf[left], copyOf[right]) < 0 })
	kept := copyOf[:0]
	for _, candidate := range copyOf {
		covered := false
		for _, prior := range kept {
			if caseCovers(prior, candidate) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		// A newly retained wider case can cover an earlier precise case. Remove
		// it now so every Value is an antichain independent of input order.
		write := 0
		for _, prior := range kept {
			if !caseCovers(candidate, prior) {
				kept[write] = prior
				write++
			}
		}
		kept = kept[:write]
		kept = append(kept, candidate)
	}
	if len(kept) == 1 && kept[0].top {
		return topValue(owner), true
	}
	return finishValue(Value{owner: owner, relation: relation, cases: append([]Case(nil), kept...)}), true
}

func (value Value) valid() bool {
	if !value.sealed || value.owner == nil || !value.owner.valid() {
		return false
	}
	if value.bottom || value.top {
		return value.relation == nil && len(value.cases) == 0 && value.bottom != value.top
	}
	return value.relation != nil && value.relation.valid() && value.relation.owner == value.owner && len(value.cases) != 0
}

// finishValue seals a Value whose cases have already been normalized by
// valueFromCases (or are the canonical empty/top constructors). It checks only
// linear ownership/order invariants: repeating the coverage proof here would
// turn one admission into a second quadratic antichain pass.
func finishValue(value Value) Value {
	if value.owner == nil || !value.owner.valid() {
		return Value{}
	}
	if value.bottom || value.top {
		if value.bottom == value.top || value.relation != nil || len(value.cases) != 0 {
			return Value{}
		}
	} else if value.relation == nil || !value.relation.valid() || value.relation.owner != value.owner {
		return Value{}
	} else {
		for index, current := range value.cases {
			if !current.valid() || current.owner != value.owner || current.relation != value.relation || (index > 0 && compareCase(value.cases[index-1], current) >= 0) {
				return Value{}
			}
		}
	}
	value.sealed = true
	switch {
	case value.top:
		value.rank[0] = 0
	case value.bottom:
		value.rank[0] = 3
	case len(value.cases) > 1:
		value.rank[0] = 2
	default:
		value.rank[0] = 1
		value.rank[1] = value.cases[0].shapeRank
		value.rank[2] = value.cases[0].syntaxRank
		value.rank[3] = value.cases[0].classRank
	}
	h := newFNV64()
	h.write([]byte("wippy.analysis.pack/value\x00\x03"))
	if value.bottom {
		writeUint64(&h, 1)
	}
	if value.top {
		writeUint64(&h, 2)
	}
	if value.relation != nil {
		writeUint64(&h, uint64(value.relation.index))
	}
	for _, current := range value.cases {
		writeUint64(&h, current.hash)
	}
	value.hash = uint64(h)
	return value
}

func (value Value) IsBottom() bool { return value.valid() && value.bottom }

func (value Value) IsTop() bool { return value.valid() && value.top }

func sameValue(left, right Value) (*algebra, bool) {
	return left.owner, left.owner != nil && left.owner == right.owner && left.valid() && right.valid()
}

func equalValue(left, right Value) bool {
	_, ok := sameValue(left, right)
	if !ok || left.bottom != right.bottom || left.top != right.top || left.relation != right.relation || len(left.cases) != len(right.cases) {
		return false
	}
	for index := range left.cases {
		if !equalCase(left.cases[index], right.cases[index]) {
			return false
		}
	}
	return true
}

func lessOrEqualValue(left, right Value) bool {
	_, ok := sameValue(left, right)
	if !ok {
		return false
	}
	if left.bottom || right.top {
		return true
	}
	if left.top || right.bottom || left.relation != right.relation {
		return false
	}
	for _, leftCase := range left.cases {
		covered := false
		for _, rightCase := range right.cases {
			if caseCovers(rightCase, leftCase) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

func joinValue(left, right Value) (Value, bool) {
	owner, ok := sameValue(left, right)
	if !ok {
		return Value{}, false
	}
	if left.bottom {
		return right, true
	}
	if right.bottom {
		return left, true
	}
	if left.top || right.top {
		return topValue(owner), true
	}
	if left.relation != right.relation {
		return Value{}, false
	}
	cases := make([]Case, 0, len(left.cases)+len(right.cases))
	cases = append(cases, left.cases...)
	cases = append(cases, right.cases...)
	return valueFromCases(left.relation, cases)
}

// widenValue performs the one lawful recurrence widening. Acyclic transfer
// uses joinValue. At a Mu boundary, non-unanimous cases become one generalized
// case; no counter, height budget, or "too many cases" escape exists.
func widenValue(previous, next Value) (Value, bool) {
	_, ok := sameValue(previous, next)
	if !ok {
		return Value{}, false
	}
	joined, ok := joinValue(previous, next)
	if !ok || joined.IsBottom() || joined.IsTop() || len(joined.cases) == 1 {
		return joined, ok
	}
	generalized, ok := generalizeCases(joined.relation, joined.cases)
	if !ok {
		return Value{}, false
	}
	return valueFromCases(joined.relation, []Case{generalized})
}

func valueRank(value Value, component int) uint64 {
	if !value.valid() {
		return 0
	}
	if component < 0 || component >= len(value.rank) {
		return 0
	}
	return value.rank[component]
}

func valueFingerprint(value Value) uint64 {
	if !value.valid() {
		return 0
	}
	return value.hash
}

func generalizeCases(relation *relation, cases []Case) (Case, bool) {
	if !relation.valid() || len(cases) == 0 {
		return Case{}, false
	}
	owner := relation.owner
	for _, current := range cases {
		if !current.valid() || current.owner != owner || current.relation != relation {
			return Case{}, false
		}
		if current.top {
			return topCase(owner)
		}
	}
	count := len(cases[0].equations)
	for _, current := range cases[1:] {
		if len(current.equations) != count {
			return topCase(owner)
		}
	}
	equations := make([]Equation, count)
	for index := 0; index < count; index++ {
		first := cases[0].equations[index]
		for _, current := range cases[1:] {
			if compareEquationTarget(first, current.equations[index]) != 0 || first.kind != current.equations[index].kind {
				return topCase(owner)
			}
		}
		switch first.kind {
		case EquationScalar:
			values := make([]Scalar, len(cases))
			for caseIndex, current := range cases {
				values[caseIndex] = current.equations[index].scalar
			}
			scalar, ok := generalizeScalars(owner, values)
			if !ok {
				return Case{}, false
			}
			equations[index], ok = scalarEquation(first.endpoint, scalar)
			if !ok {
				return Case{}, false
			}
		case EquationPack:
			values := make([]Term, len(cases))
			for caseIndex, current := range cases {
				values[caseIndex] = current.equations[index].term
			}
			term, ok := generalizeTerms(owner, values)
			if !ok {
				return Case{}, false
			}
			equations[index], ok = packEquation(first.port, term)
			if !ok {
				return Case{}, false
			}
		default:
			return Case{}, false
		}
	}
	return exactCase(relation, equations)
}

func generalizeScalars(owner *algebra, values []Scalar) (Scalar, bool) {
	if owner == nil || len(values) == 0 {
		return Scalar{}, false
	}
	if allSameScalars(values) {
		return values[0], true
	}
	class, ok := joinScalarClasses(owner, values)
	if !ok {
		return Scalar{}, false
	}
	return anyScalar(owner, class)
}

func generalizeTerms(owner *algebra, values []Term) (Term, bool) {
	if owner == nil || len(values) == 0 {
		return Term{}, false
	}
	if allSameTerms(values) {
		return values[0], true
	}
	for _, value := range values {
		if !value.valid() || value.owner != owner || value.kind == TermAny {
			return anyTerm(owner)
		}
	}
	allClosed := true
	allOpen := true
	for _, value := range values {
		allClosed = allClosed && value.kind == TermClosed
		allOpen = allOpen && value.kind == TermOpen
	}
	if !allClosed && !allOpen {
		return anyTerm(owner)
	}
	prefixes := make([][]Scalar, len(values))
	suffixes := make([][]Scalar, len(values))
	for index, value := range values {
		prefixes[index], suffixes[index] = value.prefix, value.suffix
	}
	prefix := commonPrefix(prefixes)
	suffix := commonSuffix(prefixes, prefix)
	if allOpen {
		// Open suffixes are end-relative; use them rather than treating an
		// arbitrary middle as a finite vector.
		suffix = commonSuffix(suffixes, nil)
	}
	class, ok := joinTermClasses(owner, values)
	if !ok {
		return Term{}, false
	}
	rest, ok := anyRest(owner, class)
	if !ok {
		return Term{}, false
	}
	return openTerm(owner, prefix, rest, suffix)
}

func joinScalarClasses(owner *algebra, values []Scalar) (static.Class, bool) {
	if owner == nil || len(values) == 0 {
		return static.Class{}, false
	}
	class := values[0].class
	if !values[0].valid() || values[0].owner != owner {
		return static.Class{}, false
	}
	for _, value := range values[1:] {
		if !value.valid() || value.owner != owner {
			return static.Class{}, false
		}
		var ok bool
		class, ok = owner.joinClass(class, value.class)
		if !ok {
			return static.Class{}, false
		}
	}
	return class, true
}

func joinTermClasses(owner *algebra, values []Term) (static.Class, bool) {
	if owner == nil || len(values) == 0 {
		return static.Class{}, false
	}
	classes := make([]static.Class, 0)
	for _, value := range values {
		if !value.valid() || value.owner != owner {
			return static.Class{}, false
		}
		for _, scalar := range value.prefix {
			classes = append(classes, scalar.class)
		}
		if value.kind == TermOpen {
			classes = append(classes, value.rest.class)
			for _, scalar := range value.suffix {
				classes = append(classes, scalar.class)
			}
		}
	}
	if len(classes) == 0 {
		return owner.classes.AnyValue(), true
	}
	class := classes[0]
	for _, next := range classes[1:] {
		var ok bool
		class, ok = owner.joinClass(class, next)
		if !ok {
			return static.Class{}, false
		}
	}
	return class, true
}

func allSameScalars(values []Scalar) bool {
	for index := 1; index < len(values); index++ {
		if !equalScalar(values[0], values[index]) {
			return false
		}
	}
	return len(values) > 0
}

func allSameTerms(values []Term) bool {
	for index := 1; index < len(values); index++ {
		if !equalTerm(values[0], values[index]) {
			return false
		}
	}
	return len(values) > 0
}

func commonPrefix(values [][]Scalar) []Scalar {
	if len(values) == 0 {
		return nil
	}
	limit := len(values[0])
	for _, value := range values[1:] {
		if len(value) < limit {
			limit = len(value)
		}
	}
	for index := 0; index < limit; index++ {
		for _, value := range values[1:] {
			if !equalScalar(values[0][index], value[index]) {
				return cloneScalars(values[0][:index])
			}
		}
	}
	return cloneScalars(values[0][:limit])
}

func commonSuffix(values [][]Scalar, prefix []Scalar) []Scalar {
	if len(values) == 0 {
		return nil
	}
	limit := len(values[0]) - len(prefix)
	for _, value := range values[1:] {
		available := len(value) - len(prefix)
		if available < limit {
			limit = available
		}
	}
	for length := 1; length <= limit; length++ {
		first := values[0][len(values[0])-length]
		for _, value := range values[1:] {
			if !equalScalar(first, value[len(value)-length]) {
				return cloneScalars(values[0][len(values[0])-length+1:])
			}
		}
	}
	return cloneScalars(values[0][len(values[0])-limit:])
}

func equalCase(left, right Case) bool {
	if left.owner != right.owner || left.relation != right.relation || left.top != right.top || len(left.equations) != len(right.equations) {
		return false
	}
	for index := range left.equations {
		if !equalEquation(left.equations[index], right.equations[index]) {
			return false
		}
	}
	return true
}

func compareCase(left, right Case) int {
	if left.top != right.top {
		if left.top {
			return 1
		}
		return -1
	}
	if len(left.equations) < len(right.equations) {
		return -1
	}
	if len(left.equations) > len(right.equations) {
		return 1
	}
	for index := range left.equations {
		if comparison := compareEquationValue(left.equations[index], right.equations[index]); comparison != 0 {
			return comparison
		}
	}
	return 0
}

func equalEquation(left, right Equation) bool {
	return compareEquationTarget(left, right) == 0 && left.kind == right.kind &&
		((left.kind == EquationScalar && equalScalar(left.scalar, right.scalar)) ||
			(left.kind == EquationPack && equalTerm(left.term, right.term)))
}

func compareEquationValue(left, right Equation) int {
	if comparison := compareEquation(left, right); comparison != 0 {
		return comparison
	}
	switch left.kind {
	case EquationScalar:
		return compareScalar(left.scalar, right.scalar)
	case EquationPack:
		return compareTerm(left.term, right.term)
	default:
		return 0
	}
}

// caseCovers means left denotes a superset of right's valuations. The
// structural order intentionally avoids an arbitrary Pack-language inclusion
// solver: exact expressions cover only themselves, and unknown forms cover
// only classes admitted by the same sealed Static authority.
func caseCovers(left, right Case) bool {
	if !left.valid() || !right.valid() || left.owner != right.owner {
		return false
	}
	if left.top {
		return true
	}
	if right.top || left.relation != right.relation || len(left.equations) != len(right.equations) {
		return false
	}
	for index := range left.equations {
		leftEquation, rightEquation := left.equations[index], right.equations[index]
		if compareEquationTarget(leftEquation, rightEquation) != 0 || leftEquation.kind != rightEquation.kind {
			return false
		}
		switch leftEquation.kind {
		case EquationScalar:
			if !scalarCovers(leftEquation.scalar, rightEquation.scalar) {
				return false
			}
		case EquationPack:
			if !termCovers(leftEquation.term, rightEquation.term) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func equalScalar(left, right Scalar) bool {
	if left.owner != right.owner || left.kind != right.kind || left.owner == nil || !left.owner.equalClass(left.class, right.class) {
		return false
	}
	switch left.kind {
	case ScalarEndpoint:
		return left.endpoint == right.endpoint
	case ScalarHead:
		return equalTail(left.tail, right.tail) && compareOffset(left.offset, right.offset) == 0
	case ScalarAny:
		return true
	default:
		return false
	}
}

func compareScalar(left, right Scalar) int {
	if left.kind < right.kind {
		return -1
	}
	if left.kind > right.kind {
		return 1
	}
	if class := compareClass(left.owner, left.class, right.class); class != 0 {
		return class
	}
	switch left.kind {
	case ScalarEndpoint:
		if left.endpoint.index < right.endpoint.index {
			return -1
		}
		if left.endpoint.index > right.endpoint.index {
			return 1
		}
	case ScalarHead:
		if tail := compareTail(left.tail, right.tail); tail != 0 {
			return tail
		}
		return compareOffset(left.offset, right.offset)
	}
	return 0
}

func scalarCovers(left, right Scalar) bool {
	if !left.valid() || !right.valid() || left.owner != right.owner {
		return false
	}
	if left.kind == ScalarAny {
		return left.owner.lessClass(right.class, left.class)
	}
	return equalScalar(left, right)
}

func equalTerm(left, right Term) bool {
	return compareTerm(left, right) == 0 && left.owner == right.owner
}

func compareTerm(left, right Term) int {
	if left.kind < right.kind {
		return -1
	}
	if left.kind > right.kind {
		return 1
	}
	if len(left.prefix) < len(right.prefix) {
		return -1
	}
	if len(left.prefix) > len(right.prefix) {
		return 1
	}
	for index := range left.prefix {
		if comparison := compareScalar(left.prefix[index], right.prefix[index]); comparison != 0 {
			return comparison
		}
	}
	if left.kind == TermOpen {
		if comparison := compareRest(left.rest, right.rest); comparison != 0 {
			return comparison
		}
		if len(left.suffix) < len(right.suffix) {
			return -1
		}
		if len(left.suffix) > len(right.suffix) {
			return 1
		}
		for index := range left.suffix {
			if comparison := compareScalar(left.suffix[index], right.suffix[index]); comparison != 0 {
				return comparison
			}
		}
	}
	return 0
}

func termCovers(left, right Term) bool {
	if !left.valid() || !right.valid() || left.owner != right.owner {
		return false
	}
	if left.kind == TermAny {
		return true
	}
	if equalTerm(left, right) {
		return true
	}
	if left.kind != TermOpen {
		return false
	}
	if right.kind == TermClosed {
		if len(right.prefix) < len(left.prefix)+len(left.suffix) {
			return false
		}
		for index := range left.prefix {
			if !scalarCovers(left.prefix[index], right.prefix[index]) {
				return false
			}
		}
		for index := range left.suffix {
			rightIndex := len(right.prefix) - len(left.suffix) + index
			if !scalarCovers(left.suffix[index], right.prefix[rightIndex]) {
				return false
			}
		}
		if left.rest.kind != RestAny {
			return false
		}
		for index := len(left.prefix); index < len(right.prefix)-len(left.suffix); index++ {
			if !left.owner.lessClass(right.prefix[index].class, left.rest.class) {
				return false
			}
		}
		return true
	}
	if right.kind != TermOpen || len(left.prefix) != len(right.prefix) || len(left.suffix) != len(right.suffix) {
		return false
	}
	for index := range left.prefix {
		if !scalarCovers(left.prefix[index], right.prefix[index]) {
			return false
		}
	}
	for index := range left.suffix {
		if !scalarCovers(left.suffix[index], right.suffix[index]) {
			return false
		}
	}
	if left.rest.kind == RestAny {
		return left.owner.lessClass(right.rest.class, left.rest.class)
	}
	return equalRest(left.rest, right.rest)
}

func equalTail(left, right TailRef) bool {
	return left.owner == right.owner && left.kind == right.kind && left.index == right.index && left.owner != nil && left.owner.equalClass(left.class, right.class) &&
		left.port.owner == right.port.owner && left.port.index == right.port.index && left.port.free == right.port.free &&
		(left.port.owner == nil || left.port.owner.equalClass(left.port.class, right.port.class))
}

func compareTail(left, right TailRef) int {
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
	return compareClass(left.owner, left.class, right.class)
}

func equalRest(left, right Rest) bool {
	return compareRest(left, right) == 0 && left.owner == right.owner
}

func compareRest(left, right Rest) int {
	if left.kind < right.kind {
		return -1
	}
	if left.kind > right.kind {
		return 1
	}
	if class := compareClass(left.owner, left.class, right.class); class != 0 {
		return class
	}
	if left.kind == RestTail {
		if tail := compareTail(left.tail, right.tail); tail != 0 {
			return tail
		}
		return compareOffset(left.offset, right.offset)
	}
	return 0
}

func compareClass(owner *algebra, left, right static.Class) int {
	if owner == nil || !owner.admits(left) || !owner.admits(right) {
		return 0
	}
	if owner.equalClass(left, right) {
		return 0
	}
	leftID, leftOK := owner.classIdentity(left)
	rightID, rightOK := owner.classIdentity(right)
	if !leftOK || !rightOK {
		return 0
	}
	return bytes.Compare(leftID[:], rightID[:])
}

// The rank is lexicographic, not one summed "precision" score. A generalized
// empty closed list can acquire an AnyTail class, whose class rank may be
// numerically larger than the empty list's. Its Pack-form component nevertheless
// strictly falls Exact(2) -> Open(1). Collapsing these components into one sum
// would falsely claim nontermination on that lawful widening step.
func rawCaseShapeRank(value Case) uint64 {
	if value.owner == nil || value.top {
		return 0
	}
	rank := uint64(0)
	for _, equation := range value.equations {
		if equation.kind == EquationPack {
			switch equation.term.kind {
			case TermClosed:
				rank += 2
			case TermOpen:
				rank++
			}
		}
	}
	return rank
}

func rawCaseSyntaxRank(value Case) uint64 {
	if value.owner == nil || value.top {
		return 0
	}
	rank := uint64(0)
	for _, equation := range value.equations {
		switch equation.kind {
		case EquationScalar:
			rank += scalarSyntaxRank(equation.scalar)
		case EquationPack:
			rank += termSyntaxRank(equation.term)
		}
	}
	return rank
}

func scalarSyntaxRank(value Scalar) uint64 {
	if !value.sealed || value.owner == nil {
		return 0
	}
	if value.kind != ScalarAny {
		return 1
	}
	return 0
}

func termSyntaxRank(value Term) uint64 {
	if !value.sealed || value.owner == nil || value.kind == TermAny {
		return 0
	}
	rank := uint64(0)
	if value.kind == TermOpen && value.rest.kind == RestTail {
		rank++
	}
	for _, scalar := range value.prefix {
		rank += scalarSyntaxRank(scalar)
	}
	for _, scalar := range value.suffix {
		rank += scalarSyntaxRank(scalar)
	}
	return rank
}

func rawCaseClassRank(value Case) uint64 {
	if value.owner == nil || value.top {
		return 0
	}
	rank := uint64(0)
	for _, equation := range value.equations {
		switch equation.kind {
		case EquationScalar:
			rank += scalarClassRank(equation.scalar)
		case EquationPack:
			rank += termClassRank(equation.term)
		}
	}
	return rank
}

func scalarClassRank(value Scalar) uint64 {
	if !value.sealed || value.owner == nil {
		return 0
	}
	return value.owner.classRank(value.class)
}

func termClassRank(value Term) uint64 {
	if !value.sealed || value.owner == nil || value.kind == TermAny {
		return 0
	}
	rank := uint64(0)
	for _, scalar := range value.prefix {
		rank += scalarClassRank(scalar)
	}
	if value.kind == TermOpen {
		rank += value.owner.classRank(value.rest.class)
	}
	for _, scalar := range value.suffix {
		rank += scalarClassRank(scalar)
	}
	return rank
}

func writeCaseHash(h *fnv64, value Case) {
	if value.top {
		h.byte(0xff)
		return
	}
	writeUint64(h, uint64(value.relation.index))
	for _, equation := range value.equations {
		h.byte(byte(equation.kind))
		switch equation.kind {
		case EquationScalar:
			writeUint64(h, uint64(equation.endpoint.index))
			writeScalarHash(h, equation.scalar)
		case EquationPack:
			writeUint64(h, uint64(equation.port.index))
			writeTermHash(h, equation.term)
		}
	}
}

func writeScalarHash(h *fnv64, value Scalar) {
	h.byte(byte(value.kind))
	writeClassHash(h, value.owner, value.class)
	writeUint64(h, uint64(value.endpoint.index))
	writeTailHash(h, value.tail)
	writeUint64(h, uint64(value.offset.index))
}

func writeTermHash(h *fnv64, value Term) {
	h.byte(byte(value.kind))
	writeUint64(h, uint64(len(value.prefix)))
	for _, scalar := range value.prefix {
		writeScalarHash(h, scalar)
	}
	if value.kind == TermOpen {
		h.byte(byte(value.rest.kind))
		writeClassHash(h, value.owner, value.rest.class)
		writeTailHash(h, value.rest.tail)
		writeUint64(h, uint64(value.rest.offset.index))
	}
	writeUint64(h, uint64(len(value.suffix)))
	for _, scalar := range value.suffix {
		writeScalarHash(h, scalar)
	}
}

func writeTailHash(h *fnv64, tail TailRef) {
	h.byte(byte(tail.kind))
	writeUint64(h, uint64(tail.index))
	if tail.owner != nil {
		writeClassHash(h, tail.owner, tail.class)
	}
}

func writeClassHash(h *fnv64, owner *algebra, class static.Class) {
	if owner == nil {
		return
	}
	id, ok := owner.classIdentity(class)
	if ok {
		h.write(id[:])
	}
}

type fnv64 uint64

func newFNV64() fnv64            { return fnv64(14695981039346656037) }
func (h *fnv64) byte(value byte) { *h = fnv64((uint64(*h) ^ uint64(value)) * 1099511628211) }
func (h *fnv64) write(values []byte) {
	for _, value := range values {
		h.byte(value)
	}
}
func writeUint64(h *fnv64, value uint64) {
	h.byte(byte(value >> 56))
	h.byte(byte(value >> 48))
	h.byte(byte(value >> 40))
	h.byte(byte(value >> 32))
	h.byte(byte(value >> 24))
	h.byte(byte(value >> 16))
	h.byte(byte(value >> 8))
	h.byte(byte(value))
}
