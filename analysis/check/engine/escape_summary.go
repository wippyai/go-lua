package engine

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// escapeSummaryForExport computes a per-parameter escape disposition for each
// exported function of the evaluated module. It is a per-function summary: each
// exported prototype is evaluated once with its formals seeded as distinct
// allocation identities, and the disposition is read from the placement facts
// that evaluation publishes. The result is pure data attached to engine.Result;
// it changes no diagnostic, placement plan, or module allocation graph, so an
// uncalled export never becomes a caller-visible module allocation.
//
// The pass is fail-open on data only: any body that cannot be seeded or
// evaluated contributes no entry, and a panic in a downstream kernel is absorbed
// so the summary can never disturb the module verdict.
func (l *lexicalEvaluator) escapeSummaryForExport(closure equation.OutputClosure) (summary map[string][]signature.ParamRelation, allocatedReturns map[string]bool) {
	defer func() {
		if recover() != nil {
			summary, allocatedReturns = nil, nil
		}
	}()
	if l == nil {
		return nil, nil
	}
	partition, err := equation.PartitionFromClosuresWithGuards(nil, closure)
	if err != nil {
		return nil, nil
	}
	exports := exportedFunctionHandles(closure.Values)
	if len(exports) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(exports))
	for name := range exports {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		child, found := l.byPrototype[exports[name].Prototype]
		if !found {
			continue
		}
		relations, allocatedReturn, ok := l.escapeRelationsForPrototype(child, partition)
		if !ok {
			continue
		}
		if summary == nil {
			summary = make(map[string][]signature.ParamRelation)
		}
		summary[name] = relations
		if allocatedReturn {
			if allocatedReturns == nil {
				allocatedReturns = make(map[string]bool)
			}
			allocatedReturns[name] = true
		}
	}
	return summary, allocatedReturns
}

// exportedFunctionHandles reads the returned-table member closure capabilities
// the publication kernel already published for return slot zero. It maps each
// exported member name to its closure handle without re-deriving the return
// shape from source.
func exportedFunctionHandles(values []equation.Fact) map[string]closureHandle {
	const prefix = "return-member-closure/"
	handles := make(map[string]closureHandle)
	for _, fact := range values {
		if !strings.HasPrefix(fact.Key, prefix) {
			continue
		}
		parts := strings.Split(fact.Key, "/")
		if len(parts) != 4 || parts[2] != "00000000" {
			continue
		}
		var wire memberClosureWire
		if json.Unmarshal(fact.Value, &wire) != nil || !validClosureHandle(wire.Handle) {
			continue
		}
		name := strings.TrimPrefix(wire.Suffix, ".")
		if name == "" || strings.Contains(name, ".") {
			continue
		}
		handles[name] = wire.Handle
	}
	return handles
}

// escapeRelationsForPrototype evaluates one exported body with its formals
// seeded as allocation identities and classifies each formal from the published
// placement facts. The same evaluation also decides whether the body returns a
// graph of its own. It returns ok=false when the body cannot be seeded (a
// vararg or `any` formal, an unreconstructable capture) or its evaluation fails.
func (l *lexicalEvaluator) escapeRelationsForPrototype(child front.Compilation, partition equation.Partition) ([]signature.ParamRelation, bool, bool) {
	entry, identities, ok := l.escapeChildEntry(child, partition)
	if !ok {
		return nil, false, false
	}
	outcome, _, err := l.evaluate(child, entry)
	if err != nil {
		return nil, false, false
	}
	return classifyEscapeParameters(child, outcome.Values, identities), allocatedReturnGraph(outcome.Values, identities), true
}

// allocatedReturnGraph reports whether the evaluated body returns a graph that
// did not already exist before the call. The publication kernel publishes the
// owned return escape on the allocation a return operand resolves to, so a
// unique owned root is the returned graph. A returned capture or module table
// carries no allocation in this evaluation and therefore leaves the body
// unproven; a returned formal resolves to its own seeded parameter identity and
// is excluded here, because the caller already owns that graph.
func allocatedReturnGraph(values []equation.Fact, identities map[int]string) bool {
	root, ok := placementReturnedRoot(values)
	if !ok {
		return false
	}
	for _, identity := range identities {
		if identity == root {
			return false
		}
	}
	return true
}

// escapeChildEntry seeds every formal with its declared shape and, for a formal
// whose declared type is not a closed immutable scalar, a distinct synthetic
// allocation identity. A store, send, opaque re-pass, or return of that formal
// then publishes a placement fact keyed to its identity. A closed scalar formal
// is value-copied and cannot escape by reference, so it carries no allocation
// and stays a borrow.
func (l *lexicalEvaluator) escapeChildEntry(child front.Compilation, partition equation.Partition) ([]byte, map[int]string, bool) {
	if child.WIR == nil || len(child.Boundary.Parameters) == 0 {
		return nil, nil, false
	}
	formalSeeds, ok := declaredFormalWitnessSeeds(child)
	if !ok {
		return nil, nil, false
	}
	placementSeeds := make([]entryPlacementSeed, 0, len(child.Boundary.Parameters))
	identities := make(map[int]string, len(child.Boundary.Parameters))
	for index, parameter := range child.Boundary.Parameters {
		declared := unwrap.Alias(child.WIR.Type(parameter.Type))
		if placementClosedScalarType(declared) {
			continue
		}
		identity := escapeParameterIdentity(child, index)
		placementSeeds = append(placementSeeds, entryPlacementSeed{
			Term: boundaryTerm(parameter.Symbol),
			Allocation: placementAllocationFact{
				Identity: identity,
				Result:   "placement/escape-param/" + base64.RawURLEncoding.EncodeToString([]byte(identity)),
				Kind:     "lua.table",
				Complete: true,
			},
		})
		identities[index] = identity
	}
	seeds, closureSeeds, memberClosureSeeds, tableIdentitySeeds, memberCellSeeds, admitted := l.childEntrySeedSet(child, formalSeeds, partition, true, false, false)
	if !admitted {
		return nil, nil, false
	}
	entry, err := encodeChildEntryWithPlacementCapabilities(seeds, closureSeeds, memberClosureSeeds, tableIdentitySeeds, memberCellSeeds, placementSeeds)
	if err != nil {
		return nil, nil, false
	}
	return entry, identities, true
}

