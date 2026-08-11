// Package rule declares Escape's direct delivered-payload boundary judgment.
// Target owns the static transfer declaration and Pack owns the application
// selection.  Value is consulted only through Pack's exact endpoint
// routes; no activation carries a scalar Value payload.
package rule

import (
	"github.com/wippyai/go-lua/analysis/domain/escape"
	escapeowner "github.com/wippyai/go-lua/analysis/domain/escape/owner"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	"github.com/wippyai/go-lua/program/target"
)

// TransferOperand is Escape's private static transfer/outcome row. source is
// an identity fence, not an Application carrier: the one Pack import names
// the exact Link call occurrence, and Pack projects the Target payload
// lazily from it.
type TransferOperand struct {
	source   *link.Link
	transfer target.TransferID
	outcome  uint32
	content  keyspace.ContentID
	selector packdomain.InputSelector
}

// NewTransferOperand admits one declared deliverable transfer outcome.  Its
// ContentID is Target's canonical endpoint identity; this package never
// rebuilds an equivalent hash from Link/Application coordinates.
func NewTransferOperand(source *link.Link, transfer target.TransferID, outcome uint32) (TransferOperand, bool) {
	if source == nil || !source.ContentID().Available() {
		return TransferOperand{}, false
	}
	contract, ok := source.Boundary().Target()
	if !ok || contract == nil {
		return TransferOperand{}, false
	}
	owner, ok := contract.TransferOwner(transfer)
	if !ok {
		return TransferOperand{}, false
	}
	content, disposition, ok := contract.TransferOutcomeContentID(owner, transfer, int(outcome))
	if !ok || !content.Available() || disposition&target.TransferMayDeliver == 0 {
		return TransferOperand{}, false
	}
	return TransferOperand{source: source, transfer: transfer, outcome: outcome, content: content}, true
}

// ContentID is Target's canonical transfer/outcome identity, not an Escape
// key and not a Link/Application-derived digest.
func (operand TransferOperand) ContentID() keyspace.ContentID { return operand.content }

// Rule is Escape's one-input Pack projection.  It does not decide transfer
// isolation, heap containment, ownership, or placement.
type Rule struct {
	semantic engine.SemanticKey
	rule     *engine.Rule[escape.Value, TransferOperand]
	escape   *escapeowner.Owner
	packs    *packowner.Owner
	values   *valueowner.Owner
	packRead engine.Read[engine.OrderedCells[packdomain.Value]]
	sources  engine.Read[engine.Selection[uint64, engine.OrderedCells[value.Value]]]
	write    engine.Write[escape.Value]
}

// MatchesLink is the provenance fence used by activation source compilers.
// It accepts only the exact Link shared by Escape, Pack, and Value owners;
// same-content foreign schemas cannot supply a transfer Rule to another
// source compiler.
func (rule *Rule) MatchesLink(source *link.Link) bool {
	if rule == nil || source == nil || rule.escape == nil || rule.packs == nil || rule.values == nil {
		return false
	}
	escapeSchema, packSchema, valueSchema := rule.escape.Schema(), rule.packs.Schema(), rule.values.Schema()
	return escapeSchema.Valid() && packSchema != nil && valueSchema != nil &&
		escapeSchema.Link() == source && packSchema.Link() == source && valueSchema.Link() == source
}

// Declare records the cold cross-factor judgment. All three owners must be
// for one exact Link. The selector is a domain-owned Pack endpoint walk, not
// an Application×operation registry.
func Declare(
	composition *engine.Composition,
	semantic, operandFamily, evidence engine.SemanticKey,
	escapes *escapeowner.Owner,
	packs *packowner.Owner,
	values *valueowner.Owner,
) (*Rule, bool) {
	if composition == nil || escapes == nil || packs == nil || values == nil || !escapes.Schema().Valid() || packs.Schema() == nil || values.Schema() == nil ||
		!semantic.Available() || !operandFamily.Available() || !evidence.Available() ||
		semantic == operandFamily || semantic == evidence || operandFamily == evidence {
		return nil, false
	}
	escapeLink, packLink := escapes.Schema().Link(), packs.Schema().Link()
	if escapeLink == nil || packLink == nil || values.Schema().Link() != escapeLink || packLink != escapeLink {
		return nil, false
	}
	declaration := &Rule{semantic: semantic, escape: escapes, packs: packs, values: values}
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[escape.Value, TransferOperand]{
		Semantic: semantic, OperandFamily: operandFamily, OperandContent: declaration.operandContent,
		Output: escapes.Output(), Inputs: 1,
		Admission: engine.AdmitRuleByDerivation(evidence, declaration.check), Transfer: declaration.transfer,
	}, func(rule *engine.Rule[escape.Value, TransferOperand]) bool {
		input, inputOK := rule.InputAt(0)
		packRead, packReadOK := engine.ReadFrom(rule, input, packs.ExactRead())
		sources, sourceReadOK := engine.SelectRead[escape.Value, TransferOperand, value.Value, engine.OrderedCells[value.Value], uint64](
			rule, input, values.ExactRead(), []engine.Dependency{engine.ReadDependency(packRead)}, declaration.locateSources,
		)
		write, writeOK := engine.WriteTo(rule, escapes.Write())
		if !inputOK || !packReadOK || !sourceReadOK || !writeOK {
			return false
		}
		declaration.rule, declaration.packRead, declaration.sources, declaration.write = rule, packRead, sources, write
		return true
	})
	if !ok || declared == nil || declaration.rule != declared {
		return nil, false
	}
	if !declaration.MatchesLink(escapeLink) {
		return nil, false
	}
	return declaration, true
}

