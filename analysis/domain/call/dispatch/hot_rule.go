package dispatch

import (
	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
)

type dispatchAtomReceipt struct {
	target   calldomain.Target
	known    bool
	callable bool
}

// dispatchReceipt is the sealed hot operand for one mounted Call application.
// Its atom table is built once from exact Link/Boundary/Heap/Pack authorities
// while issuing the receipt. Runtime callbacks consume only this table, the
// sealed Value schema, and owner-fenced Call targets.
type dispatchReceipt struct {
	binding    *engine.SchemaBinding
	values     *valuedomain.Schema
	algebra    *calldomain.Algebra
	key        calldomain.Key
	coordinate valuedomain.Coordinate
	id         [32]byte
	atoms      map[valuedomain.Atom]dispatchAtomReceipt
	sealed     bool
}

func (receipt dispatchReceipt) valid() bool {
	if receipt.algebra == nil {
		return false
	}
	applicationID, idOK := receipt.key.ApplicationID()
	_, mountedOK := receipt.algebra.MountedCallForApplication(applicationID)
	return receipt.sealed && receipt.binding != nil && receipt.binding.Sealed() && receipt.values != nil && receipt.algebra.Valid() && receipt.key.Valid() && receipt.key.IsApplication() && receipt.coordinate.Valid() && receipt.id != [32]byte{} && receipt.atoms != nil && idOK && applicationID.Available() && mountedOK
}

func dispatchReceiptContent(receipt dispatchReceipt) (dispatchReceipt, [32]byte, bool) {
	return receipt, receipt.id, receipt.valid()
}

func (rule *HotRule) receipt(applicationID keyspace.ContentID) (dispatchReceipt, bool) {
	if rule == nil || rule.values == nil || rule.calls == nil || !rule.heaps.Valid() || rule.packs == nil {
		return dispatchReceipt{}, false
	}
	bound, ok := newSite(rule.calls.Algebra(), rule.values.Schema(), rule.heaps, rule.packs, applicationID)
	if !ok {
		return dispatchReceipt{}, false
	}
	values := rule.values.Schema()
	if values == nil {
		return dispatchReceipt{}, false
	}
	coordinate, coordinateOK := bound.valueCoordinate()
	if !coordinateOK {
		return dispatchReceipt{}, false
	}
	atoms := make(map[valuedomain.Atom]dispatchAtomReceipt)
	if !values.AdmitsCoordinate(coordinate, values.Bottom()) {
		return dispatchReceipt{}, false
	}
	issuanceOK := true
	if !values.VisitSupport(values.Top(), func(atom valuedomain.Atom) {
		capability, known, callable := dispatchAtom(bound, atom)
		if !values.OwnsAtom(atom) || (known && !rule.calls.Algebra().OwnsTarget(capability)) {
			issuanceOK = false
			return
		}
		atoms[atom] = dispatchAtomReceipt{target: capability, known: known, callable: callable}
	}) {
		return dispatchReceipt{}, false
	}
	id, idOK := bound.contentID()
	key, keyOK := bound.callKey()
	applicationID, applicationIDOK := key.ContentID()
	if !idOK || !keyOK || !coordinateOK || !applicationIDOK || !applicationID.Available() {
		return dispatchReceipt{}, false
	}
	if !issuanceOK {
		return dispatchReceipt{}, false
	}
	receipt := dispatchReceipt{binding: rule.binding, values: values, algebra: rule.calls.Algebra(), key: key, coordinate: coordinate, id: [32]byte(id), atoms: atoms, sealed: true}
	return receipt, receipt.valid()
}

func reduceReceipt(receipt dispatchReceipt, callee valuedomain.Value) (calldomain.Value, bool) {
	if !receipt.valid() {
		return calldomain.Value{}, false
	}
	if callee.IsTop() {
		return receipt.algebra.Top(), true
	}
	if callee.IsBottom() {
		return receipt.algebra.DispatchValue(receipt.key, nil, false)
	}
	targets := make([]calldomain.Target, 0, 2)
	unknown := false
	if !receipt.values.VisitAtoms(callee, func(atom valuedomain.Atom) bool {
		mapped, ok := receipt.atoms[atom]
		if !ok {
			return false
		}
		if mapped.known {
			targets = append(targets, mapped.target)
		}
		if mapped.callable {
			unknown = true
		}
		return true
	}) {
		return calldomain.Value{}, false
	}
	return receipt.algebra.DispatchValue(receipt.key, targets, unknown)
}