func escapeParameterIdentity(child front.Compilation, index int) string {
	return "escape-param/" + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%x/%d", child.Body, index)))
}

// classifyEscapeParameters reads the disposition of each formal from the
// placement facts of the evaluated body. A formal is a borrow only when the
// body published no owned event, no shared event, no opaque-call blocker, and no
// return escape for its identity. An unseeded (closed-scalar) formal is a borrow
// by construction.
func classifyEscapeParameters(child front.Compilation, values []equation.Fact, identities map[int]string) []signature.ParamRelation {
	parsed := parsePublishedPlacement(values)
	// Containment carries an escaping container's disposition to the elements it
	// holds, so a formal stored behind an intermediate table is not mistaken for
	// a borrow.
	propagatePublishedPlacement(parsed)
	opKinds := childOperationKinds(child.Artifact)
	relations := make([]signature.ParamRelation, 0, len(child.Boundary.Parameters))
	for index := range child.Boundary.Parameters {
		relation := signature.ParamRelation{Param: index, EscapeClass: signature.EscapeBorrow, PlacementConsequence: signature.PlacementConsequenceKeep}
		if identity, seeded := identities[index]; seeded {
			relation = classifyEscapeIdentity(index, identity, parsed, values, opKinds, identities)
		}
		relations = append(relations, relation)
	}
	return relations
}

// storeContainerFormal names the formal a stored formal is stored into. A store
// boundary retains both the stored graph and the container receiving it under
// one operation and owns the stored graph alone, so the container is the single
// other seeded formal that same operation retains without owning. An absent or
// ambiguous container leaves the relation ownerless and the consumer reads the
// store as an escaping root.
func storeContainerFormal(param int, ownedOps []string, values []equation.Fact, identities map[int]string, opKinds map[string]string) (int, bool) {
	boundaries := make(map[string]bool)
	for _, operation := range ownedOps {
		if opKinds[operation] != "publication" {
			boundaries[operation] = true
		}
	}
	container, candidates := 0, 0
	for other, identity := range identities {
		if other == param || len(placementProvenOperations(values, placementEventPrefix, identity, placementEventOwned)) != 0 {
			continue
		}
		for _, operation := range placementProvenOperations(values, placementContractPrefix, identity, "retain") {
			if boundaries[operation] {
				container, candidates = other, candidates+1
				break
			}
		}
	}
	return container, candidates == 1
}

func classifyEscapeIdentity(index int, identity string, parsed publishedPlacementFacts, values []equation.Fact, opKinds map[string]string, identities map[int]string) signature.ParamRelation {
	relation := signature.ParamRelation{Param: index, EscapeClass: signature.EscapeBorrow, PlacementConsequence: signature.PlacementConsequenceKeep}
	events := parsed.events[identity]
	ownedOps := placementProvenOperations(values, placementEventPrefix, identity, placementEventOwned)
	storeOwned, returnOwned := false, false
	for _, operation := range ownedOps {
		if opKinds[operation] == "publication" {
			returnOwned = true
		} else {
			storeOwned = true
		}
	}
	// An owned event reaching the identity only through containment propagation
	// carries no operation of its own; the formal escaped inside an owned
	// container, which is a store.
	containedOwned := events[placementEventOwned] && len(ownedOps) == 0
	switch {
	case events[placementEventShared]:
		relation.EscapeClass = signature.EscapeSend
		relation.PlacementConsequence = signature.PlacementConsequenceSharedHeap
	case storeOwned || containedOwned:
		relation.EscapeClass = signature.EscapeStore
		relation.PlacementConsequence = signature.PlacementConsequenceOwnedHeap
		relation.StoredInto, relation.HasStoredInto = storeContainerFormal(index, ownedOps, values, identities, opKinds)
	case returnOwned:
		relation.EscapeClass = signature.EscapeExport
		relation.ThroughReturn = true
		relation.PlacementConsequence = signature.PlacementConsequenceKeep
	case parsed.blockers[identity]["opaque-call"] && placementBlockerStands(identity, "opaque-call", parsed.blockerOperations, parsed.contracts):
		relation.EscapeClass = signature.EscapeOpaque
		relation.PlacementConsequence = signature.PlacementConsequenceOwnedHeap
	}
	return relation
}

// placementProvenOperations returns the operations that published a proven
// placement fact for identity under one boundary of the named fact family. The
// operation kind distinguishes a store from a return escape without inspecting
// source.
func placementProvenOperations(values []equation.Fact, family, identity, boundary string) []string {
	prefix := family + base64.RawURLEncoding.EncodeToString([]byte(identity)) + "/" + boundary + "/"
	var operations []string
	for _, fact := range values {
		if string(fact.Value) == "proven" && strings.HasPrefix(fact.Key, prefix) {
			operations = append(operations, strings.TrimPrefix(fact.Key, prefix))
		}
	}
	return operations
}

func childOperationKinds(artifact equation.Artifact) map[string]string {
	kinds := make(map[string]string, len(artifact.Equations))
	for _, operation := range artifact.Equations {
		kinds[operation.Target.Name] = operation.Occurrence.Kind
	}
	return kinds
}