// Prototype creates the one activation-owned instance form. The Pack import
// ABI is intentionally identical for every transfer endpoint: role/slot are
// one pair, and all Value routes are local dynamic selector surfaces.
func (rule *Rule) Prototype(operand TransferOperand, role, slot engine.SemanticKey) (*engine.RuleInstance[escape.Value, TransferOperand], bool) {
	if rule == nil || rule.rule == nil {
		return nil, false
	}
	bound, boundOK := rule.bindOperand(operand)
	boundary, endpointOK := rule.endpoint(bound)
	if !boundOK || !endpointOK {
		return nil, false
	}
	escapeRef, escapeOK := rule.escape.Locate(boundary)
	if !escapeOK || !role.Available() || !slot.Available() {
		return nil, false
	}
	return engine.NewActivationPrototypeInstance(rule.rule, bound, func(binding *engine.RuleBinding[escape.Value, TransferOperand]) bool {
		return engine.InstancePortRead(binding, rule.packRead, role, slot) &&
			engine.InstanceSelectorRead(binding, rule.sources, rule.values.ExactRead()) &&
			engine.InstanceWrite(binding, rule.write, escapeRef)
	})
}

func (rule *Rule) operandContent(operand TransferOperand) (TransferOperand, [32]byte, bool) {
	if _, ok := rule.endpoint(operand); !ok || !rule.selectorMatches(operand) {
		return TransferOperand{}, [32]byte{}, false
	}
	return operand, [32]byte(operand.content), true
}

func (rule *Rule) endpoint(operand TransferOperand) (escape.Coordinate, bool) {
	if rule == nil || rule.escape == nil || rule.packs == nil || rule.values == nil || operand.source == nil || !operand.content.Available() ||
		!rule.escape.Schema().Valid() || rule.packs.Schema() == nil || rule.values.Schema() == nil {
		return escape.Coordinate{}, false
	}
	if rule.escape.Schema().Link() != operand.source || rule.packs.Schema().Link() != operand.source || rule.values.Schema().Link() != operand.source {
		return escape.Coordinate{}, false
	}
	if rebuilt, ok := NewTransferOperand(operand.source, operand.transfer, operand.outcome); !ok || rebuilt.content != operand.content {
		return escape.Coordinate{}, false
	}
	boundary, boundaryOK := rule.escape.Schema().CoordinateForTransfer(operand.transfer)
	if !boundaryOK {
		return escape.Coordinate{}, false
	}
	return boundary, true
}

// bindOperand freezes Target's declared payload into the private operand copy
// before an activation prototype is admitted. It is Target-owned static data,
// never an Application/formal lookup table.
func (rule *Rule) bindOperand(operand TransferOperand) (TransferOperand, bool) {
	if rule == nil || rule.packs == nil || rule.packs.Schema() == nil || operand.source == nil {
		return TransferOperand{}, false
	}
	contract, contractOK := operand.source.Boundary().Target()
	if !contractOK || contract == nil {
		return TransferOperand{}, false
	}
	operation, ownerOK := contract.TransferOwner(operand.transfer)
	_, payload, _, _, _, declarationOK := contract.TransferDeclaration(operand.transfer)
	if !ownerOK || !declarationOK {
		return TransferOperand{}, false
	}
	selector, selectorOK := rule.packs.Schema().InputSelector(operation, payload)
	if !selectorOK {
		return TransferOperand{}, false
	}
	operand.selector = selector
	return operand, true
}

