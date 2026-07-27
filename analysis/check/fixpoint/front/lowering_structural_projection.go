package front

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/ir/wir"
)

func structuralRow(compilation Compilation, family, occurrence, subject, value string) NativeProjection {
	return NativeProjection{
		Key:        family + "/" + fmt.Sprintf("%x", compilation.Body) + "/" + occurrence,
		Value:      value,
		Subject:    subject,
		Occurrence: occurrence,
	}
}

// A host binding row is licensed only when the semantic-tail publication
// kernel can join this lowering-owned call coordinate with the project-owned
// stable global binding and a concrete callable result contract. The draft
// carries the path structurally; an absent capability publishes no row.
func hostGlobalBindingFacts(root Compilation) []NativeProjection {
	var rows []NativeProjection
	forEachNativeBody(root, func(compilation Compilation) {
		body := compilation.WIR
		for index := 0; body != nil && index < body.Len(); index++ {
			instruction := body.Instr(index)
			if instruction.Op != wir.OpCall || instruction.Call.Callee.Kind != wir.OperandPath || instruction.Results.Len == 0 {
				continue
			}
			callee := body.Path(wir.PathRef(instruction.Call.Callee.Ref))
			rootPath := callee.RootOnly()
			if callee.Symbol == 0 || !body.IsImplicitGlobalSymbol(callee.Symbol) || rootPath.String() == "" {
				continue
			}
			fields := make([]string, 0, len(callee.Segments))
			complete := true
			for _, part := range callee.Segments {
				if part.Name == "" {
					complete = false
					break
				}
				fields = append(fields, part.Name)
			}
			if !complete {
				continue
			}
			occurrence := fmt.Sprintf("op-%08d", index)
			row := structuralRow(compilation, "host_global_binding", occurrence, callee.String(),
				"identity=published managed=true ownership=published release=published rooting=published type=published used_order=published value_carrier=published")
			row.Established, row.Revoked, row.Event = occurrence, "write.global", "write.global"
			row.Revocations = []NativeProjectionRevocation{
				{Established: occurrence, Revoked: "write.global", Event: "write.global"},
				{Established: "contract", Revoked: "contract/load.dynamic", Event: "load.dynamic"},
			}
			row.HostGlobal = &NativeHostGlobalRequirement{Root: rootPath.String(), Fields: fields}
			rows = append(rows, row)
		}
	})
	return rows
}
