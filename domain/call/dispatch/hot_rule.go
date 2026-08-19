package dispatch

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
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

func (rule *HotRule) receipt(applicationID identity.ContentID) (dispatchReceipt, bool) {
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
	receipts       []dispatchReceipt
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
			if !receiptOK {
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
	}, func(receipt dispatchReceipt) (uint64, bool) {
		index, ok := values.Schema().CoordinateIndex(receipt.coordinate)
		return uint64(index), ok
	}, func(receipt dispatchReceipt) (uint64, bool) {
		index, ok := calls.Algebra().KeyIndex(receipt.key)
		return uint64(index), ok && index >= 0
	})
	if !ok {
		return nil, false
	}
	hot.read = runtimeRead
	hot.implementation = implementation
	if !implementation.InstallOperandResolver(hot.resolveOperand) {
		return nil, false
	}
	return hot, true
}

// SealProgramRule is this typed rule's schema registration.
func SealProgramRule(rule *HotRule) (engine.ProgramRule, bool) {
	if rule == nil {
		return engine.ProgramRule{}, false
	}
	implementation, ok := callowner.ResolveHeterogeneousRuleImplementation(rule.implementation)
	if !ok {
		return engine.ProgramRule{}, false
	}
	return engine.SealProgramRule(implementation)
}

// resolveOperand addresses the sealed receipt plane directly. Call's mounted
// order is the ordinal space the catalog was filled in, so one module-scoped
// occurrence inverse names the row; no mount-scoped issuer stands between.
func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (dispatchReceipt, bool) {
	if rule == nil || !rule.receiptsSealed || rule.calls == nil || rule.calls.Algebra() == nil {
		return dispatchReceipt{}, false
	}
	ordinal, ok := rule.calls.Algebra().MountedCallOrdinalForOccurrence(coords.Mount, coords.Occurrence)
	if !ok || ordinal < 0 || ordinal >= len(rule.receipts) {
		return dispatchReceipt{}, false
	}
	return rule.receipts[ordinal], true
}

// sealReceiptCatalog issues every ordinary-call application witness once for
// this sealed Link/Binding pair. The receipt plane is dense in Call's
// canonical mounted order; occurrence/application inverses remain solely in
// Call. This is the one cold seal pass; hot lookup never reopens Program or
// Flow.
func (rule *HotRule) sealReceiptCatalog() bool {
	if rule == nil || rule.binding == nil || !rule.binding.Sealed() || rule.calls == nil || rule.calls.Algebra() == nil {
		return false
	}
	if rule.receiptsSealed {
		return rule.receipts != nil
	}
	if rule.values.Schema() == nil {
		return false
	}
	algebra := rule.calls.Algebra()
	receipts := make([]dispatchReceipt, algebra.MountedCallCount())
	for index := range receipts {
		mounted, mountedOK := algebra.MountedCallAtHandle(index)
		applicationID, occurrenceID, module, _, _, identityOK := algebra.MountedCallIdentity(mounted)
		ordinal, ordinalOK := algebra.MountedCallOrdinalForOccurrence(module, occurrenceID)
		if !mountedOK || !identityOK || !applicationID.Available() || !module.Available() || !occurrenceID.Available() || !ordinalOK || ordinal != index || !algebra.OwnsMountedModule(module) {
			return false
		}
		receipt, ok := rule.receipt(applicationID)
		if !ok || !receipt.valid() {
			return false
		}
		receipts[index] = receipt
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

func hotDispatchChecker(rule *HotRule) engine.RuleDerivationChecker[calldomain.Value, dispatchReceipt] {
	return func(derivation engine.RuleDerivation[calldomain.Value, dispatchReceipt]) (engine.RuleEvidence, bool) {
		if rule == nil || rule.fragment == nil || derivation.Rule() != rule.fragment.semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() == 0 {
			return engine.RuleEvidence{}, false
		}
		receipt, receiptOK := derivation.Operand()
		if !receiptOK || !derivation.OperandContentMatches(receipt.id) {
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