// HotRule is Call dispatch's receipt-native exact-read Rule binder. Its
// callbacks never retain or inspect Link, Flow, Target, or Pack topology.
type HotRule struct {
	binding        *engine.SchemaBinding
	fragment       *SchemaFragment
	values         *valueowner.HotOwner
	calls          *callowner.HotOwner
	heaps          heapdomain.Schema
	packs          *packdomain.Schema
	read           engine.Read[engine.OrderedCells[valuedomain.Value]]
	implementation *callowner.HeterogeneousRuleImplementation[valuedomain.Value, dispatchReceipt]
	receipts       map[calldomain.MountedCall]dispatchReceipt
	receiptsSealed bool
}

// BindHot binds the closed Value-read/Call-write dispatch lane through typed
// FactorRefs. Heap and Pack are used only while issuing application receipts;
// no topology scan occurs from the sealed engine callbacks.
func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, values *valueowner.HotOwner, calls *callowner.HotOwner, heaps heapdomain.Schema, packs *packdomain.Schema) (*HotRule, bool) {
	if binding == nil || fragment == nil || values == nil || !values.MatchesBinding(binding) || calls == nil || !calls.MatchesBinding(binding) || values.Schema() == nil || calls.Algebra() == nil || !calls.Algebra().Valid() || !heaps.Valid() || packs == nil || !fragment.semantic.Available() || !fragment.evidence.Available() {
		return nil, false
	}
	hot := &HotRule{binding: binding, fragment: fragment, values: values, calls: calls, heaps: heaps, packs: packs}
	implementation, runtimeRead, ok := callowner.BindHeterogeneousExactReadRule(calls, fragment.slot, fragment.read, fragment.value, fragment.write, engine.HotRuleSpec[calldomain.Value, dispatchReceipt]{
		OperandContent: dispatchReceiptContent,
		Admission:      engine.AdmitRuleByDerivation(fragment.evidence, hotDispatchChecker(hot)),
		Transfer: func(access engine.Access[calldomain.Value, dispatchReceipt]) bool {
			receipt, receiptOK := engine.Operand(access)
			if !receiptOK || !hot.acceptsReceipt(receipt) {
				return false
			}
			return engine.Product(access, func(row engine.Row) bool {
				cells, readOK := engine.ReadValue(access, row, hot.read)
				if !readOK || cells.Count() != 1 {
					return false
				}
				fact, present, available := cells.At(0)
				if !available {
					return false
				}
				if !present {
					return engine.NoCandidate(access, row)
				}
				result, resultOK := reduceReceipt(receipt, fact)
				return resultOK && engine.StageValue(access, row, result)
			})
		},
	})
	if !ok {
		return nil, false
	}
	hot.read = runtimeRead
	hot.implementation = implementation
	return hot, true
}

// sealReceiptCatalog issues every ordinary-call application witness once for
// this sealed Link/Binding pair. The receipt map is keyed by Call's opaque
// mounted row; occurrence/application inverses remain solely in Call. This is
// the one cold seal pass; hot lookup never reopens Program or Flow.
func (rule *HotRule) sealReceiptCatalog() bool {
	if rule == nil || rule.binding == nil || !rule.binding.Sealed() || rule.calls == nil || rule.calls.Algebra() == nil {
		return false
	}
	if rule.receiptsSealed {
		return rule.receipts != nil
	}
	receipts := make(map[calldomain.MountedCall]dispatchReceipt, rule.calls.Algebra().MountedCallCount())
	if rule.values.Schema() == nil {
		return false
	}
	algebra := rule.calls.Algebra()
	for index := 0; index < algebra.MountedCallCount(); index++ {
		mounted, mountedOK := algebra.MountedCallAtHandle(index)
		applicationID, occurrenceID, module, _, _, identityOK := algebra.MountedCallIdentity(mounted)
		inverse, inverseOK := algebra.MountedCallForOccurrence(module, occurrenceID)
		if !mountedOK || !identityOK || !applicationID.Available() || !module.Available() || !occurrenceID.Available() || !inverseOK || inverse != mounted || !algebra.OwnsMountedModule(module) {
			return false
		}
		receipt, ok := rule.receipt(applicationID)
		if !ok || !receipt.valid() {
			return false
		}
		if _, duplicate := receipts[mounted]; duplicate {
			return false
		}
		receipts[mounted] = receipt
	}
	if len(receipts) != algebra.MountedCallCount() {
		return false
	}
	rule.receipts = receipts
	rule.receiptsSealed = true
	return true
}

// SealOccurrenceReceipts closes Call dispatch's Link-local artifact inverse
// after its shared binding seals.  It is deliberately explicit so Program
// binding owns the cold lifecycle rather than the first engine lookup.
func (rule *HotRule) SealOccurrenceReceipts() bool {
	return rule != nil && rule.sealReceiptCatalog()
}

