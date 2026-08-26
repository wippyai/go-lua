package result

import "sort"

// The declared-not-implemented native fact family register.
//
// A native fact family is a published external identity in exactly the sense a
// diagnostic code is: a fixture, a probe, or a downstream consumer may name one
// before an issuer publishes rows under it. Naming an unimplemented family used
// to produce nothing at all - the selector matched no row, an upper bound on
// zero rows was satisfied, and the run reported a clean fixture. Silence is not
// an answer a declaration may receive.
//
// This register is the second half of the family vocabulary whose first half is
// the closed nativePublicationFamily enum above. A family is either implemented
// - the enum holds a member that issues rows under that spelling - or it is
// declared here, by name, with the surface that owes the fact. Nothing may be
// in both, and nothing a fixture references may be in neither; the laws beside
// this file and the corpus law in oracle hold both halves. Implementing a
// family is therefore a two-line change: the member joins the enum and its
// entry leaves this register.
//
// An owner is a package path because a fact is owed by the surface that holds
// the evidence it is derived from, not by whoever last edited a table.

// NativeFamilyDeclared is one family the analyzer publishes as a named identity
// without issuing rows under it. Owner names the surface that owes the fact;
// Reason states what is missing, in the terms of the analyzer rather than of a
// schedule.
type NativeFamilyDeclared struct {
	Family string
	Owner  string
	Reason string
}

// The surfaces that owe an unissued native fact.
const (
	nativeFamilyOwnerCall           = "domain/call"
	nativeFamilyOwnerCallActivation = "domain/call/activation"
	nativeFamilyOwnerDispatch       = "domain/call/dispatch"
	nativeFamilyOwnerConstraint     = "domain/constraint"
	nativeFamilyOwnerEffect         = "domain/effect"
	nativeFamilyOwnerControl        = "domain/effect/control"
	nativeFamilyOwnerOwnership      = "domain/effect/ownership"
	nativeFamilyOwnerHeap           = "domain/heap"
	nativeFamilyOwnerAllocation     = "domain/heap/allocation"
	nativeFamilyOwnerHeapContext    = "domain/heap/context"
	nativeFamilyOwnerFormalFreeze   = "domain/heap/formalfreeze"
	nativeFamilyOwnerHeapIndex      = "domain/heap/index"
	nativeFamilyOwnerCapture        = "domain/placement/capture"
	nativeFamilyOwnerPlacementPub   = "domain/placement/publication"
	nativeFamilyOwnerCompositePub   = "domain/composite/publication"
	nativeFamilyOwnerSendSafety     = "domain/sendsafety"
	nativeFamilyOwnerType           = "domain/type"
	nativeFamilyOwnerAmbient        = "domain/type/ambient"
	nativeFamilyOwnerAnnotation     = "domain/type/annotation"
	nativeFamilyOwnerAuthority      = "domain/type/authority"
	nativeFamilyOwnerNormalize      = "domain/type/normalize"
	nativeFamilyOwnerTypeRefinement = "domain/type/refinement"
	nativeFamilyOwnerTypeTable      = "domain/type/table"
	nativeFamilyOwnerTransform      = "domain/type/transform"
	nativeFamilyOwnerValue          = "domain/value"
	nativeFamilyOwnerFreshResult    = "domain/value/freshresult"
	nativeFamilyOwnerOrder          = "domain/value/order"
	nativeFamilyOwnerRefinement     = "domain/value/refinement"
	nativeFamilyOwnerRuntimeKind    = "domain/value/runtimekind"
)

