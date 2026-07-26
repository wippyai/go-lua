package engine

import (
	"fmt"

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

// N7 residual — host_global_binding cannot be licensed from globals' typ.Type
// input alone. A sound kernel publication still needs the host's stable binding
// identity plus managed ownership, rooting, release, write.global, and
// load.dynamic epoch capabilities. Until the host boundary supplies them, this
// scan remains.
//
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