// Receipt issues the complete cold application witness from the sealed
// Link-local inverse. The first call seals the immutable catalog; subsequent
// calls are O(1) by ApplicationID and never reopen Program/Flow.
func (rule *HotRule) Receipt(applicationID keyspace.ContentID) (dispatchReceipt, bool) {
	if rule == nil || rule.binding == nil || !rule.binding.Sealed() {
		return dispatchReceipt{}, false
	}
	if !rule.sealReceiptCatalog() {
		return dispatchReceipt{}, false
	}
	if !applicationID.Available() || rule.receipts == nil || rule.calls == nil || rule.calls.Algebra() == nil {
		return dispatchReceipt{}, false
	}
	mounted, mountedOK := rule.calls.Algebra().MountedCallForApplication(applicationID)
	if !mountedOK {
		return dispatchReceipt{}, false
	}
	receipt, ok := rule.receipts[mounted]
	return receipt, ok && receipt.valid()
}

// MountedIssuer is Call dispatch's exact mount-scoped artifact substitution
// authority.  It cannot be replayed against an equal ModuleKey from another
// Link or binding because the retained row is pointer-fenced to its HotRule.
type MountedIssuer struct {
	rule   *HotRule
	module keyspace.ContentID
}

// ForMount returns the exact issuer for one mounted Program ModuleKey.
func (rule *HotRule) ForMount(module keyspace.ContentID) (MountedIssuer, bool) {
	if rule == nil || !module.Available() || !rule.receiptsSealed || rule.receipts == nil || rule.calls == nil || rule.calls.Algebra() == nil || !rule.calls.Algebra().OwnsMountedModule(module) {
		return MountedIssuer{}, false
	}
	issuer := MountedIssuer{rule: rule, module: module}
	return issuer, issuer.valid()
}

func (issuer MountedIssuer) valid() bool {
	return issuer.rule != nil && issuer.rule.binding != nil && issuer.rule.binding.Sealed() &&
		issuer.rule.receiptsSealed && issuer.rule.receipts != nil && issuer.rule.calls != nil && issuer.rule.calls.Algebra() != nil &&
		issuer.module.Available() && issuer.rule.calls.Algebra().OwnsMountedModule(issuer.module)
}

// ReceiptForOccurrence returns the exact preissued dispatch receipt for one
// artifact Call.ContextID.  It is an O(1) mounted map lookup and never
// consults Program, Flow, Boundary, or the engine.
func (issuer MountedIssuer) ReceiptForOccurrence(id keyspace.ContentID) (dispatchReceipt, bool) {
	if !issuer.valid() || !id.Available() {
		return dispatchReceipt{}, false
	}
	mounted, mountedOK := issuer.rule.calls.Algebra().MountedCallForOccurrence(issuer.module, id)
	if !mountedOK {
		return dispatchReceipt{}, false
	}
	receipt, ok := issuer.rule.receipts[mounted]
	return receipt, ok && issuer.rule.acceptsReceipt(receipt)
}

// ApplicationIDForOccurrence exposes the detached application identity paired
// with an artifact call occurrence.
func (issuer MountedIssuer) ApplicationIDForOccurrence(id keyspace.ContentID) (keyspace.ContentID, bool) {
	receipt, ok := issuer.ReceiptForOccurrence(id)
	if !ok {
		return keyspace.ContentID{}, false
	}
	return receipt.key.ApplicationID()
}

// AttachMountedOccurrence admits one artifact Call dispatch row using the
// preissued mounted receipt and exact Value/Call owner surfaces.
func (rule *HotRule) AttachMountedOccurrence(assembly *engine.ReceiptAssembly, mountID, reusablePointID, occurrenceID keyspace.ContentID) (engine.BindingRuleRowRef, bool) {
	if rule == nil || rule.implementation == nil || rule.values == nil || rule.calls == nil || assembly == nil {
		return engine.BindingRuleRowRef{}, false
	}
	issuer, ok := rule.ForMount(mountID)
	if !ok {
		return engine.BindingRuleRowRef{}, false
	}
	receipt, ok := issuer.ReceiptForOccurrence(occurrenceID)
	if !ok {
		return engine.BindingRuleRowRef{}, false
	}
	capability, capabilityOK := rule.implementation.MountedCapability()
	if !capabilityOK {
		return engine.BindingRuleRowRef{}, false
	}
	occurrence, ok := assembly.AdmitMountedRuleOccurrence(capability, mountID, reusablePointID, occurrenceID)
	if !ok {
		return engine.BindingRuleRowRef{}, false
	}
	implementation, implementationOK := callowner.ResolveHeterogeneousRuleImplementation(rule.implementation)
	transaction, ok := engine.BeginMountedRuleAdmission(assembly, implementation, occurrence, receipt)
	if !implementationOK || !ok {
		return engine.BindingRuleRowRef{}, false
	}
	readRef, readOK := rule.values.Ref(receipt.coordinate)
	writeRef, writeOK := rule.calls.Ref(receipt.key)
	if !readOK || !writeOK || !engine.AddExactRead(transaction, readRef) || !engine.AddExactWrite(transaction, writeRef) {
		return engine.BindingRuleRowRef{}, false
	}
	queued := assembly.QueueMountedRuleFinalizer(capability, func() bool {
		sourceReceipt, sourceOK := transaction.Seal()
		if !sourceOK {
			return false
		}
		draft, draftOK := implementation.BeginReceiptRuleRow(sourceReceipt)
		readPart, readPartOK := implementation.ReceiptReadPart(sourceReceipt, 0)
		writePart, writePartOK := implementation.ReceiptWritePart(sourceReceipt, 0)
		if !draftOK || !readPartOK || !writePartOK || !draft.AddRead(readPart) || !draft.AddWrite(writePart) {
			return false
		}
		_, added := assembly.AddRuleFromDraft(occurrence, draft)
		return added
	})
	return engine.BindingRuleRowRef{}, queued
}

