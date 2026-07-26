package front

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

const (
	branchPredicatePrefix = "front/branch-predicate/v1/"
	branchEvidencePrefix  = "front/branch-evidence/v1/"
	branchArmEncoding     = "front/branch-arm/v1"
	branchDiffPrefix      = "front/branch-diff/v1/"
	densityRelationPrefix = "front/density-relation/v1/"
)

// branchPredicateWire is the closed branch predicate vocabulary shared with
// the canonical branch kernel.  It contains resolved WIR identities only;
// neither an AST node nor a state callback can enter the equation artifact.
type branchPredicateWire struct {
	Kind           string `json:"kind"`
	Path           string `json:"path,omitempty"`
	OtherPath      string `json:"other_path,omitempty"`
	TypeName       string `json:"type_name,omitempty"`
	Literal        string `json:"literal,omitempty"`
	LenFloor       int64  `json:"len_floor,omitempty"`
	NumFloor       int64  `json:"num_floor,omitempty"`
	NumCeil        int64  `json:"num_ceil,omitempty"`
	HasNumCeil     bool   `json:"has_num_ceil,omitempty"`
	NumCeilNegated bool   `json:"num_ceil_negated,omitempty"`
	Modulus        int64  `json:"modulus,omitempty"`
	Residue        int64  `json:"residue,omitempty"`
	Negated        bool   `json:"negated,omitempty"`
	ProducerPoint  uint32 `json:"producer_point,omitempty"`
	HasProducer    bool   `json:"has_producer,omitempty"`
}

type branchDiffWire struct {
	CoHi     int64  `json:"co_hi"`
	HiPath   string `json:"hi_path"`
	HiIsLen  bool   `json:"hi_is_len,omitempty"`
	CoHi2    int64  `json:"co_hi2,omitempty"`
	Hi2Path  string `json:"hi2_path,omitempty"`
	Hi2IsLen bool   `json:"hi2_is_len,omitempty"`
	HasHi2   bool   `json:"has_hi2,omitempty"`
	LoPath   string `json:"lo_path"`
	LoIsLen  bool   `json:"lo_is_len,omitempty"`
	C        int64  `json:"c,omitempty"`
	Edge     bool   `json:"edge,omitempty"`
}

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
		operands = append(operands, equation.Operand{Role: fmt.Sprintf("implied-%08d", index), Term: term})
	}
	for index, sufficient := range body.SufficientChecks(instruction.SufficientChecks) {
		term, err := branchEvidenceTerm(sufficient)
		if err != nil {
			return nil, fmt.Errorf("sufficient check %d: %w", index, err)
		}
		operands = append(operands, equation.Operand{Role: fmt.Sprintf("sufficient-%08d", index), Term: term})
	}
	for _, edge := range []struct {
		name string
		arms wir.ArmRange
	}{{"true", instruction.SufficientCheckArmsTrue}, {"false", instruction.SufficientCheckArmsFalse}} {
		for armIndex, arm := range body.SufficientCheckArms(edge.arms) {
			armRole := fmt.Sprintf("sufficient-arm-%s-%08d", edge.name, armIndex)
			operands = append(operands, equation.Operand{Role: armRole, Term: equation.ClosedTerm([]byte(branchArmEncoding))})
			for checkIndex, sufficient := range arm {
				term, err := branchEvidenceTerm(sufficient)
				if err != nil {
					return nil, fmt.Errorf("sufficient %s arm %d check %d: %w", edge.name, armIndex, checkIndex, err)
				}
				operands = append(operands, equation.Operand{Role: fmt.Sprintf("%s-check-%08d", armRole, checkIndex), Term: term})
			}
		}
	}
	for index, diff := range body.BranchDiffConstraints(instruction.DiffConstraints) {
		term, err := branchDiffTerm(diff)
		if err != nil {
			return nil, fmt.Errorf("difference constraint %d: %w", index, err)
		}
		operands = append(operands, equation.Operand{Role: fmt.Sprintf("difference-%08d", index), Term: term})
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

func branchEvidenceTerm(check wir.ImpliedCheck) (equation.Term, error) {
	predicate, err := branchPredicateTerm(check.Check)
	if err != nil {
		return equation.Term{}, err
	}
	prefix := fmt.Sprintf("%s%t/%t/", branchEvidencePrefix, check.Edge, check.Polarity)
	return equation.ClosedTerm(append([]byte(prefix), predicate.Encoding...)), nil
}

func branchPredicateTerm(check wir.Check) (equation.Term, error) {
	kind, ok := branchCheckKind(check.Kind)
	if !ok || check.Kind == wir.CheckNone {
		return equation.Term{}, fmt.Errorf("unsupported normalized check kind %d", check.Kind)
	}
	wire := branchPredicateWire{
		Kind: kind, TypeName: check.TypeName, LenFloor: check.LenFloor,
		NumFloor: check.NumFloor, NumCeil: check.NumCeil, HasNumCeil: check.HasNumCeil,
		NumCeilNegated: check.NumCeilNegated, Modulus: check.Modulus, Residue: check.Residue,
		Negated:       check.Negated,
		ProducerPoint: uint32(check.ProducerPoint), HasProducer: check.HasProducerPoint,
	}
	var err error
	if requiresBranchPath(check.Kind) {
		wire.Path, err = checkPathKey(check.Path)
		if err != nil {
			return equation.Term{}, err
		}
	}
	if requiresOtherBranchPath(check.Kind, check.TypeName) {
		wire.OtherPath, err = checkPathKey(check.OtherPath)
		if err != nil {
			return equation.Term{}, fmt.Errorf("other path: %w", err)
		}
	}
	if check.Kind == wir.CheckLiteralEqual || check.Kind == wir.CheckLiteralNot {
		wire.Literal, err = literalScalarEncoding(check.Literal)
		if err != nil {
			return equation.Term{}, err
		}
	}
	if (check.Kind == wir.CheckTypeEqual || check.Kind == wir.CheckTypeNot) && check.TypeName == "" && wire.OtherPath == "" {
		return equation.Term{}, fmt.Errorf("type predicate has neither a type name nor an other path")
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return equation.Term{}, fmt.Errorf("encode predicate: %w", err)
	}
	return equation.ClosedTerm(append([]byte(branchPredicatePrefix), encoded...)), nil
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
	wire := branchDiffWire{CoHi: diff.CoHi, HiPath: hi, HiIsLen: diff.HiIsLen, CoHi2: diff.CoHi2, Hi2IsLen: diff.Hi2IsLen, HasHi2: diff.HasHi2, LoPath: lo, LoIsLen: diff.LoIsLen, C: diff.C, Edge: diff.Edge}
	if diff.HasHi2 {
		wire.Hi2Path, err = checkPathKey(diff.Hi2Path)
		if err != nil {
			return equation.Term{}, fmt.Errorf("second high path: %w", err)
		}
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return equation.Term{}, fmt.Errorf("encode difference constraint: %w", err)
	}
	return equation.ClosedTerm(append([]byte(branchDiffPrefix), encoded...)), nil
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