// nativeFamiliesDeclared is the authored register, in family order. Every row
// is a fact the analyzer names and does not yet publish.
var nativeFamiliesDeclared = []NativeFamilyDeclared{
	{"alias_disjoint", nativeFamilyOwnerHeap, "no alias-disjointness fact is issued over heap identities"},
	{"branch-proof", nativeFamilyOwnerRefinement, "no branch-edge proof fact is issued"},
	{"builtin_call", nativeFamilyOwnerDispatch, "no builtin-resolution fact is issued, and no global-rebind revocation qualifies one"},
	{"call-argument", nativeFamilyOwnerCallActivation, "no call-argument value fact is issued at an activation"},
	{"call-result", nativeFamilyOwnerFreshResult, "no call-result value fact is issued"},
	{"call_scc", nativeFamilyOwnerCall, "no call-graph strongly-connected-component fact is issued"},
	{"callee_set", nativeFamilyOwnerDispatch, "no callee-set completeness fact is issued"},
	{"capture_epoch_root", nativeFamilyOwnerCapture, "no capture-environment epoch-root fact is issued"},
	{"capture_transport", nativeFamilyOwnerCapture, "no capture-transport fact is issued"},
	{"closure", nativeFamilyOwnerCapture, "no closure prototype-and-captures fact is issued"},
	{"concat_site", nativeFamilyOwnerValue, "no concatenation-site fact is issued; the scalar operator family covers arithmetic only"},
	{"discriminant_select", nativeFamilyOwnerTypeRefinement, "no discriminant-selection exhaustiveness fact is issued"},
	{"effect_row", nativeFamilyOwnerEffect, "no effect row is issued on the native lane"},
	{"epoch", nativeFamilyOwnerHeapContext, "no epoch fact is issued, so no revocation names its deoptimization point"},
	{"eval_node", nativeFamilyOwnerControl, "no evaluation-node fact is issued for short-circuit operands"},
	{"explicit-any", nativeFamilyOwnerAnnotation, "no declared-any fact is issued"},
	{"function_entry", nativeFamilyOwnerCall, "no function-entry completion or result-arity fact is issued"},
	{"heap", nativeFamilyOwnerHeap, "no heap fact is issued on the native lane"},
	{"host_global_binding", nativeFamilyOwnerAmbient, "no host-global binding fact is issued"},
	{"interproc_summary", nativeFamilyOwnerCall, "no interprocedural summary fact is issued"},
	{"list_construction", nativeFamilyOwnerAllocation, "no list-literal construction fact is issued"},
	{"metatable_seal", nativeFamilyOwnerTypeTable, "no metatable-seal fact is issued"},
	{"nilability", nativeFamilyOwnerRefinement, "no nilability fact is issued, so no narrowing or widening is published"},
	{"numeric_branch", nativeFamilyOwnerOrder, "no numeric-branch edge carrier fact is issued"},
	{"numeric_loop_carrier", nativeFamilyOwnerConstraint, "no numeric loop-carrier fact is issued"},
	{"placement", nativeFamilyOwnerPlacementPub, "the placement summary is published on its own surface and issues no native lane row"},
	{"publication_identity", nativeFamilyOwnerCompositePub, "no publication-identity fact is issued"},
	{"record_construction", nativeFamilyOwnerAllocation, "no record-construction fact is issued"},
	{"record_entry_ownership", nativeFamilyOwnerOwnership, "no record-entry ownership fact is issued"},
	{"recursive_type_identity", nativeFamilyOwnerNormalize, "no recursive-type fixpoint identity fact is issued"},
	{"runtime-type-proof", nativeFamilyOwnerRuntimeKind, "no runtime-type proof fact is issued"},
	{"sealed_table", nativeFamilyOwnerFormalFreeze, "no sealed-table fact is issued"},
	{"shape_identity", nativeFamilyOwnerType, "no shape-identity fact is issued"},
	{"shape_transition", nativeFamilyOwnerTransform, "no shape-transition fact is issued"},
	{"table_construction_bound", nativeFamilyOwnerConstraint, "no table-construction bound fact is issued"},
	{"table_element", nativeFamilyOwnerHeap, "no table-element presence or class fact is issued"},
	{"table_growth", nativeFamilyOwnerAllocation, "no table-growth site fact is issued"},
	{"table_length", nativeFamilyOwnerHeapIndex, "no raw table-length fact is issued"},
	{"throw_template", nativeFamilyOwnerControl, "no throw-template fact is issued"},
	{"typed_producer", nativeFamilyOwnerAuthority, "no typed-producer authority fact is issued"},
	{"value", nativeFamilyOwnerValue, "no general proved-value fact is issued; the constant value family covers proved constants only"},
}

// nativeFamilyImplementedLast is the final member of the closed implemented
// vocabulary. It is the one place the enum's extent is stated, so the read
// model below cannot fall behind a new member.
const nativeFamilyImplementedLast = nativePublicationFamilySendSafety

// NativeFamiliesImplemented is the closed set of family spellings the analyzer
// issues rows under, in the enum's own order. It is a live projection of the
// enum rather than a second list of names.
func NativeFamiliesImplemented() []string {
	names := make([]string, 0, int(nativeFamilyImplementedLast))
	for family := nativePublicationFamilyInvalid + 1; family <= nativeFamilyImplementedLast; family++ {
		name := family.String()
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names
}

// NativeFamilyImplemented answers whether the analyzer issues rows under one
// family spelling.
func NativeFamilyImplemented(name string) bool {
	if name == "" {
		return false
	}
	for family := nativePublicationFamilyInvalid + 1; family <= nativeFamilyImplementedLast; family++ {
		if family.String() == name {
			return true
		}
	}
	return false
}

// NativeFamiliesDeclaredNotImplemented is the register's read model, in family
// order.
func NativeFamiliesDeclaredNotImplemented() []NativeFamilyDeclared {
	rows := make([]NativeFamilyDeclared, len(nativeFamiliesDeclared))
	copy(rows, nativeFamiliesDeclared)
	sort.Slice(rows, func(left, right int) bool { return rows[left].Family < rows[right].Family })
	return rows
}

// NativeFamilyDeclaredNotImplemented answers one family's register entry.
func NativeFamilyDeclaredNotImplemented(name string) (NativeFamilyDeclared, bool) {
	for _, row := range nativeFamiliesDeclared {
		if row.Family == name {
			return row, true
		}
	}
	return NativeFamilyDeclared{}, false
}

// NativeFamilyStatus is one family's answer to "does the analyzer publish
// this".
type NativeFamilyStatus uint8

const (
	// NativeFamilyUnknown is a family the analyzer neither implements nor
	// declares. Naming one is a defect in the naming configuration, not a
	// pending fact.
	NativeFamilyUnknown NativeFamilyStatus = iota
	// NativeFamilyStatusImplemented is a family whose enum member issues rows.
	NativeFamilyStatusImplemented
	// NativeFamilyStatusDeclared is a family the register names with an owner.
	NativeFamilyStatusDeclared
)

// NativeFamilyAnswer classifies one named family against the closed enum and
// the register together. It is the single reading a consumer needs: a selector
// naming an unknown family is misconfigured, and one naming a declared family
// is waiting on the owner the register names.
func NativeFamilyAnswer(name string) (NativeFamilyStatus, NativeFamilyDeclared) {
	if NativeFamilyImplemented(name) {
		return NativeFamilyStatusImplemented, NativeFamilyDeclared{}
	}
	if row, declared := NativeFamilyDeclaredNotImplemented(name); declared {
		return NativeFamilyStatusDeclared, row
	}
	return NativeFamilyUnknown, NativeFamilyDeclared{}
}
