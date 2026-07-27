package engine

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/domain/constraint/decision"
	"github.com/wippyai/go-lua/analysis/domain/constraint/numeric"
	"github.com/wippyai/go-lua/analysis/domain/constraint/solver"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// affineTermEncoding is the closed value of one affine identity: the index term
// this expression produced equals base + offset. It names the base by the same
// term spelling the value and epoch lanes use, so the base's own replacement
// event is directly readable from it.
const affineTermEncoding = "affine-term/v1/"

type branchResidueClassWire struct {
	Modulus int64 `json:"modulus"`
	Residue int64 `json:"residue"`
	Negated bool  `json:"negated,omitempty"`
}

// trueEdgeBranchDifferences collects the difference descriptors this branch
// proves on its true edge.
func trueEdgeBranchDifferences(operation equation.BoundEquation) ([]branchDiffWire, error) {
	var out []branchDiffWire
	for _, operand := range operation.Operands {
		if !operand.Role.InFamily(equation.RoleFamilyDifference) {
			continue
		}
		wire, present, err := front.DecodeBranchDiffWire(operand.Value)
		if err != nil {
			return nil, fmt.Errorf("engine: decode difference operand %q: %w", operand.Role, err)
		}
		if !present {
			return nil, fmt.Errorf("engine: difference operand %q has no difference wire", operand.Role)
		}
		if wire.Edge {
			out = append(out, wire)
		}
	}
	return out, nil
}

// artifactTrueEdgeLengthRelation reports a published difference descriptor that
// relates an operand to an array length on the branch's true edge. It reads the
// artifact form of the operand, before the partition binds it.
func artifactTrueEdgeLengthRelation(role equation.OperandRole, encoding []byte) (bool, error) {
	if !role.InFamily(equation.RoleFamilyDifference) {
		return false, nil
	}
	wire, present, err := front.DecodeBranchDiffWire(encoding)
	if err != nil {
		return false, err
	}
	if !present {
		return false, fmt.Errorf("engine: difference role %q has no difference wire", role)
	}
	return wire.Edge && (wire.LoIsLen || wire.HiIsLen || wire.Hi2IsLen), nil
}

// relationVariable names the solver variable for one operand of a normalized
// relation. The numeric IR treats every variable as an opaque key, so the
// value/length distinction has to survive in the key itself.
func relationVariable(pathKey string, isLength bool) pathdom.PathKey {
	if isLength {
		return pathdom.PathKey("len/" + pathKey)
	}
	const valueAxis = "value"
	return pathdom.PathKey(valueAxis + "/" + pathKey)
}

// relationAssertions lowers the bound evidence a branch proves on its true edge
// into the numeric constraint IR the solver portfolio consumes: a normalized
// floor becomes GeConst, an index-in-range predicate becomes value <= len, and
// each difference descriptor becomes its Le or bounded-sum form.
func relationAssertions(predicates []branchPredicateWire, differences []branchDiffWire) []numeric.NumericConstraint {
	asserted := make([]numeric.NumericConstraint, 0, len(predicates)+len(differences))
	for _, predicate := range predicates {
		if predicate.Negated || predicate.Path == "" {
			continue
		}
		switch predicate.Kind {
		case "num-ge":
			asserted = append(asserted, numeric.GeConst{X: relationVariable(predicate.Path, false), C: predicate.NumFloor})
		case "index-in-range":
			if predicate.OtherPath == "" {
				continue
			}
			asserted = append(asserted, numeric.Le{
				X: relationVariable(predicate.Path, false),
				Y: relationVariable(predicate.OtherPath, true),
				C: 0,
			})
		}
	}
	for _, difference := range differences {
		low := relationVariable(difference.LoPath, difference.LoIsLen)
		high := relationVariable(difference.HiPath, difference.HiIsLen)
		if !difference.HasHi2 && difference.CoHi == 1 {
			asserted = append(asserted, numeric.Le{X: high, Y: low, C: difference.C})
			continue
		}
		second := pathdom.PathKey("")
		coefficient := int64(0)
		if difference.HasHi2 {
			second = relationVariable(difference.Hi2Path, difference.Hi2IsLen)
			coefficient = difference.CoHi2
		}
		asserted = append(asserted, numeric.NewScaledLe(difference.CoHi, high, coefficient, second, low, difference.C))
	}
	return asserted
}

