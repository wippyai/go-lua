package engine

import (
	"encoding/base64"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// N7 residual — nilability needs point-local publications not only for branch
// refinements but also assertions, writes, captured-call invalidation, and loop
// backedge widening. The solve does not yet publish one complete epoch/capture/
// backedge invalidation class for those coordinates, so this scan remains.
//
// nilabilityNativeFacts projects only refinements that the lowered body has
// already made explicit: a nil-capable path, plus a normalized branch or
// assertion that establishes its non-nil (or nil) arm. It consults the
// authoritative CFG for loop membership so a guard whose narrowing a backedge
// revokes widens to maybe_nil, but it never reconstructs that topology or
// infers optionality from source spelling.
func nilabilityNativeFacts(root front.Compilation) []NativeFact {
	var rows []NativeFact
	forEachNativeBody(root, func(compilation front.Compilation) {
		rows = append(rows, nilabilityBodyFacts(compilation)...)
	})
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
	backedge := newBackedgeCarriers(compilation)
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
				// A direct `~= nil` establishes both arm facts independently. When a
				// loop backedge reassigns the carrier from an optional source before
				// re-entering this guard, the then-arm non_nil does not survive to the
				// loop header: the loop-carried value there is maybe_nil.
				arm := "nilability=non_nil"
				if backedge.widens(instruction.Point, path, types) {
					arm = "nilability=maybe_nil"
				}
				rows = append(rows,
					nilabilityNativeRow(compilation, occurrence, path, arm),
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

// backedgeCarriers answers whether a guard's non-nil narrowing survives a loop
// backedge. It uses the body's authoritative CFG reachability, never
// instruction order, so a reassignment that merely follows a guard in source is
// not mistaken for one that flows back to it. Reachability is built once and
// only when the source topology actually has a cycle.
type backedgeCarriers struct {
	body  *wir.Body
	reach *cfg.Reachability
}

func newBackedgeCarriers(compilation front.Compilation) backedgeCarriers {
	carriers := backedgeCarriers{body: compilation.WIR}
	// Cyclic is non-nil exactly when the body's source topology has a cycle, so a
	// backedge is possible. An acyclic body needs no reachability query.
	if compilation.Graph != nil && compilation.Cyclic != nil {
		carriers.reach = cfg.NewReachability(compilation.Graph)
	}
	return carriers
}

// widens reports that the carrier guarded at branch is reassigned from an
// optional source at a point sharing a cycle with the branch. The backedge then
// carries a possibly-nil value back to the loop header, so the guard's non_nil
// narrowing must widen to maybe_nil there.
func (c backedgeCarriers) widens(branch cfg.Point, carrier path.Path, types map[string]typ.Type) bool {
	if c.reach == nil || c.body == nil || len(carrier.Segments) != 0 {
		return false
	}
	for index := 0; index < c.body.Len(); index++ {
		instruction := c.body.Instr(index)
		if instruction.Dst.Kind != wir.OperandPath {
			continue
		}
		target := c.body.Path(wir.PathRef(instruction.Dst.Ref))
		if len(target.Segments) != 0 || target.Symbol != carrier.Symbol || target.Root != carrier.Root {
			continue
		}
		if !c.reach.CanReach(branch, instruction.Point) || !c.reach.CanReach(instruction.Point, branch) {
			continue
		}
		if nativeWriteSourceOptional(c.body, instruction, types) {
			return true
		}
	}
	return false
}

// nativeWriteSourceOptional reports whether the value a write binds into its
// destination is nil-capable. Only an optional source leaves a loop-carried
// carrier maybe_nil at the loop header; a write of a proven non-nil value does
// not revoke the guard.
func nativeWriteSourceOptional(body *wir.Body, instruction wir.Instruction, types map[string]typ.Type) bool {
	switch instruction.Op {
	case wir.OpDynamicIndexRead:
		container, ok := nativeOperandPath(body, instruction.A)
		if !ok {
			return false
		}
		containerType, ok := nativePathType(container, types)
		if !ok || containerType == nil {
			return false
		}
		element, ok := access.RuntimeIndex(containerType, nativeIndexKeyType(body, instruction.B, types))
		return ok && element != nil && typevalue.TypeIncludesNil(element)
	case wir.OpAssign:
		source, ok := nativeOperandPath(body, instruction.A)
		if !ok {
			return false
		}
		sourceType, ok := nativePathType(source, types)
		return ok && sourceType != nil && typevalue.TypeIncludesNil(sourceType)
	}
	return false
}

// nativeIndexKeyType recovers the key type of a dynamic index read so element
// projection distinguishes array from keyed-container element optionality. A key
// with no witnessed type defaults to a numeric index, the array read shape.
func nativeIndexKeyType(body *wir.Body, operand wir.Operand, types map[string]typ.Type) typ.Type {
	if key, ok := nativeOperandPath(body, operand); ok {
		if value, ok := nativePathType(key, types); ok && value != nil {
			return value
		}
	}
	return typ.Number
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
