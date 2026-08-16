package empty

import (
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/analysis/domain/heap/allocation/catalog"
	"github.com/wippyai/go-lua/analysis/domain/heap/allocation/internal/source"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
)

// HotRule is Empty allocation's exact-read/one-carry receipt-native vertical.
// It retains only the Heap-owned issuer and the read receipt issued with the
// exact cold Rule cell; it does not retain a legacy Rule or Factor authority.
type HotRule struct {
	implementation *heapowner.RuleImplementation[source.Root]
	owner          *heapowner.HotOwner
	read           engine.Read[engine.OrderedCells[heapdomain.Value]]
	catalog        *allocationcatalog.Catalog
	schema         heapdomain.Schema
}

// BindHot attaches Empty's private transform, transfer, and evidence checker
// to its exact cold fragment through Heap's already-bound output Factor.
func BindHot(fragment *SchemaFragment, owner *heapowner.HotOwner, catalog *allocationcatalog.Catalog) (*HotRule, bool) {
	if fragment == nil || fragment.slot == nil || owner == nil || !owner.Schema().Valid() ||
		catalog == nil || !catalog.FencedToHeap(owner.Schema()) ||
		!fragment.semantic.Available() || !fragment.transform.Available() || !fragment.evidence.Available() ||
		!distinct(fragment.semantic, fragment.transform, fragment.evidence) {
		return nil, false
	}
	var runtimeRead engine.Read[engine.OrderedCells[heapdomain.Value]]
	implementation, read, ok := heapowner.BindExactReadAndCarryRule(owner, fragment.slot, fragment.read, fragment.carry, fragment.write, engine.HotRuleSpec[heapdomain.Value, source.Root]{
		// Root.New issued the complete cold classification receipt. The hot
		// member, transfer, and derivation paths use only its O(1) fence.
		OperandContent: func(operand source.Root) (source.Root, [32]byte, bool) {
			return emptyContent(owner.Schema(), operand)
		},
		Admission: engine.AdmitRuleByDerivation(fragment.evidence, hotEmptyChecker(owner, fragment.semantic, fragment.transform, &runtimeRead)),
		Transfer: func(access engine.Access[heapdomain.Value, source.Root]) bool {
			operand, operandOK := engine.Operand(access)
			if !operandOK {
				return false
			}
			return engine.Product(access, func(row engine.Row) bool {
				cells, cellsOK := engine.ReadValue(access, row, runtimeRead)
				if !cellsOK || cells.Count() != 1 {
					return false
				}
				predecessor, present, available := cells.At(0)
				if !available {
					return false
				}
				if !present {
					return engine.NoCandidate(access, row)
				}
				_, next, resultOK := emptyResult(owner.Schema(), operand, predecessor)
				return resultOK && engine.StageValue(access, row, next)
			})
		},
	}, engine.HotCarrySpec[heapdomain.Value, source.Root]{
		Apply: func(operand source.Root, predecessor heapdomain.Value) (heapdomain.Value, bool) {
			return owner.Schema().Age(predecessor, operand.Key())
		},
	})
	if !ok || implementation == nil {
		return nil, false
	}
	runtimeRead = read
	return &HotRule{implementation: implementation, owner: owner, read: read, catalog: catalog, schema: owner.Schema()}, true
}

type MountedIssuer struct {
	rule  *HotRule
	mount allocationcatalog.Mount
}

func (rule *HotRule) ForMount(module keyspace.ContentID) (MountedIssuer, bool) {
	if rule == nil || rule.catalog == nil {
		return MountedIssuer{}, false
	}
	mount, ok := rule.catalog.ForMount(module)
	return MountedIssuer{rule: rule, mount: mount}, ok && mount.OwnedBy(rule.catalog)
}

func (issuer MountedIssuer) ReceiptForOccurrence(id keyspace.ContentID) (source.Root, bool) {
	if issuer.rule == nil || !issuer.mount.OwnedBy(issuer.rule.catalog) {
		return source.Root{}, false
	}
	root, ok := issuer.mount.RootForOccurrence(id)
	return root, ok && root.Form() == source.FormEmpty && root.FencedTo(issuer.rule.catalogHeap())
}

