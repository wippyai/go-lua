package engine

import (
	"fmt"
	"strconv"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

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

// A resolved host call has a native binding exactly when the project supplied
// the root global and lowering resolved a non-top result contract. This is the
// same authority the call-results kernel consumes; publishing it before solve
// removes the former Result.Native join against already-rendered value rows.
func hostGlobalBindingFactsFromGlobals(compilation front.Compilation, globals map[string]typ.Type) []NativeFact {
	body := compilation.WIR
	if body == nil || len(globals) == 0 {
		return nil
	}
	var rows []NativeFact
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpCall || instruction.Call.Callee.Kind != wir.OperandPath || instruction.Results.Len == 0 {
			continue
		}
		callee := body.Path(wir.PathRef(instruction.Call.Callee.Ref))
		root := callee.RootOnly()
		if callee.Symbol == 0 || !body.IsImplicitGlobalSymbol(callee.Symbol) || globals[root.String()] == nil {
			continue
		}
		calleeType := globals[root.String()]
		var resolved bool
		for _, part := range callee.Segments {
			if part.Name == "" {
				resolved = false
				break
			}
			calleeType, resolved = access.Field(calleeType, part.Name)
			if !resolved {
				break
			}
		}
		function, callable := unwrap.Alias(calleeType).(*typ.Function)
		if !resolved || !callable || len(function.Returns) == 0 {
			continue
		}
		resultType := unwrap.Alias(function.Returns[0])
		if resultType == nil || typ.AbsentOrTopLike(resultType) || resultType.Kind() == kind.Any || resultType.Kind() == kind.Unknown {
			continue
		}
		occurrence := fmt.Sprintf("op-%08d", index)
		row := structuralRow(compilation, "host_global_binding", occurrence, callee.String(),
			"identity=published managed=true ownership=published release=published rooting=published type=published used_order=published value_carrier=published")
		row.Established, row.Revoked, row.Event = occurrence, "write.global", "write.global"
		row.Revocations = []NativeRevocation{
			{Established: occurrence, Revoked: "write.global", Event: "write.global"},
			{Established: "contract", Revoked: "contract/load.dynamic", Event: "load.dynamic"},
		}
		rows = append(rows, row)
	}
	return rows
}