// selectorMatches authenticates the operand-local cold selector only while
// an instance is built or admitted. Observe reads the already-checked field
// directly, so it has no Target query, map lookup, offset lookup, or mutable
// Rule state on the solver path.
func (rule *Rule) selectorMatches(operand TransferOperand) bool {
	if rule == nil || rule.packs == nil || rule.packs.Schema() == nil || operand.source == nil {
		return false
	}
	contract, contractOK := operand.source.Boundary().Target()
	if !contractOK || contract == nil {
		return false
	}
	operation, ownerOK := contract.TransferOwner(operand.transfer)
	_, payload, _, _, _, declarationOK := contract.TransferDeclaration(operand.transfer)
	if !ownerOK || !declarationOK {
		return false
	}
	expected, expectedOK := rule.packs.Schema().InputSelector(operation, payload)
	return expectedOK && operand.selector == expected
}

// observe applies the cold selector to this exact non-extreme Call Pack
// fact. It never reconstructs an application identity, consults Target, or
// turns an open Pack tail into a scalar value on the solver path.
func (rule *Rule) observe(operand TransferOperand, fact packdomain.Value) (packdomain.InputObservation, bool) {
	if rule == nil || rule.packs == nil || rule.packs.Schema() == nil || fact.IsBottom() || fact.IsTop() {
		return packdomain.InputObservation{}, false
	}
	schema := rule.packs.Schema()
	root, rootOK := schema.RootForValue(fact)
	if !rootOK {
		return packdomain.InputObservation{}, false
	}
	return schema.ObserveInput(root, fact, operand.selector)
}

// visitSources retains Pack's exact endpoint tags while deduplicating the
// same endpoint reached through multiple Pack alternatives. A selector cannot
// emit duplicate (Ref, tag) routes, and there is no reason to manufacture a
// second Value premise for an existing endpoint identity.
func (rule *Rule) visitSources(observation packdomain.InputObservation, visit func(linkboundary.Value, uint64) bool) (complete, ok bool) {
	if rule == nil || rule.packs == nil || rule.packs.Schema() == nil || visit == nil {
		return false, false
	}
	seen := make(map[uint64]struct{})
	return rule.packs.Schema().VisitInputSources(observation, func(source linkboundary.Value) bool {
		endpoint, endpointOK := rule.packs.Schema().Endpoint(source)
		tag, tagOK := rule.packs.Schema().EndpointTag(endpoint)
		if !endpointOK || !tagOK {
			return false
		}
		if _, duplicate := seen[tag]; duplicate {
			return true
		}
		seen[tag] = struct{}{}
		return visit(source, tag)
	})
}

// locateSources is the typed Pack-to-Value projection boundary. It takes
// existing Boundary endpoint identities from the selected Pack fact and routes
// them to existing Value coordinates; no Value fact is placed on the
// activation port.
func (rule *Rule) locateSources(context engine.SelectorContext, operand TransferOperand) bool {
	if rule == nil || rule.packs == nil || rule.values == nil {
		return false
	}
	cells, readOK := engine.SelectorRead(context, rule.packRead)
	if !readOK || cells.Count() != 1 {
		return false
	}
	fact, present, available := cells.At(0)
	if !available || !present || fact.IsBottom() || fact.IsTop() {
		return available
	}
	observation, observed := rule.observe(operand, fact)
	if !observed || observation.IsBottom() || observation.IsTop() {
		return observed
	}
	type route struct {
		source linkboundary.Value
		tag    uint64
	}
	routes := make([]route, 0)
	complete, visited := rule.visitSources(observation, func(source linkboundary.Value, tag uint64) bool {
		routes = append(routes, route{source: source, tag: tag})
		return true
	})
	// An incomplete Pack observation must be widened by transfer. It has no
	// finite exact Value routes, so the staged read intentionally stays empty.
	if !visited || !complete {
		return visited
	}
	for _, route := range routes {
		coordinate, coordinateOK := rule.values.Schema().CoordinateFor(route.source)
		ref, refOK := rule.values.Locate(coordinate)
		if !coordinateOK || !refOK || !engine.SelectRoute(context, ref, route.tag) {
			return false
		}
	}
	return true
}