// AttachMountedOccurrence seals HeapEmpty's exact read/carry/write incidence
// beneath the same mounted allocation proof that issued its operand.
func (rule *HotRule) AttachMountedOccurrence(assembly *engine.ReceiptAssembly, mountID, reusablePointID, occurrenceID keyspace.ContentID) (engine.BindingRuleRowRef, bool) {
	if rule == nil || rule.owner == nil || rule.catalog == nil || assembly == nil {
		return engine.BindingRuleRowRef{}, false
	}
	issuer, issuerOK := rule.ForMount(mountID)
	operand, operandOK := issuer.ReceiptForOccurrence(occurrenceID)
	implementation, implementationOK := heapowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	ref, refOK := rule.owner.Ref(operand.Key())
	if !issuerOK || !operandOK || !implementationOK || !refOK {
		return engine.BindingRuleRowRef{}, false
	}
	occurrence, occurrenceOK := assembly.AdmitMountedRuleOccurrence(mountedCapability(rule.implementation), mountID, reusablePointID, occurrenceID)
	if !occurrenceOK {
		return engine.BindingRuleRowRef{}, false
	}
	transaction, transactionOK := engine.BeginMountedRuleAdmission(assembly, implementation, occurrence, operand)
	if !transactionOK || !engine.AddExactRead(transaction, ref) || !transaction.AddCarry() || !engine.AddExactWrite(transaction, ref) {
		return engine.BindingRuleRowRef{}, false
	}
	queued := assembly.QueueMountedRuleFinalizer(mountedCapability(rule.implementation), func() bool {
		sourceReceipt, sourceOK := transaction.Seal()
		draft, draftOK := implementation.BeginReceiptRuleRow(sourceReceipt)
		readPart, readPartOK := implementation.ReceiptReadPart(sourceReceipt, 0)
		carryPart, carryPartOK := implementation.ReceiptCarryPart(sourceReceipt, 0)
		writePart, writePartOK := implementation.ReceiptWritePart(sourceReceipt, 0)
		if !sourceOK || !draftOK || !readPartOK || !carryPartOK || !writePartOK || !draft.AddRead(readPart) || !draft.AddCarry(carryPart) || !draft.AddWrite(writePart) {
			return false
		}
		_, added := assembly.AddRuleFromDraft(occurrence, draft)
		return added
	})
	return engine.BindingRuleRowRef{}, queued
}

// AttachMountedReceiptMember resolves both the committed graph member and its
// exact mounted allocation receipt internally.  No private Heap coordinate or
// operand capability escapes to the central artifact compiler.
func (rule *HotRule) AttachMountedReceiptMember(compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, mountID, reusablePointID, occurrenceID keyspace.ContentID) (*engine.ReceiptMember, bool) {
	if rule == nil || rule.owner == nil || graph == nil {
		return nil, false
	}
	issuer, issuerOK := rule.ForMount(mountID)
	operand, operandOK := issuer.ReceiptForOccurrence(occurrenceID)
	member, memberOK := graph.MountedRuleMember(mountedCapability(rule.implementation), mountID, reusablePointID, occurrenceID)
	implementation, implementationOK := heapowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	if !issuerOK || !operandOK || !memberOK || !implementationOK {
		return nil, false
	}
	return engine.AttachReceiptRuleMember(compilation, implementation, member, operand)
}

func (rule *HotRule) catalogHeap() heapdomain.Schema {
	if rule == nil || rule.catalog == nil {
		return heapdomain.Schema{}
	}
	// Every operand is re-fenced by the exact HotOwner schema in content and
	// evidence. Retain the same schema explicitly for issuance.
	return rule.schema
}

// Implementation returns Heap owner's opaque receipt issuer only after the
// exact SchemaBinding seals.
func (rule *HotRule) Implementation() (*heapowner.RuleImplementation[source.Root], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := heapowner.ResolveRuleImplementation(rule.implementation)
	return rule.implementation, ok
}