// AttachMountedReceiptMember resolves and attaches one exact post-commit
// dispatch member from the mounted graph directory.
func (rule *HotRule) AttachMountedReceiptMember(compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, mountID, reusablePointID, occurrenceID keyspace.ContentID) (*engine.ReceiptMember, bool) {
	if rule == nil || compilation == nil || graph == nil || rule.implementation == nil {
		return nil, false
	}
	capability, capabilityOK := rule.implementation.MountedCapability()
	if !capabilityOK {
		return nil, false
	}
	member, ok := graph.MountedRuleMember(capability, mountID, reusablePointID, occurrenceID)
	if !ok {
		return nil, false
	}
	issuer, ok := rule.ForMount(mountID)
	if !ok {
		return nil, false
	}
	operand, ok := issuer.ReceiptForOccurrence(occurrenceID)
	if !ok {
		return nil, false
	}
	implementation, ok := callowner.ResolveHeterogeneousRuleImplementation(rule.implementation)
	if !ok {
		return nil, false
	}
	return engine.AttachReceiptRuleMember(compilation, implementation, member, operand)
}

func (rule *HotRule) acceptsReceipt(receipt dispatchReceipt) bool {
	if rule == nil || rule.binding == nil || rule.values == nil || rule.calls == nil || !receipt.valid() || receipt.binding != rule.binding {
		return false
	}
	if !rule.receiptsSealed || rule.receipts == nil {
		return false
	}
	algebra := rule.calls.Algebra()
	if algebra == nil || !algebra.Valid() {
		return false
	}
	applicationID, applicationOK := receipt.key.ApplicationID()
	mounted, mountedOK := algebra.MountedCallForApplication(applicationID)
	issued, issuedOK := rule.receipts[mounted]
	if !applicationOK || !mountedOK || !issuedOK || issued.id != receipt.id || issued.key != receipt.key {
		return false
	}
	values := rule.values.Schema()
	return values != nil && algebra != nil && receipt.values == values && receipt.algebra == algebra && algebra.OwnsKey(receipt.key) && values.AdmitsCoordinate(receipt.coordinate, values.Bottom())
}

func hotDispatchChecker(rule *HotRule) engine.RuleDerivationChecker[calldomain.Value, dispatchReceipt] {
	return func(derivation engine.RuleDerivation[calldomain.Value, dispatchReceipt]) (engine.RuleEvidence, bool) {
		if rule == nil || rule.fragment == nil || derivation.Rule() != rule.fragment.semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() == 0 {
			return engine.RuleEvidence{}, false
		}
		receipt, receiptOK := derivation.Operand()
		if !receiptOK || !rule.acceptsReceipt(receipt) || !derivation.OperandContentMatches(receipt.id) {
			return engine.RuleEvidence{}, false
		}
		input, inputOK := derivation.InputAt(0)
		if !inputOK || input.Guard().Empty() {
			return engine.RuleEvidence{}, false
		}
		for index := 0; index < derivation.DispositionCount(); index++ {
			disposition, dispositionOK := derivation.DispositionAt(index)
			if !dispositionOK || disposition.Guard().Empty() {
				return engine.RuleEvidence{}, false
			}
			cells, cellsOK := engine.DerivationDispositionReadValue(derivation, disposition, rule.read)
			if !cellsOK || cells.Count() != 1 {
				return engine.RuleEvidence{}, false
			}
			fact, present, available := cells.At(0)
			if !available {
				return engine.RuleEvidence{}, false
			}
			if !present {
				if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
					return engine.RuleEvidence{}, false
				}
				continue
			}
			expected, expectedOK := reduceReceipt(receipt, fact)
			if !expectedOK || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 {
				return engine.RuleEvidence{}, false
			}
			actual, actualOK := disposition.Value()
			if !actualOK || !rule.calls.Algebra().Equal(actual, expected) {
				return engine.RuleEvidence{}, false
			}
		}
		return derivation.Accept()
	}
}
