package engine

import (
	"fmt"
	"strconv"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
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
	var visit func(front.Compilation, int)
	visit = func(compilation front.Compilation, depth int) {
		rows = append(rows, captureNativeFacts(compilation, depth)...)
		rows = append(rows, typedProducerNativeFacts(compilation)...)
		rows = append(rows, tableConstructionBoundFacts(compilation)...)
		rows = append(rows, hostGlobalBindingFacts(compilation, published)...)
		for _, child := range compilation.Nested {
			visit(child, depth+1)
		}
	}
	visit(root, 0)
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

func captureNativeFacts(compilation front.Compilation, depth int) []NativeFact {
	body := compilation.WIR
	if body == nil || bodyHasNumericLoop(body) {
		// A closure constructed in a numeric loop can denote a distinct lexical
		// environment per iteration.  No singleton epoch root is available.
		return nil
	}
	var rows []NativeFact
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpClosure || instruction.Func == 0 || instruction.List.Len == 0 {
			continue
		}
		child := body.Proto(instruction.Func)
		if child.Body == nil || len(child.Boundary.Captures) != int(instruction.List.Len) {
			continue
		}
		occurrence := fmt.Sprintf("op-%08d", index)
		coordinate := "edge_form"
		if depth == 1 && len(compilation.Boundary.Parameters) == 0 && len(compilation.Boundary.Captures) == 0 {
			coordinate = "entry"
		}
		for captureIndex, operand := range body.Operands(instruction.List) {
			if operand.Kind != wir.OperandPath {
				continue
			}
			captured := body.Path(wir.PathRef(operand.Ref))
			if captured.IsEmpty() || captured.Symbol == 0 || child.Boundary.Captures[captureIndex].Symbol != captured.Symbol {
				continue
			}
			value := "active_epoch_root=1 begin_coordinate=" + coordinate + " coverage_proof=complete uniqueness_proof=complete"
			if coordinate == "edge_form" {
				value += " coordinate_fields=[from,to,edge_kind,edge_ordinal,begin_ordinal] levels=" + strconv.Itoa(depth+1)
			}
			rows = append(rows, structuralRow(compilation, "capture_epoch_root", occurrence+"/"+strconv.Itoa(captureIndex), captured.String(), value))
		}
		rows = append(rows, captureTransportFacts(compilation, index, instruction)...)
	}
	return rows
}

func captureTransportFacts(compilation front.Compilation, closureIndex int, closure wir.Instruction) []NativeFact {
	body := compilation.WIR
	if body == nil {
		return nil
	}
	var rows []NativeFact
	for _, operand := range body.Operands(closure.List) {
		if operand.Kind != wir.OperandPath {
			continue
		}
		root := body.Path(wir.PathRef(operand.Ref))
		if root.IsEmpty() || !isNumericArrayType(typeAtTableBirth(body, root)) || !captureTableFilledBefore(body, root, closureIndex) {
			continue
		}
		value := "carried_through=closure_construction element_class=number initialization=complete presence=dense_prefix"
		occurrence := fmt.Sprintf("op-%08d", closureIndex)
		row := structuralRow(compilation, "capture_transport", occurrence, root.String(), value)
		row.Established = occurrence
		// The contract's deopt alternatives are independently materialized so
		// consumers can retain the exact event that applies at their boundary.
		for _, event := range []string{"write.element", "write.length", "grow"} {
			candidate := row
			candidate.Key += "/" + event
			candidate.Revoked = event
			candidate.Event = event
			rows = append(rows, candidate)
		}
	}
	return rows
}

func typeAtTableBirth(body *wir.Body, root path.Path) typ.Type {
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpMakeTable || instruction.Dst.Kind != wir.OperandPath {
			continue
		}
		candidate := body.Path(wir.PathRef(instruction.Dst.Ref))
		if candidate.Key() == root.Key() {
			return body.Type(instruction.Type)
		}
	}
	return nil
}

func isNumericArrayType(value typ.Type) bool {
	value = unwrap.Alias(value)
	array, ok := value.(*typ.Array)
	if !ok || array.Element == nil {
		return false
	}
	element := unwrap.Alias(array.Element)
	return element != nil && (element.Kind() == kind.Number || element.Kind() == kind.Integer)
}

func captureTableFilledBefore(body *wir.Body, root path.Path, end int) bool {
	entries := make(map[int]bool)
	for index := 0; index < end; index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpStaticMemberWrite || instruction.Dst.Kind != wir.OperandPath {
			continue
		}
		member := body.Path(wir.PathRef(instruction.Dst.Ref))
		if !member.SameRootIgnoringVersion(root) || len(member.Segments) != 1 || member.Segments[0].Kind != segment.SegmentIndexInt {
			continue
		}
		entries[member.Segments[0].Index] = true
	}
	if len(entries) == 0 {
		return false
	}
	for index := 1; index <= len(entries); index++ {
		if !entries[index] {
			return false
		}
	}
	return true
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

func bodyHasNumericLoop(body *wir.Body) bool {
	for index := 0; index < body.Len(); index++ {
		if instruction := body.Instr(index); instruction.Op == wir.OpIterate && instruction.Iter == wir.IterNumeric {
			return true
		}
	}
	return false
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