// reduce maps one selected Value premise to Escape's finite root relation.
// Exact source allocations cross a dynamic boundary as Recent; a
// pre-materialized Recent/Summary alternative retains its role. Primitive,
// endpoint, boot, and opaque references create no allocation escape claim.
func (rule *Rule) reduce(fact value.Value) (escape.Value, bool) {
	if rule == nil || rule.escape == nil || rule.values == nil || !rule.escape.Schema().Valid() || rule.values.Schema() == nil {
		return escape.Value{}, false
	}
	valueSchema := rule.values.Schema()
	escapeSchema := rule.escape.Schema()
	if valueSchema.Equal(fact, valueSchema.Top()) {
		return escapeSchema.Top()
	}
	result, resultOK := escapeSchema.Bottom()
	if !resultOK {
		return escape.Value{}, false
	}
	derived := false
	visited := valueSchema.VisitAtoms(fact, func(atom value.Atom) bool {
		reference, role, rooted := atom.Reference()
		if !rooted {
			return true
		}
		raw, allocation := reference.AllocationKey()
		if !allocation {
			return true
		}
		escapeRoot, rootOK := escapeSchema.RootFor(raw)
		mapped, mappedOK := escapeRole(role)
		if !rootOK || !mappedOK {
			derived = false
			return false
		}
		piece, pieceOK := escapeSchema.Of(escapeRoot, mapped)
		next, joined := escapeSchema.Join(result, piece)
		if !pieceOK || !joined {
			derived = false
			return false
		}
		result, derived = next, true
		return true
	})
	return result, visited && derived
}

func escapeRole(role materialization.Role) (materialization.Role, bool) {
	switch role {
	case materialization.Exact, materialization.Recent:
		return materialization.Recent, true
	case materialization.Summary:
		return materialization.Summary, true
	default:
		return materialization.Invalid, false
	}
}

func (rule *Rule) reduceSelection(access engine.Access[escape.Value, TransferOperand], row engine.Row) (escape.Value, bool) {
	selection, selected := engine.ReadValue(access, row, rule.sources)
	if !selected {
		return escape.Value{}, false
	}
	count, counted := engine.SelectionCount(access, row, selection)
	if !counted || count < 0 {
		return escape.Value{}, false
	}
	result, resultOK := rule.escape.Schema().Bottom()
	if !resultOK {
		return escape.Value{}, false
	}
	derived := false
	for index := 0; index < count; index++ {
		_, cells, selected := engine.SelectionAt(access, row, selection, index)
		if !selected || cells.Count() != 1 {
			return escape.Value{}, false
		}
		fact, present, available := cells.At(0)
		if !available {
			return escape.Value{}, false
		}
		if !present {
			continue
		}
		piece, reduced := rule.reduce(fact)
		if !reduced {
			continue
		}
		next, joined := rule.escape.Schema().Join(result, piece)
		if !joined {
			return escape.Value{}, false
		}
		result, derived = next, true
	}
	return result, derived
}

func (rule *Rule) transfer(access engine.Access[escape.Value, TransferOperand]) bool {
	operand, operandOK := engine.Operand(access)
	if !operandOK {
		return false
	}
	if _, ok := rule.endpoint(operand); !ok {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		cells, readOK := engine.ReadValue(access, row, rule.packRead)
		if !readOK || cells.Count() != 1 {
			return false
		}
		fact, present, available := cells.At(0)
		if !available {
			return false
		}
		if !present || fact.IsBottom() {
			return engine.NoCandidate(access, row)
		}
		if fact.IsTop() {
			top, topOK := rule.escape.Schema().Top()
			return topOK && engine.StageValue(access, row, top)
		}
		observation, observed := rule.observe(operand, fact)
		if !observed {
			return false
		}
		if observation.IsBottom() {
			return engine.NoCandidate(access, row)
		}
		if observation.IsTop() {
			top, topOK := rule.escape.Schema().Top()
			return topOK && engine.StageValue(access, row, top)
		}
		complete, visited := rule.visitSources(observation, func(linkboundary.Value, uint64) bool { return true })
		if !visited {
			return false
		}
		if !complete {
			top, topOK := rule.escape.Schema().Top()
			return topOK && engine.StageValue(access, row, top)
		}
		result, reduced := rule.reduceSelection(access, row)
		if !reduced {
			return engine.NoCandidate(access, row)
		}
		return engine.StageValue(access, row, result)
	})
}

func (rule *Rule) expectedRoutes(observation packdomain.InputObservation) (complete bool, routes map[uint64]linkboundary.Value, ok bool) {
	routes = make(map[uint64]linkboundary.Value)
	complete, ok = rule.visitSources(observation, func(source linkboundary.Value, tag uint64) bool {
		routes[tag] = source
		return true
	})
	return complete, routes, ok
}

