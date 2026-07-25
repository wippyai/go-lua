package engine

import (
	"fmt"
	"strconv"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Truthiness classes. Lua's falsy set is exactly {nil, false}, so `0` and `""`
// are truthy and a single boolean cannot express the third case: a condition
// that is decided at runtime keeps its test.
const (
	nativeTruthyClass  = "always_truthy"
	nativeFalsyClass   = "always_falsy"
	nativeDynamicClass = "dynamic_nil_or_false"
)

// branchNativeFacts projects the point-local truthiness class of every branch
// condition the body lowers, and the partition of the branch that condition
// decides. Both read the resolved WIR: the normalized condition descriptor, the
// exact constant a single-assignment binding was born with, and the declared
// type of the condition path. A condition the body does not decide publishes
// the dynamic class rather than silence, because absence names no location for
// a consumer to keep its test at.
//
// decided carries the branch coordinates the value closure already partitioned.
// Those coordinates keep the closure's row and publish no second verdict here.
func branchNativeFacts(root front.Compilation, decided map[string]bool) []NativeFact {
	var rows []NativeFact
	var visit func(front.Compilation)
	visit = func(compilation front.Compilation) {
		rows = append(rows, branchBodyFacts(compilation, decided)...)
		for _, child := range compilation.Nested {
			visit(child)
		}
	}
	visit(root)
	return rows
}

func branchBodyFacts(compilation front.Compilation, decided map[string]bool) []NativeFact {
	body := compilation.WIR
	if body == nil {
		return nil
	}
	constants, writes := nativeBodyConstants(body)
	types := nativeBodyRootTypes(compilation, body)
	row := func(family, occurrence, subject, content string) NativeFact {
		return NativeFact{
			Lane: NativeLaneValues, Family: family,
			Key:   family + "/" + fmt.Sprintf("%x", compilation.Body) + "/" + occurrence,
			Value: content, Subject: subject, Occurrence: occurrence, Trust: NativeTrustProven,
		}
	}

	var out []NativeFact
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpBranch {
			continue
		}
		occurrence := fmt.Sprintf("op-%08d", index)
		if decided[fmt.Sprintf("%x", compilation.Body)+"/"+occurrence] {
			continue
		}
		check := body.Check(instruction.Check)
		name, subject := nativeCheckSubject(check)
		var constant nativeConstant
		known := false
		if name != "" && writes[name] <= 1 {
			constant, known = constants[name]
		}

		switch check.Kind {
		case wir.CheckTruthy, wir.CheckFalsy:
			class := nativeTruthinessClass(constant, known, types[name])
			out = append(out, row("truthiness_class", occurrence, subject, "class="+class))
			out = append(out, row("branch_partition", occurrence, subject,
				nativeBranchPartition(nativeTruthEdge(class, check.Kind == wir.CheckFalsy))))
		case wir.CheckNumGe, wir.CheckNumLe:
			out = append(out, row("branch_partition", occurrence, subject,
				nativeBranchPartition(nativeNumericEdge(check, constant, known))))
		default:
			out = append(out, row("branch_partition", occurrence, subject, nativeBranchPartition(nativeEdgeDynamic)))
		}
	}
	return out
}

// nativeBranchEdge is the arm a condition selects: the true edge, the false
// edge, or neither because the condition is decided at runtime.
type nativeBranchEdge int

const (
	nativeEdgeDynamic nativeBranchEdge = iota
	nativeEdgeTrue
	nativeEdgeFalse
)

func nativeBranchPartition(edge nativeBranchEdge) string {
	switch edge {
	case nativeEdgeTrue:
		return "dead_arm=else dead_arm_reachable=false partition=always_taken"
	case nativeEdgeFalse:
		return "dead_arm=then dead_arm_reachable=false partition=always_not_taken"
	default:
		return "partition=dynamic"
	}
}

// nativeTruthEdge maps a truthiness class onto the branch edge it forces. A
// falsy check inverts the mapping: the true edge of `if not x` is x's falsy
// side.
func nativeTruthEdge(class string, negated bool) nativeBranchEdge {
	truthy := class == nativeTruthyClass
	if class != nativeTruthyClass && class != nativeFalsyClass {
		return nativeEdgeDynamic
	}
	if truthy != negated {
		return nativeEdgeTrue
	}
	return nativeEdgeFalse
}