func emptyContent(schema heapdomain.Schema, operand source.Root) (source.Root, [32]byte, bool) {
	id, ok := operand.ID()
	if !ok || operand.Form() != source.FormEmpty || !operand.FencedTo(schema) {
		return source.Root{}, [32]byte{}, false
	}
	return operand, [32]byte(id), true
}

func emptyResult(schema heapdomain.Schema, operand source.Root, predecessor heapdomain.Value) (heapdomain.Key, heapdomain.Value, bool) {
	if !schema.Valid() || operand.Form() != source.FormEmpty || !operand.FencedTo(schema) || predecessor.IsBottom() {
		return heapdomain.Key{}, heapdomain.Value{}, false
	}
	shape := heapdomain.ShapeIneligible
	if operand.Kind() == heapdomain.AllocationTable {
		shape = heapdomain.ShapeEligible
	}
	none, noneOK := schema.ContainmentNone()
	initializer, initOK := schema.BeginObject(shape, heapdomain.FrozenMutable, none)
	fresh, freshOK := initializer.Finish()
	if !noneOK || !initOK || !freshOK {
		return heapdomain.Key{}, heapdomain.Value{}, false
	}
	next, nextOK := schema.Create(predecessor, operand.Key(), fresh)
	return operand.Key(), next, nextOK
}

func hotEmptyChecker(owner *heapowner.HotOwner, semantic, transform engine.SemanticKey, read *engine.Read[engine.OrderedCells[heapdomain.Value]]) engine.RuleDerivationChecker[heapdomain.Value, source.Root] {
	return func(derivation engine.RuleDerivation[heapdomain.Value, source.Root]) (engine.RuleEvidence, bool) {
		if owner == nil || read == nil || !owner.Schema().Valid() || derivation.Rule() != semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 1 {
			return engine.RuleEvidence{}, false
		}
		operand, operandOK := derivation.Operand()
		id, idOK := operand.ID()
		ref, refOK := owner.Ref(operand.Key())
		input, inputOK := derivation.InputAt(0)
		if !operandOK || !idOK || operand.Form() != source.FormEmpty || !operand.FencedTo(owner.Schema()) || !refOK || !inputOK || input.Guard().Empty() || !derivation.OperandContentMatches([32]byte(id)) || !engine.DerivationReadMatchesRef(derivation, *read, ref) {
			return engine.RuleEvidence{}, false
		}
		for index := 0; index < derivation.DispositionCount(); index++ {
			disposition, dispositionOK := derivation.DispositionAt(index)
			if !dispositionOK || disposition.Guard().Empty() {
				return engine.RuleEvidence{}, false
			}
			cells, cellsOK := engine.DerivationDispositionReadValue(derivation, disposition, *read)
			if !cellsOK || cells.Count() != 1 {
				return engine.RuleEvidence{}, false
			}
			predecessor, present, available := cells.At(0)
			if !available {
				return engine.RuleEvidence{}, false
			}
			if !present {
				_, transformed := disposition.CarryTransform()
				if disposition.Kind() != engine.RuleDispositionNoCandidate || transformed || disposition.TransformOnly() || disposition.TargetCount() != 0 {
					return engine.RuleEvidence{}, false
				}
				continue
			}
			_, next, nextOK := emptyResult(owner.Schema(), operand, predecessor)
			target, targetOK := disposition.TargetAt(0)
			actual, actualOK := disposition.Value()
			carry, transformed := disposition.CarryTransform()
			if !nextOK || !targetOK || !actualOK || disposition.Kind() != engine.RuleDispositionStaged || disposition.TransformOnly() || !transformed || carry != transform || disposition.TargetCount() != 1 || !engine.TargetMatchesRef(target, ref) || !owner.Schema().Domain().Equal(actual, next) {
				return engine.RuleEvidence{}, false
			}
		}
		return derivation.Accept()
	}
}
