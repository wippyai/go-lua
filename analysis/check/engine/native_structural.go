package engine

import (
	"fmt"
	"strconv"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// structuralNativeFacts projects only lowering-owned topology and already
// published value evidence.  It does not run a second dataflow analysis: an
// absent capture, constructor, resolved annotation, or proven host result
// simply leaves the corresponding contract unpublished.
func structuralNativeFacts(root front.Compilation, published []NativeFact) []NativeFact {
	var rows []NativeFact
	var visit func(front.Compilation)
	visit = func(compilation front.Compilation) {
		rows = append(rows, typedProducerNativeFacts(compilation)...)
		rows = append(rows, tableConstructionBoundFacts(compilation)...)
		rows = append(rows, hostGlobalBindingFacts(compilation, published)...)
		for _, child := range compilation.Nested {
			visit(child)
		}
	}
	visit(root)
	return rows
}

func structuralRow(compilation front.Compilation, family, occurrence, subject, value string) NativeFact {
	return NativeFact{
		Lane:       NativeLaneValues,
		Family:     family,
		Key:        family + "/" + fmt.Sprintf("%x", compilation.Body) + "/" + occurrence,
		Value:      value,
		Subject:    subject,
		Occurrence: occurrence,
		Trust:      NativeTrustProven,
	}
}

func typedProducerNativeFacts(compilation front.Compilation) []NativeFact {
	body := compilation.WIR
	if body == nil {
		return nil
	}
	var rows []NativeFact
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpClaim || instruction.Claim != wir.ClaimAnnotation || instruction.Dst.Kind != wir.OperandPath {
			continue
		}
		declared := unwrap.Alias(body.Type(instruction.Type))
		if declared == nil || declared.Kind() == kind.Unknown {
			continue
		}
		value := "classification=compile_time_only"
		if nativeLayoutRelevant(declared) {
			value = "classification=runtime_relevant requires_native_operation_contract=true"
		} else if declared.Kind() != kind.Any {
			value += " value_bit_identity=true"
		}
		occurrence := fmt.Sprintf("op-%08d", index)
		rows = append(rows, structuralRow(compilation, "typed_producer", occurrence, body.Path(wir.PathRef(instruction.Dst.Ref)).String(), value))
	}
	return rows
}

func nativeLayoutRelevant(value typ.Type) bool {
	switch value.Kind() {
	case kind.Record, kind.Array, kind.Map, kind.ReadonlyMap, kind.Tuple:
		return true
	default:
		return false
	}
}

func tableConstructionBoundFacts(compilation front.Compilation) []NativeFact {
	body := compilation.WIR
	if body == nil {
		return nil
	}
	var rows []NativeFact
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpMakeTable || !isRecordType(body.Type(instruction.Type)) {
			continue
		}
		count, inLoop, exact := constructorOccurrences(body, index)
		if !exact {
			continue
		}
		mode := "once_only"
		if inLoop {
			mode = "repeatable"
		}
		occurrence := fmt.Sprintf("op-%08d", index)
		rows = append(rows, structuralRow(compilation, "table_construction_bound", occurrence, "", "max_occurrences="+strconv.FormatInt(count, 10)+" occurrence_mode="+mode))
	}
	return rows
}

func isRecordType(value typ.Type) bool {
	value = unwrap.Alias(value)
	return value != nil && value.Kind() == kind.Record
}

func constructorOccurrences(body *wir.Body, constructor int) (int64, bool, bool) {
	for index := constructor - 1; index >= 0; index-- {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpIterate || instruction.Iter != wir.IterNumeric {
			continue
		}
		operands := body.Operands(instruction.List)
		if len(operands) != 3 {
			return 0, true, false
		}
		start, startOK := integerConst(body, operands[0])
		limit, limitOK := integerConst(body, operands[1])
		step, stepOK := integerConst(body, operands[2])
		if !startOK || !limitOK || !stepOK || step == 0 {
			return 0, true, false
		}
		if step > 0 {
			if start > limit {
				return 0, true, true
			}
			return (limit-start)/step + 1, true, true
		}
		if start < limit {
			return 0, true, true
		}
		return (start-limit)/(-step) + 1, true, true
	}
	return 1, false, true
}

func integerConst(body *wir.Body, operand wir.Operand) (int64, bool) {
	if operand.Kind != wir.OperandConst {
		return 0, false
	}
	constant := body.Const(wir.ConstRef(operand.Ref))
	if constant.Kind != wir.ConstNumber {
		return 0, false
	}
	value, err := strconv.ParseInt(constant.Number, 10, 64)
	return value, err == nil
}

func hostGlobalBindingFacts(compilation front.Compilation, published []NativeFact) []NativeFact {
	body := compilation.WIR
	if body == nil {
		return nil
	}
	var rows []NativeFact
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpCall || instruction.Call.Callee.Kind != wir.OperandPath || instruction.Results.Len == 0 {
			continue
		}
		callee := body.Path(wir.PathRef(instruction.Call.Callee.Ref))
		if callee.Symbol == 0 || !body.IsImplicitGlobalSymbol(callee.Symbol) {
			continue
		}
		results := body.Operands(instruction.Results)
		if !publishedProvenValue(compilation, published, results[0]) {
			continue
		}
		occurrence := fmt.Sprintf("op-%08d", index)
		value := "identity=published managed=true ownership=published release=published rooting=published type=published used_order=published value_carrier=published"
		row := structuralRow(compilation, "host_global_binding", occurrence, callee.String(), value)
		row.Established = occurrence
		// The global identity remains a capability boundary: a rebinding or
		// dynamic load invalidates the native binding contract.
		row.Revoked = "write.global"
		row.Event = "write.global"
		row.Revocations = []NativeRevocation{
			{Established: row.Established, Revoked: row.Revoked, Event: "write.global"},
			{Established: "contract", Revoked: "contract/load.dynamic", Event: "load.dynamic"},
		}
		rows = append(rows, row)
	}
	return rows
}

func publishedProvenValue(compilation front.Compilation, published []NativeFact, operand wir.Operand) bool {
	if operand.Kind != wir.OperandTemp && operand.Kind != wir.OperandPath {
		return false
	}
	var key string
	if operand.Kind == wir.OperandTemp {
		key = "temp/" + strconv.FormatUint(uint64(operand.Ref), 10)
	} else {
		key = string(compilation.WIR.Path(wir.PathRef(operand.Ref)).Key())
	}
	for _, fact := range published {
		if fact.Lane == NativeLaneValues && fact.Family == "value" && fact.Trust == NativeTrustProven && fact.Term == key {
			return true
		}
	}
	return false
}