// relationContainers lists the arrays whose length the branch's relations bound
// something against. A container that never appears as a length operand cannot
// be the subject of an in-range proof, so the candidate set is complete.
func relationContainers(predicates []branchPredicateWire, differences []branchDiffWire) []string {
	seen := make(map[string]bool)
	add := func(name string, isLength bool) {
		if isLength && name != "" {
			seen[name] = true
		}
	}
	for _, predicate := range predicates {
		if predicate.Kind == "index-in-range" && !predicate.Negated {
			add(predicate.OtherPath, true)
		}
	}
	for _, difference := range differences {
		add(difference.HiPath, difference.HiIsLen)
		add(difference.Hi2Path, difference.Hi2IsLen)
		add(difference.LoPath, difference.LoIsLen)
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// relationalIndexPair is one in-range conclusion the solver derived from the
// branch's relations: index is proven no greater than the length of container.
type relationalIndexPair struct{ index, container string }

// relationalIndexUpperBounds asks the wired solver portfolio which index/array
// pairs the branch's own relations put in range. Only an index that already
// carries a positive floor is a candidate: the upper bound alone never proves
// presence, and asking for it elsewhere would only cost solver time.
//
// The portfolio is the single constraint path. Difference logic answers the
// transitive and length-equality goals; the exact linear backend answers the
// bounded-sum residue difference logic cannot express.
func relationalIndexUpperBounds(predicates []branchPredicateWire, differences []branchDiffWire, indexes []string) []relationalIndexPair {
	if len(differences) == 0 || len(indexes) == 0 {
		return nil
	}
	containers := relationContainers(predicates, differences)
	if len(containers) == 0 {
		return nil
	}
	asserted := relationAssertions(predicates, differences)
	if len(asserted) == 0 {
		return nil
	}
	portfolio := solver.DefaultPortfolio()
	var proven []relationalIndexPair
	for _, index := range indexes {
		for _, container := range containers {
			goal := numeric.Le{X: relationVariable(index, false), Y: relationVariable(container, true), C: 0}
			if portfolio.Entails(asserted, goal) == decision.Valid {
				proven = append(proven, relationalIndexPair{index: index, container: container})
			}
		}
	}
	return proven
}

// branchNumericTruth asks the numeric authorities whether a branch's own
// evidence already decides which arm runs. It answers from proven facts only:
// the residue windows the arithmetic lane published for the paths under test,
// the bound descriptors the front normalized onto this branch's edges, and the
// exact affine backend that refutes their conjunction. A path those authorities
// say nothing about leaves the branch undecided, so an abstract value never
// decides an arm.
//
// The two verdicts are separate proofs. The true edge is refuted when the
// conjunction it asserts has no model. The false edge is refuted only for a
// branch whose selector is a single normalized check, because only there is the
// false edge exactly that check's negation.
func branchNumericTruth(operation equation.BoundEquation, partition equation.Partition) (truth, decided bool, err error) {
	predicates, err := trueEdgeNumericPredicates(operation)
	if err != nil {
		return false, false, err
	}
	differences, err := trueEdgeBranchDifferences(operation)
	if err != nil {
		return false, false, err
	}
	if len(predicates) == 0 && len(differences) == 0 {
		return false, false, nil
	}
	if !numericEdgeSatisfiable(predicates, differences, partition) {
		return false, true, nil
	}
	if predicate, single, selectorErr := negatableBranchSelector(operation); selectorErr != nil {
		return false, false, selectorErr
	} else if single {
		predicate.Negated = !predicate.Negated
		if !numericEdgeSatisfiable([]branchPredicateWire{predicate}, nil, partition) {
			return true, true, nil
		}
	}
	return false, false, nil
}

// trueEdgeNumericPredicates collects the normalized checks the branch's true
// edge asserts. Only the selector itself and the implied checks state that. A
// sufficient check is the converse relation - taking the edge follows from it,
// not it from the edge - so a disjunction publishes every arm's sufficient
// check on the same edge and their conjunction describes no execution at all.
// A refutation reads necessary conditions only, so those roles, and any role
// this vocabulary does not name, stay out of the assertion set.
func trueEdgeNumericPredicates(operation equation.BoundEquation) ([]branchPredicateWire, error) {
	predicates := make([]branchPredicateWire, 0, len(operation.Operands))
	for _, operand := range operation.Operands {
		if operand.Role != equation.RolePredicate && !operand.Role.InFamily(equation.RoleFamilyImplied) {
			continue
		}
		predicate, trueEdge, recognized, err := branchEvidencePredicate(operand)
		if err != nil {
			return nil, err
		}
		if !recognized || !trueEdge || predicate.Path == "" {
			continue
		}
		predicates = append(predicates, predicate)
	}
	return predicates, nil
}

// negatableBranchSelector returns the normalized check whose negation is
// exactly the branch's false edge. A branch that also carries a scalar
// condition is selected by that condition, and a compound condition's false
// edge refutes no individual conjunct, so neither form yields one.
func negatableBranchSelector(operation equation.BoundEquation) (branchPredicateWire, bool, error) {
	for _, operand := range operation.Operands {
		if operand.Role == "condition" {
			return branchPredicateWire{}, false, nil
		}
	}
	predicate, found, err := soleBranchPredicate(operation)
	if err != nil {
		return branchPredicateWire{}, false, err
	}
	if !found || predicate.Path == "" {
		return branchPredicateWire{}, false, nil
	}
	return predicate, true, nil
}

// numericEdgeSatisfiable reports that the numeric authorities admit a model for
// an edge that asserts these predicates and difference relations. It is the
// refutation seam: a false answer is a proof that the edge is never taken, and
// every path the authorities carry no fact about simply contributes no
// constraint, so an unconstrained edge always remains satisfiable.
func numericEdgeSatisfiable(predicates []branchPredicateWire, differences []branchDiffWire, partition equation.Partition) bool {
	for _, predicate := range predicates {
		if predicate.Kind != "mod-residue" {
			continue
		}
		if branchResidueClassRefutes(predicate, partition) {
			return false
		}
		window, published := publishedResidueWindow([]byte("path/"+predicate.Path), partition)
		if !published {
			continue
		}
		holds, class := residueClassWindowVerdict(window, predicate.Modulus, predicate.Residue)
		if class && holds == predicate.Negated {
			return false
		}
	}
	asserted := relationAssertions(nil, differences)
	paths := make(map[string]bool, len(predicates))
	for _, predicate := range predicates {
		asserted = append(asserted, numericPredicateConstraints(predicate)...)
		paths[predicate.Path] = true
		if predicate.OtherPath != "" {
			paths[predicate.OtherPath] = true
		}
	}
	names := make([]string, 0, len(paths))
	for name := range paths {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		asserted = append(asserted, residueWindowConstraints(name, partition)...)
	}
	// One constraint over unbounded variables always has a model, so the exact
	// backend is asked only where two of them can conflict.
	if len(asserted) < 2 {
		return true
	}
	return solver.AffineSatisfiable(asserted)
}

// branchResidueClassRefutes consumes only guarded class rows visible in this
// partition. One exact class modulo m is disjoint from every other class
// modulo m; the current predicate is therefore refuted exactly when its truth
// value on that established class is false. A later epoch names a different
// subject value and invalidates the row.
func branchResidueClassRefutes(predicate branchPredicateWire, partition equation.Partition) bool {
	term := []byte("path/" + predicate.Path)
	values := partition.FamilyValues(factkey.BuildKey(
		factkey.BranchResidueClass,
		[]factkey.Part{factkey.EncodedTermPart(term)},
		"",
	))
	for {
		fact, ok := values.Next()
		if !ok {
			return false
		}
		if epoch, versioned := currentEpoch(term, partition); versioned && epoch > fact.Occurrence {
			continue
		}
		var established branchResidueClassWire
		if front.DecodeRequiredWireJSON(fact.Payload, &established) != nil || established.Modulus <= 0 || established.Negated {
			continue
		}
		if established.Modulus != predicate.Modulus {
			continue
		}
		holds := established.Residue == predicate.Residue
		if predicate.Negated {
			holds = !holds
		}
		if !holds {
			return true
		}
	}
}

// numericPredicateConstraints lowers one normalized check into the affine
// constraints it states while it holds. The lowering follows the predicate
// evaluator exactly: a floor predicate holds when `value >= NumFloor` differs
// from Negated, so its negated form states `value < NumFloor` and is carried as
// `value <= NumFloor`. That is a relaxation, never a tightening, so a
// refutation over the relaxed set refutes the exact one. A check outside the
// affine fragment - a disequality, a residue class - states nothing here and is
// answered by its own authority.
func numericPredicateConstraints(predicate branchPredicateWire) []numeric.NumericConstraint {
	value := relationVariable(predicate.Path, false)
	switch predicate.Kind {
	case "num-ge":
		if predicate.Negated {
			return []numeric.NumericConstraint{numeric.LeConst{X: value, C: predicate.NumFloor}}
		}
		return []numeric.NumericConstraint{numeric.GeConst{X: value, C: predicate.NumFloor}}
	case "num-le":
		if !predicate.HasNumCeil {
			return nil
		}
		if predicate.Negated {
			return []numeric.NumericConstraint{numeric.GeConst{X: value, C: predicate.NumCeil}}
		}
		return []numeric.NumericConstraint{numeric.LeConst{X: value, C: predicate.NumCeil}}
	case "len-ge":
		length := relationVariable(predicate.Path, true)
		if predicate.Negated {
			return []numeric.NumericConstraint{numeric.LeConst{X: length, C: predicate.LenFloor}}
		}
		return []numeric.NumericConstraint{numeric.GeConst{X: length, C: predicate.LenFloor}}
	case "index-in-range":
		if predicate.Negated || predicate.OtherPath == "" {
			return nil
		}
		return []numeric.NumericConstraint{numeric.Le{X: value, Y: relationVariable(predicate.OtherPath, true), C: 0}}
	case "literal-equal":
		if predicate.Negated {
			return nil
		}
		constant, integral := scalarIntegerConstant([]byte(predicate.Literal))
		if !integral {
			return nil
		}
		return []numeric.NumericConstraint{
			numeric.GeConst{X: value, C: constant},
			numeric.LeConst{X: value, C: constant},
		}
	default:
		return nil
	}
}

// residueWindowConstraints states the interval a path's published residue
// window pins it to. A window measured against a container's length is carried
// as the relation it is - the length is a solver variable like any other - so
// no numeric relation between the two is invented here.
func residueWindowConstraints(name string, partition equation.Partition) []numeric.NumericConstraint {
	window, published := publishedResidueWindow([]byte("path/"+name), partition)
	if !published {
		return nil
	}
	value := relationVariable(name, false)
	constraints := []numeric.NumericConstraint{numeric.GeConst{X: value, C: window.Low}}
	if window.Container == "" {
		return append(constraints, numeric.LeConst{X: value, C: window.High})
	}
	container, rooted := strings.CutPrefix(window.Container, "path/")
	if !rooted || container == "" {
		return constraints
	}
	return append(constraints, numeric.Le{X: value, Y: relationVariable(container, true), C: window.High})
}

// residueClassWindowVerdict intersects a residue class with the window its path
// occupies. Both are closed integer facts, so the intersection is the whole
// answer: a class with no member inside the window refutes the check, and a
// window every member of which lies in the class proves it. A window measured
// against a container's length has no constant bounds to intersect and decides
// nothing.
//
// The residue is consumed exactly as the normalized check carries it. The front
// admits only a positive modulus and a residue already inside [0, modulus-1],
// so a residue outside that range is not this check's class and is answered by
// no verdict rather than reduced into a class the source never named.
func residueClassWindowVerdict(window residueWindow, modulus, residue int64) (holds, decided bool) {
	if window.Container != "" || modulus <= 0 || residue < 0 || residue >= modulus || window.Low > window.High {
		return false, false
	}
	largest, representable := residueClassCeiling(window.High, modulus, residue)
	if !representable {
		return false, false
	}
	if largest < window.Low {
		return false, true
	}
	// Consecutive integers occupy different classes of any modulus above one,
	// so only a window pinned to a single value lies wholly inside a class.
	if modulus == 1 {
		return true, true
	}
	if window.Low == window.High {
		return largest == window.Low, true
	}
	return false, false
}

// encodeAffineTerm renders one affine identity. The offset leads so the base
// term, which contains the separator itself, stays the undivided tail.
func encodeAffineTerm(base []byte, offset int64) []byte {
	return []byte(affineTermEncoding + strconv.FormatInt(offset, 10) + "/" + string(base))
}

// decodeAffineTerm reads an affine identity back into its base term and offset.
func decodeAffineTerm(encoded []byte) ([]byte, int64, bool) {
	rest, found := strings.CutPrefix(string(encoded), affineTermEncoding)
	if !found {
		return nil, 0, false
	}
	digits, base, split := strings.Cut(rest, "/")
	offset, err := strconv.ParseInt(digits, 10, 64)
	if !split || err != nil || base == "" {
		return nil, 0, false
	}
	return []byte(base), offset, true
}

// affineExpressionTerm reads an integer add or subtract of a bound path and a
// constant as the affine identity base + offset. The carrier decides the
// answer: a shifted term lands on a slot exactly when its base does, so the
// base must be a number and the offset an exact integer. A fractional constant,
// a non-numeric base, or a subtraction that negates the base yields no
// identity, and the index term then carries no in-range proof at all.
func affineExpressionTerm(operator wir.Operator, leftTerm, leftValue, rightTerm, rightValue []byte) ([]byte, int64, bool) {
	switch operator {
	case wir.BinAdd:
		if offset, constant := integerConstantOperand(rightValue); constant && affineCarrier(leftTerm, leftValue) {
			return leftTerm, offset, true
		}
		if offset, constant := integerConstantOperand(leftValue); constant && affineCarrier(rightTerm, rightValue) {
			return rightTerm, offset, true
		}
	case wir.BinSub:
		offset, constant := integerConstantOperand(rightValue)
		if constant && offset != math.MinInt64 && affineCarrier(leftTerm, leftValue) {
			return leftTerm, -offset, true
		}
	}
	return nil, 0, false
}

// affineCarrier accepts a base operand that is a bound path holding a number.
// The path spelling is required because the relations an in-range proof is
// discharged against name paths, not temporaries.
func affineCarrier(term, value []byte) bool {
	if !strings.HasPrefix(string(term), "path/") {
		return false
	}
	target, ok := shapefact.DecodeTarget(value)
	if !ok || target == nil {
		return false
	}
	switch unwrap.Alias(subst.ExpandInstantiated(target)).Kind() {
	case kind.Integer, kind.Number:
		return true
	}
	return false
}

// integerConstantOperand accepts only a constant that is exactly an integer.
// A float operand leaves the shifted term off the slot lattice, so it produces
// no offset rather than a rounded one.
func integerConstantOperand(value []byte) (int64, bool) {
	if scalar, found := shapefact.DecodeScalarKind(value, shapefact.ScalarNumber); found {
		offset, err := strconv.ParseInt(string(scalar.Data), 10, 64)
		return offset, err == nil
	}
	if target, ok := shapefact.DecodeTarget(value); ok {
		if literal, isLiteral := unwrap.Alias(target).(*typ.Literal); isLiteral && literal != nil && literal.Base == kind.Integer {
			offset, err := strconv.ParseInt(literal.String(), 10, 64)
			return offset, err == nil
		}
	}
	return 0, false
}

// indexRelationFacts republishes the relations a branch proves on its true edge
// as guarded facts, in the exact closed encoding the front produced. An index
// term the branch never saw - a shifted term computed inside the arm - is
// discharged against these same relations later, so the branch keeps them
// available instead of collapsing them into the boolean pairs it can name now.
func indexRelationFacts(predicates []branchPredicateWire, differences []branchDiffWire, operationName string, guards []equation.Guard) []equation.Fact {
	facts := make([]equation.Fact, 0, len(predicates)+len(differences))
	for _, predicate := range predicates {
		if predicate.Negated || predicate.Path == "" {
			continue
		}
		if predicate.Kind != "num-ge" && predicate.Kind != "index-in-range" {
			continue
		}
		encoded, err := front.EncodeBranchPredicateWire(predicate)
		if err != nil {
			continue
		}
		facts = append(facts, equation.Fact{
			Key: factkey.BuildKey(factkey.HeapIndexRelation, []factkey.Part{
				factkey.OpaquePart(fmt.Sprintf("p-%08d", len(facts))),
			}, operationName).String(),
			Value:  encoded,
			Guards: guards,
		})
	}
	for _, difference := range differences {
		encoded, err := front.EncodeBranchDiffWire(difference)
		if err != nil {
			continue
		}
		facts = append(facts, equation.Fact{
			Key: factkey.BuildKey(factkey.HeapIndexRelation, []factkey.Part{
				factkey.OpaquePart(fmt.Sprintf("d-%08d", len(facts))),
			}, operationName).String(),
			Value:  encoded,
			Guards: guards,
		})
	}
	return facts
}

// relationPaths lists every path a republished relation constrains. A write to
// any of them replaces a value the relation was stated about, so the relation
// stops holding at that point.
func relationPaths(predicate branchPredicateWire, difference branchDiffWire, isPredicate bool) []string {
	if isPredicate {
		return []string{predicate.Path, predicate.OtherPath}
	}
	return []string{difference.HiPath, difference.Hi2Path, difference.LoPath}
}

// provenNumericFloor returns the largest constant lower bound the relations
// state directly on a path. The floor is a normalized constant, not a goal, so
// it is read back rather than re-derived.
func provenNumericFloor(predicates []branchPredicateWire, path string) (int64, bool) {
	floor, found := int64(0), false
	for _, predicate := range predicates {
		if predicate.Kind != "num-ge" || predicate.Negated || predicate.Path != path {
			continue
		}
		if !found || predicate.NumFloor > floor {
			floor, found = predicate.NumFloor, true
		}
	}
	return floor, found
}

// affineIndexInRange decides presence of container[base+offset] from the
// relations the guard proved about base. The upper side is a portfolio goal at
// the shifted constant: base + offset <= len(container) is the difference
// base - len(container) <= -offset. The lower side is the base's own constant
// floor shifted by the same offset, which is why i >= 1 admits i + 1 but never
// i - 1.
func affineIndexInRange(predicates []branchPredicateWire, differences []branchDiffWire, base, container string, offset int64) bool {
	if base == "" || container == "" || offset == math.MinInt64 {
		return false
	}
	floor, hasFloor := provenNumericFloor(predicates, base)
	shifted, representable := addChecked(floor, offset)
	if !hasFloor || !representable || shifted < 1 {
		return false
	}
	asserted := relationAssertions(predicates, differences)
	if len(asserted) == 0 {
		return false
	}
	goal := numeric.Le{X: relationVariable(base, false), Y: relationVariable(container, true), C: -offset}
	return solver.DefaultPortfolio().Entails(asserted, goal) == decision.Valid
}

// addChecked reports the sum only when it is representable, so a source-level
// offset can never wrap a floor into a proof.
func addChecked(a, b int64) (int64, bool) {
	sum := a + b
	if (b > 0 && sum < a) || (b < 0 && sum > a) {
		return 0, false
	}
	return sum, true
}

// provenFloorPaths lists the index paths that currently carry a positive floor,
// read back from the encoded lower-bound relations the branch lane maintains.
func provenFloorPaths(lower map[string][]byte) []string {
	out := make([]string, 0, len(lower))
	for _, index := range lower {
		if name, found := strings.CutPrefix(string(index), "path/"); found && name != "" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