// nativeTruthinessClass decides the class from the exact constant the binding
// carries, and from its declared type when no constant does. The type answer is
// only ever a positive class when the type admits no falsy value at all; every
// other type stays dynamic.
func nativeTruthinessClass(constant nativeConstant, known bool, declared typ.Type) string {
	if known {
		switch constant.kind {
		case wir.ConstNil:
			return nativeFalsyClass
		case wir.ConstBool:
			if constant.boolean {
				return nativeTruthyClass
			}
			return nativeFalsyClass
		case wir.ConstNumber, wir.ConstString:
			return nativeTruthyClass
		}
	}
	if declared == nil {
		return nativeDynamicClass
	}
	if !refinement.TypeAdmitsFalsy(declared) && refinement.TypeAdmitsTruthy(declared) {
		return nativeTruthyClass
	}
	if !refinement.TypeAdmitsTruthy(declared) && refinement.TypeAdmitsFalsy(declared) {
		return nativeFalsyClass
	}
	return nativeDynamicClass
}

// nativeNumericEdge decides a normalized numeric comparison against the exact
// integer constant its subject was born with. A float literal is deliberately
// not admitted: the bound is an integer and a float subject would decide the
// arm on a rounding rule the descriptor does not carry.
func nativeNumericEdge(check wir.Check, constant nativeConstant, known bool) nativeBranchEdge {
	if !known || constant.kind != wir.ConstNumber || !numericLiteralIsInteger(constant.number) {
		return nativeEdgeDynamic
	}
	value, err := strconv.ParseInt(constant.number, 10, 64)
	if err != nil {
		return nativeEdgeDynamic
	}
	var holds bool
	switch check.Kind {
	case wir.CheckNumGe:
		holds = value >= check.NumFloor
	case wir.CheckNumLe:
		if !check.HasNumCeil {
			return nativeEdgeDynamic
		}
		holds = value <= check.NumCeil
	default:
		return nativeEdgeDynamic
	}
	// A negated bound holds on the false edge of the comparison, so the arm it
	// selects is the mirror of the bound's own verdict.
	if check.Negated {
		holds = !holds
	}
	if holds {
		return nativeEdgeTrue
	}
	return nativeEdgeFalse
}

// nativeConstant is the exact constant a binding was born with, in the
// vocabulary the lowered body already carries.
type nativeConstant struct {
	kind    wir.ConstKind
	boolean bool
	number  string
}

// nativeBodyConstants records the constant every binding is assigned and how
// many times the body writes it. A binding written more than once carries no
// single constant, so the write count is returned with the environment rather
// than folded into it.
func nativeBodyConstants(body *wir.Body) (map[string]nativeConstant, map[string]int) {
	constants, writes := make(map[string]nativeConstant), make(map[string]int)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpAssign {
			continue
		}
		name := nativeOperandKey(body, instruction.Dst)
		if name == "" {
			continue
		}
		writes[name]++
		if instruction.A.Kind != wir.OperandConst {
			continue
		}
		constant := body.Const(wir.ConstRef(instruction.A.Ref))
		constants[name] = nativeConstant{kind: constant.Kind, boolean: constant.Bool, number: constant.Number}
	}
	return constants, writes
}

// nativeBodyRootTypes joins the declared types the body resolved for its roots
// and its formals under the operand key vocabulary the rest of this projection
// uses.
func nativeBodyRootTypes(compilation front.Compilation, body *wir.Body) map[string]typ.Type {
	types := make(map[string]typ.Type)
	for _, root := range body.RootTypes() {
		if name := nativePathKey(root.Path.Symbol, root.Path.Key()); name != "" {
			types[name] = body.Type(root.Type)
		}
	}
	for _, parameter := range compilation.Boundary.Parameters {
		if parameter.Symbol != 0 {
			types[fmt.Sprintf("sym%d", parameter.Symbol)] = body.Type(parameter.Type)
		}
	}
	return types
}

func nativeOperandKey(body *wir.Body, operand wir.Operand) string {
	switch operand.Kind {
	case wir.OperandPath:
		item := body.Path(wir.PathRef(operand.Ref))
		return nativePathKey(item.Symbol, item.Key())
	case wir.OperandTemp:
		return fmt.Sprintf("temp/%d", operand.Ref)
	default:
		return ""
	}
}

func nativePathKey(symbol wir.SymbolID, key path.PathKey) string {
	if symbol != 0 {
		return fmt.Sprintf("sym%d", symbol)
	}
	return string(key)
}

// nativeCheckSubject names the binding a normalized condition tests. A check
// with no path names nothing, and the row it produces carries no subject rather
// than a reconstructed one.
func nativeCheckSubject(check wir.Check) (name, subject string) {
	if check.Path.IsEmpty() {
		return "", ""
	}
	return nativePathKey(check.Path.Symbol, check.Path.Key()), check.Path.String()
}
