package engine

import (
	"encoding/base64"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// nilabilityNativeFacts projects only refinements that the lowered body has
// already made explicit: a nil-capable path, plus a normalized branch or
// assertion that establishes its non-nil (or nil) arm. It does not attempt to
// reconstruct control flow or infer optionality from source spelling.
func nilabilityNativeFacts(root front.Compilation) []NativeFact {
	var rows []NativeFact
	var visit func(front.Compilation)
	visit = func(compilation front.Compilation) {
		rows = append(rows, nilabilityBodyFacts(compilation)...)
		for _, child := range compilation.Nested {
			visit(child)
		}
	}
	visit(root)
	return rows
}

func nilabilityBodyFacts(compilation front.Compilation) []NativeFact {
	body := compilation.WIR
	if body == nil {
		return nil
	}
	types := nativePathTypes(body)
	var rows []NativeFact
	seenBranches := make(map[string]struct{})
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		occurrence := fmt.Sprintf("op-%08d", index)
		switch instruction.Op {
		case wir.OpBranch:
			check := body.Check(instruction.Check)
			path, then, otherwise, ok := nilabilityBranch(check, types)
			if !ok {
				continue
			}
			if _, emitted := seenBranches[string(path.Key())]; emitted {
				continue
			}
			seenBranches[string(path.Key())] = struct{}{}
			rows = append(rows, nilabilityNativeRow(compilation, occurrence, path,
				"else_edge="+otherwise+" nilability=non_nil then_edge="+then))
			switch check.Kind {
			case wir.CheckNotNil:
				// A direct `~= nil` establishes both arm facts independently.
				rows = append(rows,
					nilabilityNativeRow(compilation, occurrence, path, "nilability=non_nil"),
					nilabilityNativeRow(compilation, occurrence, path, "nilability=nil"),
				)
			case wir.CheckTruthy, wir.CheckFalsy:
				rows = append(rows, nilabilityNativeRow(compilation, occurrence, path, "nilability=nil"))
			case wir.CheckNil:
				// A captured path's only surviving fact is the post-guard non-nil
				// arm. Its nil arm can be changed by the closure handed to a call.
				if !nativePathIsCaptured(body, path) {
					rows = append(rows, nilabilityNativeRow(compilation, occurrence, path, "nilability=nil"))
				}
			}
		case wir.OpClaim:
			if instruction.Claim != wir.ClaimAssert {
				continue
			}
			path, ok := nativeOperandPath(body, instruction.A)
			if !ok || !nativeNilCapablePath(path, types) {
				continue
			}
			rows = append(rows, nilabilityNativeRow(compilation, occurrence, path, "nilability=non_nil"))
		case wir.OpCall:
			// Lowering attaches this check only to a recognized call whose normal
			// completion validates the operand. The check, rather than a callee
			// spelling, is the authority for this postcondition.
			check := body.Check(instruction.Check)
			path, _, _, ok := nilabilityBranch(check, types)
			if !ok || check.Kind != wir.CheckTruthy {
				continue
			}
			rows = append(rows, nilabilityNativeRow(compilation, occurrence, path, "nilability=non_nil"))
		}
	}
	return rows
}

func nativePathTypes(body *wir.Body) map[string]typ.Type {
	types := make(map[string]typ.Type)
	for _, root := range body.RootTypes() {
		if root.Type == 0 {
			continue
		}
		if value := body.Type(root.Type); value != nil {
			types[string(root.Path.Key())] = value
		}
	}
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpClaim || instruction.Dst.Kind != wir.OperandPath || instruction.Type == 0 {
			continue
		}
		if value := body.Type(instruction.Type); value != nil {
			types[string(body.Path(wir.PathRef(instruction.Dst.Ref)).Key())] = value
		}
	}
	return types
}

func nativeOperandPath(body *wir.Body, operand wir.Operand) (path.Path, bool) {
	if operand.Kind != wir.OperandPath {
		return path.Path{}, false
	}
	return body.Path(wir.PathRef(operand.Ref)), true
}

func nativeNilCapablePath(path path.Path, types map[string]typ.Type) bool {
	value, ok := nativePathType(path, types)
	return ok && value != nil && typevalue.TypeIncludesNil(value) && proof.ProjectionWithoutNil(value) != nil
}

func nativePathType(path path.Path, types map[string]typ.Type) (typ.Type, bool) {
	if value, ok := types[string(path.Key())]; ok {
		return value, true
	}
	root := path
	root.Segments = nil
	value, ok := types[string(root.Key())]
	if !ok || value == nil {
		return nil, false
	}
	for _, part := range path.Segments {
		if part.Kind != segment.SegmentField { // Other projections have no exact nilability witness here.
			return nil, false
		}
		value, ok = access.Field(value, part.Name)
		if !ok || value == nil {
			return nil, false
		}
	}
	return value, true
}

func nilabilityBranch(check wir.Check, types map[string]typ.Type) (path.Path, string, string, bool) {
	if !nativeNilCapablePath(check.Path, types) {
		return path.Path{}, "", "", false
	}
	switch check.Kind {
	case wir.CheckNil:
		return check.Path, "nil", "non_nil", true
	case wir.CheckNotNil:
		return check.Path, "non_nil", "nil", true
	case wir.CheckTruthy:
		if !nativeTruthyNilOnly(check.Path, types) {
			return path.Path{}, "", "", false
		}
		return check.Path, "non_nil", "nil", true
	case wir.CheckFalsy:
		if !nativeTruthyNilOnly(check.Path, types) {
			return path.Path{}, "", "", false
		}
		return check.Path, "nil", "non_nil", true
	default:
		return path.Path{}, "", "", false
	}
}

func nativeTruthyNilOnly(path path.Path, types map[string]typ.Type) bool {
	value, ok := nativePathType(path, types)
	if !ok || value == nil || !typevalue.TypeIncludesNil(value) {
		return false
	}
	withoutNil := proof.ProjectionWithoutNil(value)
	return withoutNil != nil && (typ.TypeEquals(withoutNil, typ.String) || typ.TypeEquals(withoutNil, typ.Number))
}

func nilabilityNativeRow(compilation front.Compilation, occurrence string, path path.Path, value string) NativeFact {
	events := []string{"write.local"}
	if len(path.Segments) != 0 {
		events = []string{"write.field", "call.opaque", "escape", "suspend"}
	} else if nativePathIsCaptured(compilation.WIR, path) {
		events = []string{"write.local", "write.upvalue", "call.opaque"}
	}
	encoded := base64.RawURLEncoding.EncodeToString([]byte(path.Key()))
	return NativeFact{
		Lane: NativeLaneValues, Family: "nilability",
		Key:   "nilability/" + fmt.Sprintf("%x", compilation.Body) + "/" + occurrence + "/" + encoded,
		Value: value, Subject: path.String(), Occurrence: occurrence, Trust: NativeTrustProven,
		Established: "contract", Revoked: "contract/nilability", Event: events[0], Revocations: nativeContractRevocations("contract/nilability", events),
	}
}

func nativeContractRevocations(revoked string, events []string) []NativeRevocation {
	out := make([]NativeRevocation, 0, len(events))
	for _, event := range events {
		if event != "" {
			out = append(out, NativeRevocation{Established: "contract", Revoked: revoked, Event: event})
		}
	}
	return out
}

func nativePathIsCaptured(body *wir.Body, path path.Path) bool {
	if body == nil {
		return false
	}
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpClosure {
			continue
		}
		for _, operand := range body.Operands(instruction.List) {
			candidate, ok := nativeOperandPath(body, operand)
			if ok && candidate.Key() == path.Key() {
				return true
			}
		}
	}
	return false
}