func (rule *Rule) check(derivation engine.RuleDerivation[escape.Value, TransferOperand]) (engine.RuleEvidence, bool) {
	// The Pack port is the one fixed read. Each exact staged Value route is
	// also represented in the derivation's read plane, so its cardinality is
	// intentionally unbounded. The selector evidence below authenticates all
	// of those routes by tag and Ref.
	if rule == nil || derivation.Rule() != rule.semantic || derivation.InputCount() != 1 || derivation.ReadCount() < 1 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	input, inputOK := derivation.InputAt(0)
	operand, operandOK := derivation.Operand()
	boundary, endpointOK := rule.endpoint(operand)
	if !inputOK || input.Guard().Empty() || !operandOK || !endpointOK || !derivation.OperandContentMatches([32]byte(operand.content)) {
		return engine.RuleEvidence{}, false
	}
	escapeRef, escapeOK := rule.escape.Locate(boundary)
	top, topOK := rule.escape.Schema().Top()
	disposition, dispositionOK := derivation.DispositionAt(0)
	cells, cellsOK := engine.DerivationDispositionReadValue(derivation, disposition, rule.packRead)
	if !escapeOK || !topOK || !dispositionOK || !cellsOK || cells.Count() != 1 || !disposition.Guard().Same(input.Guard()) {
		return engine.RuleEvidence{}, false
	}
	fact, present, available := cells.At(0)
	if !available {
		return engine.RuleEvidence{}, false
	}

	noCandidate := func() (engine.RuleEvidence, bool) {
		if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
	staged := func(expected escape.Value) (engine.RuleEvidence, bool) {
		if disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 {
			return engine.RuleEvidence{}, false
		}
		actual, valueOK := disposition.Value()
		target, targetOK := disposition.TargetAt(0)
		if !valueOK || !targetOK || !engine.TargetMatchesRef(target, escapeRef) || !rule.escape.Schema().Equal(actual, expected) {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}

	selectionCount, selectionOK := engine.DerivationDispositionSelectionCount(derivation, disposition, rule.sources)
	if !selectionOK {
		return engine.RuleEvidence{}, false
	}
	if !present || fact.IsBottom() {
		if selectionCount != 0 {
			return engine.RuleEvidence{}, false
		}
		return noCandidate()
	}
	if fact.IsTop() {
		if selectionCount != 0 {
			return engine.RuleEvidence{}, false
		}
		return staged(top)
	}
	observation, observed := rule.observe(operand, fact)
	if !observed {
		return engine.RuleEvidence{}, false
	}
	if observation.IsBottom() {
		if selectionCount != 0 {
			return engine.RuleEvidence{}, false
		}
		return noCandidate()
	}
	if observation.IsTop() {
		if selectionCount != 0 {
			return engine.RuleEvidence{}, false
		}
		return staged(top)
	}
	complete, routes, routesOK := rule.expectedRoutes(observation)
	if !routesOK {
		return engine.RuleEvidence{}, false
	}
	if !complete {
		if selectionCount != 0 {
			return engine.RuleEvidence{}, false
		}
		return staged(top)
	}
	if selectionCount != len(routes) {
		return engine.RuleEvidence{}, false
	}
	seen := make(map[uint64]struct{}, selectionCount)
	result, resultOK := rule.escape.Schema().Bottom()
	if !resultOK {
		return engine.RuleEvidence{}, false
	}
	derived := false
	for index := 0; index < selectionCount; index++ {
		tag, selected, selectedOK := engine.DerivationDispositionSelectionAt(derivation, disposition, rule.sources, index)
		source, expected := routes[tag]
		if !selectedOK || !expected || selected.Count() != 1 {
			return engine.RuleEvidence{}, false
		}
		if _, duplicate := seen[tag]; duplicate {
			return engine.RuleEvidence{}, false
		}
		seen[tag] = struct{}{}
		coordinate, coordinateOK := rule.values.Schema().CoordinateFor(source)
		ref, refOK := rule.values.Locate(coordinate)
		if !coordinateOK || !refOK || !engine.DerivationDispositionSelectionMatchesRef(derivation, disposition, rule.sources, index, ref) {
			return engine.RuleEvidence{}, false
		}
		fact, present, available := selected.At(0)
		if !available {
			return engine.RuleEvidence{}, false
		}
		if !present {
			continue
		}
		piece, reduced := rule.reduce(fact)
		if !reduced {
			continue
		}
		next, joined := rule.escape.Schema().Join(result, piece)
		if !joined {
			return engine.RuleEvidence{}, false
		}
		result, derived = next, true
	}
	if !derived {
		return noCandidate()
	}
	return staged(result)
}
