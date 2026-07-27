package front

import (
	"fmt"
	"strconv"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

const (
	branchArmEncoding     = "front/branch-arm/v1"
	densityRelationPrefix = "front/density-relation/v1/"
)

// shortCircuitBypass names the value-position short-circuit whose guard a
// branch is. result carries value on the bypass edge, which is the edge that
// does not evaluate the right operand: false for and, true for or. On that edge
// Lua yields the left operand, so result holds exactly the projection of value
// which the edge admits.
type shortCircuitBypass struct {
	result wir.Operand
	value  wir.Operand
	edge   bool
}

// shortCircuitBypassGuards indexes the value-position short-circuit regions by
// the guard point that decides them. A point owning two regions names no single
// result, so it is withheld rather than resolved by insertion order.
func shortCircuitBypassGuards(body *wir.Body) map[cfg.Point]shortCircuitBypass {
	guards := make(map[cfg.Point]shortCircuitBypass)
	ambiguous := make(map[cfg.Point]bool)
	body.ForEachStructuralExpressionRegion(func(owner wir.StructuralExpressionOwner, region wir.StructuralExpressionRegion) bool {
		if !owner.HasTemp || region.BypassValue.Kind == wir.OperandNone {
			return true
		}
		if _, seen := guards[region.Guard]; seen {
			ambiguous[region.Guard] = true
			return true
		}
		guards[region.Guard] = shortCircuitBypass{
			result: wir.Operand{Kind: wir.OperandTemp, Ref: owner.Temp},
			value:  region.BypassValue,
			edge:   !region.RHSOnTrue,
		}
		return true
	})
	for point := range ambiguous {
		delete(guards, point)
	}
	if len(guards) == 0 {
		return nil
	}
	return guards
}

// branchOperands lowers every branch-owned WIR descriptor.  In particular,
// compound-condition evidence and difference constraints are retained instead
// of being silently dropped when this front has no consumer for them yet.
func branchOperands(body *wir.Body, instruction wir.Instruction, bypass shortCircuitBypass, isShortCircuit bool) ([]equation.Operand, error) {
	check := body.Check(instruction.Check)
	operands := make([]equation.Operand, 0, 1+int(instruction.ImpliedChecks.Len)+int(instruction.SufficientChecks.Len)+int(instruction.DiffConstraints.Len))
	if instruction.A.Kind != wir.OperandNone {
		condition, err := scalarTerm(body, instruction.A)
		if err != nil {
			return nil, fmt.Errorf("condition: %w", err)
		}
		operands = append(operands, equation.Operand{Role: "condition", Term: condition})
	}
	if check.Kind != wir.CheckNone {
		predicate, err := branchPredicateTerm(check)
		if err != nil {
			return nil, err
		}
		operands = append(operands, equation.Operand{Role: "predicate", Term: predicate})
		if display := check.Path.String(); display != "" {
			operands = append(operands, equation.Operand{Role: "predicate-display", Term: equation.ClosedTerm([]byte(display))})
		}
	}
	if instruction.A.Kind == wir.OperandNone && check.Kind == wir.CheckNone {
		return nil, fmt.Errorf("branch has neither a scalar condition nor a normalized predicate")
	}
	for index, implied := range body.ImpliedChecks(instruction.ImpliedChecks) {
		term, err := branchEvidenceTerm(implied)
		if err != nil {
			return nil, fmt.Errorf("implied check %d: %w", index, err)
		}
		operands = append(operands, equation.Operand{Role: equation.IndexedRole(equation.RoleFamilyImplied, index), Term: term})
	}
	for index, sufficient := range body.SufficientChecks(instruction.SufficientChecks) {
		term, err := branchEvidenceTerm(sufficient)
		if err != nil {
			return nil, fmt.Errorf("sufficient check %d: %w", index, err)
		}
		operands = append(operands, equation.Operand{Role: equation.IndexedRole(equation.RoleFamilySufficient, index), Term: term})
	}
	for _, edge := range []struct {
		name string
		arms wir.ArmRange
	}{{"true", instruction.SufficientCheckArmsTrue}, {"false", instruction.SufficientCheckArmsFalse}} {
		for armIndex, arm := range body.SufficientCheckArms(edge.arms) {
			armRole := equation.SuffixedRole(equation.RoleFamilySufficientArm, fmt.Sprintf("%s-%08d", edge.name, armIndex))
			operands = append(operands, equation.Operand{Role: armRole, Term: equation.ClosedTerm([]byte(branchArmEncoding))})
			for checkIndex, sufficient := range arm {
				term, err := branchEvidenceTerm(sufficient)
				if err != nil {
					return nil, fmt.Errorf("sufficient %s arm %d check %d: %w", edge.name, armIndex, checkIndex, err)
				}
				operands = append(operands, equation.Operand{Role: equation.SuffixedRole(equation.RoleFamilySufficientArm, fmt.Sprintf("%s-%08d-check-%08d", edge.name, armIndex, checkIndex)), Term: term})
			}
		}
	}
	for index, diff := range body.BranchDiffConstraints(instruction.DiffConstraints) {
		term, err := branchDiffTerm(diff)
		if err != nil {
			return nil, fmt.Errorf("difference constraint %d: %w", index, err)
		}
		operands = append(operands, equation.Operand{Role: equation.IndexedRole(equation.RoleFamilyDifference, index), Term: term})
	}
	if isShortCircuit {
		result, err := scalarTerm(body, bypass.result)
		if err != nil {
			return nil, fmt.Errorf("short-circuit result: %w", err)
		}
		value, err := scalarTerm(body, bypass.value)
		if err != nil {
			return nil, fmt.Errorf("short-circuit operand: %w", err)
		}
		operands = append(operands,
			equation.Operand{Role: "short-circuit-result", Term: result},
			equation.Operand{Role: "short-circuit-operand", Term: value},
			equation.Operand{Role: "short-circuit-bypass", Term: equation.ClosedTerm([]byte(shapefact.BooleanValueString(bypass.edge)))},
		)
	}
	return operands, nil
}

// branchChainOperands freezes authored if/elseif topology into the branch
// equations that own its checks. Engine consumers group these publications
// after canonicalization; they never need WIR chain or branch-check reads.
func branchChainOperands(body *wir.Body) (map[cfg.Point]equation.Term, error) {
	out := make(map[cfg.Point]equation.Term)
	var firstErr error
	body.ForEachIfChainDescriptor(func(chain wir.IfChainDescriptor) bool {
		// Both consumers require an authored if/elseif chain. A single if has
		// no coverage relation to publish and stays on its ordinary predicate
		// operand only.
		if len(chain.Branches) < 2 {
			return true
		}
		published := make(map[cfg.Point]equation.Term, len(chain.Branches))
		complete := true
		for position, branch := range chain.Branches {
			checks := body.BranchChecks(branch.Point)
			wiredChecks := make([]BranchChainCheckWire, 0, len(checks))
			for _, check := range checks {
				if _, supported := branchCheckKind(check.Kind); !supported || check.Kind == wir.CheckNone {
					complete = false
					break
				}
				predicate, err := branchPredicateWireForCheck(check)
				if err != nil {
					firstErr = err
					return false
				}
				wired := BranchChainCheckWire{
					Predicate: predicate,
					Path:      branchChainPathWire(check.Path),
					OtherPath: branchChainPathWire(check.OtherPath),
				}
				if check.Kind == wir.CheckLiteralEqual || check.Kind == wir.CheckLiteralNot {
					target, encoded := shapefact.EncodeTarget(check.Literal)
					if !encoded {
						firstErr = fmt.Errorf("branch chain literal has no canonical target")
						return false
					}
					wired.LiteralTarget = string(target)
				}
				wiredChecks = append(wiredChecks, wired)
			}
			if !complete {
				break
			}
			encoded, err := EncodeBranchChainWire(BranchChainWire{
				ID: chain.ID, Position: uint32(position), Count: uint32(len(chain.Branches)),
				HasElse: chain.HasElse, HeadSpan: chain.HeadSpan, Checks: wiredChecks,
			})
			if err != nil {
				firstErr = err
				return false
			}
			if _, duplicate := out[branch.Point]; duplicate {
				firstErr = fmt.Errorf("branch point %d belongs to multiple authored chains", branch.Point)
				return false
			}
			published[branch.Point] = equation.ClosedTerm(encoded)
		}
		if complete {
			for point, term := range published {
				out[point] = term
			}
		}
		return true
	})
	if firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

func branchChainPathWire(value path.Path) BranchChainPathWire {
	if value.IsEmpty() {
		return BranchChainPathWire{}
	}
	wire := BranchChainPathWire{Key: string(value.Key()), Display: value.String()}
	if len(value.Segments) == 0 {
		return wire
	}
	last := value.Segments[len(value.Segments)-1]
	if last.Kind != segment.SegmentField {
		return wire
	}
	parent := value
	parent.Segments = append([]segment.Segment(nil), value.Segments[:len(value.Segments)-1]...)
	wire.FinalField = last.Name
	wire.ParentKey = string(parent.Key())
	wire.ParentDisplay = parent.String()
	return wire
}

func branchEvidenceTerm(check wir.ImpliedCheck) (equation.Term, error) {
	wire, err := branchPredicateWireForCheck(check.Check)
	if err != nil {
		return equation.Term{}, err
	}
	encoded, err := EncodeBranchEvidenceWire(wire, check.Edge, check.Polarity)
	if err != nil {
		return equation.Term{}, err
	}
	return equation.ClosedTerm(encoded), nil
}

func branchPredicateTerm(check wir.Check) (equation.Term, error) {
	wire, err := branchPredicateWireForCheck(check)
	if err != nil {
		return equation.Term{}, err
	}
	encoded, err := EncodeBranchPredicateWire(wire)
	if err != nil {
		return equation.Term{}, err
	}
	return equation.ClosedTerm(encoded), nil
}

func branchPredicateWireForCheck(check wir.Check) (BranchPredicateWire, error) {
	kind, ok := branchCheckKind(check.Kind)
	if !ok || check.Kind == wir.CheckNone {
		return BranchPredicateWire{}, fmt.Errorf("unsupported normalized check kind %d", check.Kind)
	}
	wire := BranchPredicateWire{
		Kind: kind, TypeName: check.TypeName, LenFloor: check.LenFloor,
		NumFloor: check.NumFloor, NumCeil: check.NumCeil, HasNumCeil: check.HasNumCeil,
		NumCeilNegated: check.NumCeilNegated, Modulus: check.Modulus, Residue: check.Residue,
		Negated: check.Negated,
	}
	var err error
	if requiresBranchPath(check.Kind) {
		wire.Path, err = checkPathKey(check.Path)
		if err != nil {
			return BranchPredicateWire{}, err
		}
	}
	if requiresOtherBranchPath(check.Kind, check.TypeName) {
		wire.OtherPath, err = checkPathKey(check.OtherPath)
		if err != nil {
			return BranchPredicateWire{}, fmt.Errorf("other path: %w", err)
		}
	}
	if check.Kind == wir.CheckLiteralEqual || check.Kind == wir.CheckLiteralNot {
		wire.Literal, err = literalScalarEncoding(check.Literal)
		if err != nil {
			return BranchPredicateWire{}, err
		}
	}
	if (check.Kind == wir.CheckTypeEqual || check.Kind == wir.CheckTypeNot) && check.TypeName == "" && wire.OtherPath == "" {
		return BranchPredicateWire{}, fmt.Errorf("type predicate has neither a type name nor an other path")
	}
	return wire, nil
}

func branchDiffTerm(diff wir.BranchDiffConstraint) (equation.Term, error) {
	hi, err := checkPathKey(diff.HiPath)
	if err != nil {
		return equation.Term{}, fmt.Errorf("high path: %w", err)
	}
	lo, err := checkPathKey(diff.LoPath)
	if err != nil {
		return equation.Term{}, fmt.Errorf("low path: %w", err)
	}
	wire := BranchDiffWire{CoHi: diff.CoHi, HiPath: hi, HiIsLen: diff.HiIsLen, CoHi2: diff.CoHi2, Hi2IsLen: diff.Hi2IsLen, HasHi2: diff.HasHi2, LoPath: lo, LoIsLen: diff.LoIsLen, C: diff.C, Edge: diff.Edge}
	if diff.HasHi2 {
		wire.Hi2Path, err = checkPathKey(diff.Hi2Path)
		if err != nil {
			return equation.Term{}, fmt.Errorf("second high path: %w", err)
		}
	}
	encoded, err := EncodeBranchDiffWire(wire)
	if err != nil {
		return equation.Term{}, err
	}
	return equation.ClosedTerm(encoded), nil
}

func checkPathKey(checkPath path.Path) (string, error) {
	if checkPath.IsEmpty() || checkPath.Key() == "" {
		return "", fmt.Errorf("empty predicate path")
	}
	return string(checkPath.Key()), nil
}

func literalScalarEncoding(value typ.Type) (string, error) {
	literal, ok := value.(*typ.Literal)
	if !ok || literal == nil {
		return "", fmt.Errorf("literal predicate has no scalar literal")
	}
	switch value := literal.Value.(type) {
	case bool:
		return shapefact.BooleanValueString(value), nil
	case int64:
		return shapefact.ScalarTextValueString(shapefact.ScalarNumber, strconv.FormatInt(value, 10)), nil
	case float64:
		return shapefact.ScalarTextValueString(shapefact.ScalarNumber, strconv.FormatFloat(value, 'g', -1, 64)), nil
	case string:
		return shapefact.ScalarTextValueString(shapefact.ScalarString, strconv.Quote(value)), nil
	default:
		return "", fmt.Errorf("literal predicate has unsupported scalar type %T", value)
	}
}

func requiresBranchPath(kind wir.CheckKind) bool {
	return kind != wir.CheckNone
}

func requiresOtherBranchPath(kind wir.CheckKind, typeName string) bool {
	switch kind {
	case wir.CheckPathEqual, wir.CheckPathNot, wir.CheckIndexInRange:
		return true
	case wir.CheckTypeEqual, wir.CheckTypeNot:
		return typeName == ""
	default:
		return false
	}
}

func branchCheckKind(kind wir.CheckKind) (string, bool) {
	switch kind {
	case wir.CheckTruthy:
		return "truthy", true
	case wir.CheckFalsy:
		return "falsy", true
	case wir.CheckNil:
		return "nil", true
	case wir.CheckNotNil:
		return "not-nil", true
	case wir.CheckTypeEqual:
		return "type-equal", true
	case wir.CheckTypeNot:
		return "type-not", true
	case wir.CheckLiteralEqual:
		return "literal-equal", true
	case wir.CheckLiteralNot:
		return "literal-not", true
	case wir.CheckPathEqual:
		return "path-equal", true
	case wir.CheckPathNot:
		return "path-not", true
	case wir.CheckLenGe:
		return "len-ge", true
	case wir.CheckIndexInRange:
		return "index-in-range", true
	case wir.CheckNumGe:
		return "num-ge", true
	case wir.CheckNumLe:
		return "num-le", true
	case wir.CheckFrozenTable:
		return "frozen-table", true
	case wir.CheckModResidue:
		return "mod-residue", true
	default:
		return "", false
	}
}
