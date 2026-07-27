package engine

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	placement "github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/module/exportrelation"
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
func (l *lexicalEvaluator) escapeSummaryForExport(body equation.BodyID, closure equation.OutputClosure) (summary map[string][]signature.ParamRelation, allocatedReturns map[string]bool, returnTuples map[string][][][]byte, returnTemplates map[string][]exportrelation.Value, conditionalReturns map[string]*exportrelation.ConditionalReturn, forwardedReturns map[string]exportrelation.Value) {
	defer func() {
		if recover() != nil {
			summary, allocatedReturns, returnTuples, returnTemplates, conditionalReturns, forwardedReturns = nil, nil, nil, nil, nil, nil
		}
	}()
	if l == nil {
		return nil, nil, nil, nil, nil, nil
	}
	partition, err := equation.PartitionFromClosuresWithGuards(nil, closure)
	if err != nil {
		return nil, nil, nil, nil, nil, nil
	}
	exports := exportedFunctionHandles(closure.Values)
	if len(exports) == 0 {
		return nil, nil, nil, nil, nil, nil
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
		relations, allocatedReturn, tuples, templates, conditional, forwarded, hasForwarded, ok := l.escapeRelationsForPrototype(body, child, partition)
		if ok {
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
		if len(tuples) == 0 {
			tuples, templates, conditional, forwarded, hasForwarded = l.returnTuplesForPrototype(body, child, partition)
		}
		if len(templates) != 0 {
			if returnTemplates == nil {
				returnTemplates = make(map[string][]exportrelation.Value)
			}
			returnTemplates[name] = templates
		}
		if conditional != nil {
			if conditionalReturns == nil {
				conditionalReturns = make(map[string]*exportrelation.ConditionalReturn)
			}
			conditionalReturns[name] = conditional
		}
		if len(tuples) != 0 {
			if returnTuples == nil {
				returnTuples = make(map[string][][][]byte)
			}
			returnTuples[name] = tuples
		}
		if hasForwarded {
			if forwardedReturns == nil {
				forwardedReturns = make(map[string]exportrelation.Value)
			}
			forwardedReturns[name] = forwarded
		}
	}
	return summary, allocatedReturns, returnTuples, returnTemplates, conditionalReturns, forwardedReturns
}

// returnTuplesForPrototype uses the same closed declared-entry lane as the
// uncalled evaluator.  It is independent of escape classification: scalar
// formals need no placement identity, but their body can still establish a
// complete value/error return relation.
func (l *lexicalEvaluator) returnTuplesForPrototype(body equation.BodyID, child front.Compilation, partition equation.Partition) ([][][]byte, []exportrelation.Value, *exportrelation.ConditionalReturn, exportrelation.Value, bool) {
	seeds, ok := declaredFormalSeeds(child)
	if !ok {
		return nil, nil, nil, exportrelation.Value{}, false
	}
	index := l.admissionBody(child)
	entry, admitted, err := index.childEntry(l, body, seeds, partition, false, true, true, true)
	if err != nil || !admitted {
		return nil, nil, nil, exportrelation.Value{}, false
	}
	outcome, _, err := l.evaluate(child, entry)
	if err != nil {
		return nil, nil, nil, exportrelation.Value{}, false
	}
	tuples, complete := completeReturnCandidates(outcome.Outcomes)
	if !complete {
		return nil, nil, nil, exportrelation.Value{}, false
	}
	forwarded, hasForwarded := singleForwardedReturn(child, tuples, outcome.Values)
	origins := singleReturnMemberOrigins(child, outcome)
	templates := completeReturnTemplates(child, outcome, origins)
	return tuples, templates, conditionalReturnForPrototype(child, outcome, origins), forwarded, hasForwarded
}

// exportedFunctionHandles reads the returned-table member closure capabilities
// the publication kernel already published for return slot zero. It maps each
// exported member name to its closure handle without re-deriving the return
// shape from source.
func exportedFunctionHandles(values []equation.Fact) map[string]closureHandle {
	prefix := factkey.ReturnMemberClosure.Key().String()
	handles := make(map[string]closureHandle)
	for _, fact := range values {
		if !factkey.OwnsPrefix(prefix, fact.Key) {
			continue
		}
		parts := factkey.Segments(fact.Key)
		if len(parts) != 4 || parts[2] != "00000000" {
			continue
		}
		var wire memberClosureWire
		if front.DecodeRequiredWireJSON(fact.Value, &wire) != nil || !validClosureHandle(wire.Handle) {
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
func (l *lexicalEvaluator) escapeRelationsForPrototype(body equation.BodyID, child front.Compilation, partition equation.Partition) ([]signature.ParamRelation, bool, [][][]byte, []exportrelation.Value, *exportrelation.ConditionalReturn, exportrelation.Value, bool, bool) {
	entry, identities, ok := l.escapeChildEntry(body, child, partition)
	if !ok {
		return nil, false, nil, nil, nil, exportrelation.Value{}, false, false
	}
	outcome, _, err := l.evaluate(child, entry)
	if err != nil {
		return nil, false, nil, nil, nil, exportrelation.Value{}, false, false
	}
	tuples, complete := completeReturnCandidates(outcome.Outcomes)
	if !complete {
		tuples = nil
	}
	// Tuple publication is optional.  An opaque or partial return catalog must
	// never suppress the independent placement escape summary.
	forwarded, hasForwarded := singleForwardedReturn(child, tuples, outcome.Values)
	origins := singleReturnMemberOrigins(child, outcome)
	templates := completeReturnTemplates(child, outcome, origins)
	return classifyEscapeParameters(child, outcome.Values, identities), allocatedReturnGraph(outcome.Values, identities), tuples, templates, conditionalReturnForPrototype(child, outcome, origins), forwarded, hasForwarded, true
}

// completeReturnCandidates preserves the engine's path-correlated return
// facts. A missing arity or slot rejects the entire catalog: downstream
// consumers must never infer a tuple from an opaque return path.
type returnCandidateValues struct {
	arity  int
	seen   bool
	values map[int][]byte
}

func completeReturnCandidates(outcomes []equation.Fact) ([][][]byte, bool) {
	candidates := make(map[string]*returnCandidateValues)
	for _, fact := range outcomes {
		parts := factkey.Segments(fact.Key)
		if len(parts) != 3 || parts[0] != "return-candidate" || parts[1] == "" {
			continue
		}
		item := candidates[parts[1]]
		if item == nil {
			item = &returnCandidateValues{arity: -1, values: make(map[int][]byte)}
			candidates[parts[1]] = item
		}
		if parts[2] == "arity" {
			arity, err := strconv.Atoi(string(fact.Value))
			if err != nil || arity < 0 || item.seen {
				return nil, false
			}
			item.arity, item.seen = arity, true
			continue
		}
		index, err := strconv.Atoi(parts[2])
		if err != nil || index < 0 {
			return nil, false
		}
		if _, duplicate := item.values[index]; duplicate {
			return nil, false
		}
		item.values[index] = append([]byte(nil), fact.Value...)
	}
	if len(candidates) == 0 {
		return nil, false
	}
	names := make([]string, 0, len(candidates))
	for name := range candidates {
		names = append(names, name)
	}
	sort.Strings(names)
	tuples := make([][][]byte, 0, len(names))
	for _, name := range names {
		item := candidates[name]
		if !item.seen || item.arity < 1 || len(item.values) != item.arity {
			return nil, false
		}
		values := make([][]byte, item.arity)
		for index := range values {
			value, found := item.values[index]
			if !found {
				return nil, false
			}
			values[index] = value
		}
		tuples = append(tuples, values)
	}
	arity := candidates[names[0]].arity
	for _, name := range names {
		if candidates[name].arity != arity {
			return nil, false
		}
	}
	return tuples, true
}

func completeReturnTemplates(child front.Compilation, outcome equation.OutputClosure, origins map[string]int) []exportrelation.Value {
	candidates := completeReturnTemplateMap(child, outcome, origins)
	if len(candidates) == 0 {
		return nil
	}
	names := make([]string, 0, len(candidates))
	for name := range candidates {
		names = append(names, name)
	}
	sort.Strings(names)
	templates := make([]exportrelation.Value, 0, len(names))
	for _, name := range names {
		templates = append(templates, candidates[name])
	}
	return templates
}

func completeReturnTemplateMap(child front.DraftsBoundaryView, outcome equation.OutputClosure, origins map[string]int) map[string]exportrelation.Value {
	candidates := make(map[string][]byte)
	for _, fact := range outcome.Outcomes {
		parts := factkey.Segments(fact.Key)
		if len(parts) == 3 && parts[0] == "return-candidate" && parts[2] == "arity" && string(fact.Value) == "1" {
			candidates[parts[1]] = nil
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	publications := 0
	for _, operation := range child.DraftArtifact().Equations {
		if operation.Occurrence.Kind == "publication" {
			publications++
		}
	}
	if publications != len(candidates) {
		return nil
	}
	for _, fact := range outcome.Values {
		parts := factkey.Segments(fact.Key)
		if len(parts) != 3 || parts[0] != "return-relation-surface" || parts[2] != "00000000" {
			continue
		}
		if _, found := candidates[parts[1]]; found {
			candidates[parts[1]] = fact.Value
		}
	}
	templates := make(map[string]exportrelation.Value, len(candidates))
	for name, surface := range candidates {
		if len(surface) == 0 {
			return nil
		}
		template, ok := relationSurfaceTemplate(surface, origins, "")
		if !ok {
			parameter, direct := soleDirectReturnFormal(child)
			if len(candidates) != 1 || !direct {
				return nil
			}
			template = exportrelation.Value{Parameter: &parameter}
		}
		templates[name] = template
	}
	return templates
}

func conditionalReturnForPrototype(child front.DraftsBoundaryView, outcome equation.OutputClosure, origins map[string]int) *exportrelation.ConditionalReturn {
	templates := completeReturnTemplateMap(child, outcome, origins)
	if len(templates) != 2 {
		return nil
	}
	formals := make(map[string]int, len(child.BodyBoundary().Parameters))
	for index, parameter := range child.BodyBoundary().Parameters {
		term := boundaryTerm(parameter.Symbol)
		formals[term] = index
		formals[strings.TrimPrefix(term, "path/")] = index
	}
	branchName := ""
	parameter := -1
	literal := ""
	for _, operation := range child.DraftArtifact().Equations {
		if operation.Occurrence.Kind != "branch-relations" {
			continue
		}
		encoded, found := artifactOperand(operation.Operands, equation.MustOperandRole("predicate"))
		predicate, decoded, err := front.DecodeBranchPredicateWire(encoded)
		index, formal := formals[predicate.Path]
		if !found || err != nil || !decoded || predicate.Kind != "literal-equal" || predicate.Negated ||
			predicate.Literal == "" || !formal || branchName != "" {
			continue
		}
		branchName, parameter, literal = operation.Target.Name, index, predicate.Literal
	}
	if branchName == "" {
		return nil
	}
	var match, otherwise exportrelation.Value
	haveMatch, haveOtherwise := false, false
	for _, operation := range child.DraftArtifact().Equations {
		template, returned := templates[operation.Target.Name]
		if !returned || operation.Occurrence.Kind != "publication" {
			continue
		}
		edge := ""
		for _, guard := range operation.Guards {
			parts := strings.Split(string(guard.Encoding), "/")
			if len(parts) == 4 && parts[0] == "front" && parts[1] == "branch" && parts[2] == branchName {
				if edge != "" {
					return nil
				}
				edge = parts[3]
			}
		}
		switch edge {
		case "true":
			match, haveMatch = template, true
		case "false":
			otherwise, haveOtherwise = template, true
		default:
			return nil
		}
	}
	if !haveMatch || !haveOtherwise {
		return nil
	}
	return &exportrelation.ConditionalReturn{Parameter: parameter, Literal: literal, Match: match, Otherwise: otherwise}
}

func relationSurfaceTemplate(encoded []byte, origins map[string]int, prefix string) (exportrelation.Value, bool) {
	if parameter, found := origins[prefix]; found {
		return exportrelation.Value{Parameter: &parameter}, true
	}
	scalar := exportrelation.Value{Scalar: string(encoded)}
	if scalar.Closed() {
		return scalar, true
	}
	shape, ok := shapefact.DecodeTable(encoded)
	if !ok || !shape.Closed || len(shape.Members) == 0 {
		return exportrelation.Value{}, false
	}
	value := exportrelation.Value{Table: make([]exportrelation.Member, 0, len(shape.Members))}
	for _, member := range shape.Members {
		if !member.Present || member.Suffix == "" {
			return exportrelation.Value{}, false
		}
		segments, ok := segment.ParseFormattedSegments(member.Suffix)
		if !ok {
			return exportrelation.Value{}, false
		}
		if len(segments) != 1 {
			continue
		}
		child, ok := relationSurfaceTemplate([]byte(member.Value), origins, prefix+member.Suffix)
		if !ok {
			return exportrelation.Value{}, false
		}
		value.Table = append(value.Table, exportrelation.Member{Suffix: member.Suffix, Value: child})
	}
	return value, len(value.Table) != 0
}

func singleReturnMemberOrigins(child front.BoundaryView, outcome equation.OutputClosure) map[string]int {
	candidate := ""
	for _, fact := range outcome.Outcomes {
		parts := factkey.Segments(fact.Key)
		if len(parts) != 3 || parts[0] != "return-candidate" || parts[2] != "arity" || string(fact.Value) != "1" {
			continue
		}
		if candidate != "" && candidate != parts[1] {
			return nil
		}
		candidate = parts[1]
	}
	if candidate == "" {
		return nil
	}
	formals := make(map[string]int, len(child.BodyBoundary().Parameters))
	for index, parameter := range child.BodyBoundary().Parameters {
		formals[boundaryTerm(parameter.Symbol)] = index
	}
	if len(formals) == 0 {
		return nil
	}
	prefix := factkey.ReturnMemberOrigin.Key().String() + candidate + "/00000000/"
	origins := make(map[string]int)
	for _, fact := range outcome.Values {
		encoded, found := factkey.TailPrefix(prefix, fact.Key)
		if !found || encoded == "" {
			continue
		}
		suffix, err := base64.RawURLEncoding.DecodeString(encoded)
		index, formal := formals[string(fact.Value)]
		if err != nil || len(suffix) == 0 || !formal {
			continue
		}
		if prior, exists := origins[string(suffix)]; exists && prior != index {
			return nil
		}
		origins[string(suffix)] = index
	}
	return origins
}

func singleForwardedReturn(child front.DraftsBoundaryView, tuples [][][]byte, values []equation.Fact) (exportrelation.Value, bool) {
	if len(tuples) != 1 || len(tuples[0]) != 1 {
		return exportrelation.Value{}, false
	}
	returned := ""
	for _, operation := range child.DraftArtifact().Equations {
		if operation.Occurrence.Kind != "publication" {
			continue
		}
		value, found := artifactOperand(operation.Operands, equation.IndexedRole(equation.RoleFamilyReturnValue, 0))
		if !found {
			return exportrelation.Value{}, false
		}
		for _, operand := range operation.Operands {
			if operand.Role.InFamily(equation.RoleFamilyReturnValue) && operand.Role != equation.IndexedRole(equation.RoleFamilyReturnValue, 0) {
				return exportrelation.Value{}, false
			}
		}
		if returned != "" && returned != string(value) {
			return exportrelation.Value{}, false
		}
		returned = string(value)
	}
	if returned == "" {
		return exportrelation.Value{}, false
	}
	prefix := factkey.ImportedReturnRelation.Key().String() + returned + "/"
	var encoded []byte
	latest := ""
	for _, fact := range values {
		if factkey.OwnsPrefix(prefix, fact.Key) && fact.Key > latest {
			encoded, latest = fact.Value, fact.Key
		}
	}
	if encoded == nil {
		return exportrelation.Value{}, false
	}
	var wire importedReturnRelationWire
	if front.DecodeRequiredWireJSON(encoded, &wire) != nil {
		return exportrelation.Value{}, false
	}
	formals := make(map[string]int, len(child.BodyBoundary().Parameters))
	for index, parameter := range child.BodyBoundary().Parameters {
		formals[boundaryTerm(parameter.Symbol)] = index
	}
	var compose func(exportrelation.Value) (exportrelation.Value, bool)
	compose = func(template exportrelation.Value) (exportrelation.Value, bool) {
		if template.Parameter != nil {
			index := *template.Parameter
			if index < 0 || index >= len(wire.Arguments) {
				return exportrelation.Value{}, false
			}
			argument := wire.Arguments[index]
			if parameter, found := formals[argument]; found {
				return exportrelation.Value{Parameter: &parameter}, true
			}
			scalar := exportrelation.Value{Scalar: argument}
			return scalar, scalar.Closed()
		}
		if template.Scalar != "" {
			return exportrelation.Value{Scalar: template.Scalar}, true
		}
		if len(template.Table) == 0 {
			return exportrelation.Value{}, false
		}
		out := exportrelation.Value{Table: make([]exportrelation.Member, 0, len(template.Table))}
		for _, member := range template.Table {
			child, ok := compose(member.Value)
			if !ok || member.Suffix == "" {
				return exportrelation.Value{}, false
			}
			out.Table = append(out.Table, exportrelation.Member{Suffix: member.Suffix, Value: child})
		}
		return out, true
	}
	forwarded, ok := compose(wire.Template)
	return forwarded, ok && forwarded.Valid(len(child.BodyBoundary().Parameters))
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
func (l *lexicalEvaluator) escapeChildEntry(body equation.BodyID, child front.Compilation, partition equation.Partition) ([]byte, map[int]string, bool) {
	if child.LoweredBody() == nil || len(child.BodyBoundary().Parameters) == 0 {
		return nil, nil, false
	}
	formalSeeds, ok := declaredFormalWitnessSeeds(child)
	if !ok {
		return nil, nil, false
	}
	placementSeeds := make([]entryPlacementSeed, 0, len(child.BodyBoundary().Parameters))
	identities := make(map[int]string, len(child.BodyBoundary().Parameters))
	for index, parameter := range child.BodyBoundary().Parameters {
		declared := unwrap.Alias(child.LoweredBody().Type(parameter.Type))
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
	seeds, closureSeeds, memberClosureSeeds, tableIdentitySeeds, memberCellSeeds, admitted := l.childEntrySeedSet(body, child, formalSeeds, partition, true, false, false)
	if !admitted {
		return nil, nil, false
	}
	entry, err := encodeChildEntryWithPlacementCapabilities(seeds, closureSeeds, memberClosureSeeds, tableIdentitySeeds, memberCellSeeds, placementSeeds)
	if err != nil {
		return nil, nil, false
	}
	return entry, identities, true
}

func escapeParameterIdentity(child front.BoundaryView, index int) string {
	return "escape-param/" + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%x/%d", child.BodyID(), index)))
}

// classifyEscapeParameters reads the disposition of each formal from the
// placement facts of the evaluated body. A formal is a borrow only when the
// body published no owned event, no shared event, no opaque-call blocker, and no
// return escape for its identity. An unseeded (closed-scalar) formal is a borrow
// by construction.
func classifyEscapeParameters(child front.DraftsBoundaryView, values []equation.Fact, identities map[int]string) []signature.ParamRelation {
	parsed := parsePublishedPlacement(values)
	// Containment carries an escaping container's disposition to the elements it
	// holds, so a formal stored behind an intermediate table is not mistaken for
	// a borrow.
	propagatePublishedPlacement(parsed)
	opKinds := childOperationKinds(child.DraftArtifact())
	relations := make([]signature.ParamRelation, 0, len(child.BodyBoundary().Parameters))
	for index := range child.BodyBoundary().Parameters {
		relation := signature.ParamRelation{Param: index, EscapeClass: placement.Borrow, PlacementConsequence: placement.Keep}
		if identity, seeded := identities[index]; seeded {
			relation = classifyEscapeIdentity(index, identity, parsed, values, opKinds, identities)
		}
		relations = append(relations, relation)
	}
	return relations
}

func soleDirectReturnFormal(child front.DraftsBoundaryView) (int, bool) {
	formals := make(map[string]int, len(child.BodyBoundary().Parameters))
	for index, parameter := range child.BodyBoundary().Parameters {
		formals[boundaryTerm(parameter.Symbol)] = index
	}
	returned, seen := -1, false
	for _, operation := range child.DraftArtifact().Equations {
		if operation.Occurrence.Kind == "entry" {
			continue
		}
		if operation.Occurrence.Kind != "publication" {
			return 0, false
		}
		value, found := artifactOperand(operation.Operands, equation.IndexedRole(equation.RoleFamilyReturnValue, 0))
		if !found {
			return 0, false
		}
		for _, operand := range operation.Operands {
			if operand.Role.InFamily(equation.RoleFamilyReturnValue) && operand.Role != equation.IndexedRole(equation.RoleFamilyReturnValue, 0) {
				return 0, false
			}
		}
		index, formal := formals[string(value)]
		if !formal || (seen && returned != index) {
			return 0, false
		}
		returned, seen = index, true
	}
	return returned, seen
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
		if other == param || len(placementProvenOperations(values, factkey.PlacementEvent.Key().String(), identity, placementEventOwned)) != 0 {
			continue
		}
		for _, operation := range placementProvenOperations(values, factkey.PlacementContract.Key().String(), identity, placement.BoundaryRetain) {
			if boundaries[operation] {
				container, candidates = other, candidates+1
				break
			}
		}
	}
	return container, candidates == 1
}

func classifyEscapeIdentity(index int, identity string, parsed publishedPlacementFacts, values []equation.Fact, opKinds map[string]string, identities map[int]string) signature.ParamRelation {
	relation := signature.ParamRelation{Param: index, EscapeClass: placement.Borrow, PlacementConsequence: placement.Keep}
	events := parsed.events[identity]
	ownedOps := placementProvenOperations(values, factkey.PlacementEvent.Key().String(), identity, placementEventOwned)
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
		relation.EscapeClass = placement.Send
		relation.PlacementConsequence = placement.ConsequenceShared
	case storeOwned || containedOwned:
		relation.EscapeClass = placement.Store
		relation.PlacementConsequence = placement.ConsequenceOwned
		relation.StoredInto, relation.HasStoredInto = storeContainerFormal(index, ownedOps, values, identities, opKinds)
	case returnOwned:
		relation.EscapeClass = placement.Export
		relation.ThroughReturn = true
		relation.PlacementConsequence = placement.Keep
	case parsed.blockers[identity]["opaque-call"] && placementBlockerStands(identity, "opaque-call", parsed.blockerOperations, parsed.contracts):
		relation.EscapeClass = placement.Opaque
		relation.PlacementConsequence = placement.ConsequenceOwned
	}
	return relation
}

// placementProvenOperations returns the operations that published a proven
// placement fact for identity under one boundary of the named fact family. The
// operation kind distinguishes a store from a return escape without inspecting
// source.
func placementProvenOperations[T ~string](values []equation.Fact, family, identity string, boundary T) []string {
	prefix := family + base64.RawURLEncoding.EncodeToString([]byte(identity)) + "/" + string(boundary) + "/"
	var operations []string
	for _, fact := range values {
		if string(fact.Value) == "proven" && factkey.OwnsPrefix(prefix, fact.Key) {
			operations = append(operations, factkey.BodyPrefix(prefix, fact.Key))
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
