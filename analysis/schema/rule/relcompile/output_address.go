package relcompile

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// outputAddress resolves the destination geometry of a Program fold into the
// same sealed tuple coordinates Compile uses for its Apply slots. The
// published writer relation is the authored source relation for the
// retained output row; a routed write is addressed by the factor row it
// publishes, not by the route row that carries the destination projection.
// No physical ordinal or first-slot convention is involved.
func (resolver ruleResolver) outputAddress(path string, output ruleprogram.OutputDecl, publication Publication, semantic signature.Signature, occurrences []ReadOccurrence, joinOccurrences [][]ReadOccurrence, candidateRef member.CandidateRef, candidate model.RelationID, joins []JoinSpec) (algebra.OutputAddress, error) {
	if output.Mode == ruleprogram.ModeStructural {
		return algebra.OwnerNamed(), nil
	}
	if !output.Available() || !semantic.Available() {
		return algebra.OutputAddress{}, refuse(resolver.site(path), Name{Entry: resolver.entry}, KindAddress, ReasonUnavailable)
	}
	layout, layoutOK := resolver.layout(candidate, joins)
	if !layoutOK {
		return algebra.OutputAddress{}, refuse(resolver.site(path), Name{Entry: resolver.entry}, KindAddress, ReasonUndeclared)
	}

	destinationName := NewName(output.Destination.Axis, output.Destination.Member)
	destinationColumn, destinationRelation, destinationOK := resolver.destinationColumn(path, destinationName)
	if !destinationOK {
		return algebra.OutputAddress{}, refuse(resolver.site(path+".destination"), destinationName, KindAddress, ReasonUndeclared)
	}

	// Exact writes retain the authored destination projection.  It is normally
	// the candidate relation's address, but the schema also permits a
	// consumer-owned exact projection over a foreign candidate.  In both cases
	// the destination relation and its coordinate must be present in the
	// retained tuple; using candidate as a convention would silently write a
	// different row for the consumer form.
	if output.Mode == ruleprogram.ModeExact {
		projection, projectionErr := resolver.registry.Projection(resolver.site(path+".destination"), destinationName)
		if projectionErr != nil || projection.Role != member.Destination || projection.Column != destinationColumn || projection.Relation != destinationRelation || !destinationUsesWriterKeyCarrier(projection, publication) {
			return algebra.OutputAddress{}, refuse(resolver.site(path+".destination"), destinationName, KindAddress, ReasonForeign)
		}
		if !candidateRef.Issued() && projection.CandidateProvider != candidateRef {
			return algebra.OutputAddress{}, refuse(resolver.site(path+".destination.provider"), destinationName, KindAddress, ReasonForeign)
		}
		// Exact writes are one row per candidate. The destination projection is
		// owner evidence for the candidate's write coordinate; it is not a
		// physical cell in the result relation named by that projection. Resolve
		// the authored candidate coordinate from the retained candidate source,
		// as the plan ABI does, so a destination projection may be a consumer
		// projection over a foreign candidate without aliasing a joined row.
		sourceOrdinal, ordinalOK := candidateSourceOrdinal([]ReadOccurrence{CandidateOccurrence()}, candidate, layout)
		source, sourceOK := resolver.addressSource(candidate, int(sourceOrdinal), layout)
		if !ordinalOK || !sourceOK {
			return algebra.OutputAddress{}, refuse(resolver.site(path+".source"), Name{Entry: resolver.entry}, KindAddress, ReasonUndeclared)
		}
		if !resolver.sourceSupplies(semantic, occurrences, layout, source, candidate, candidateRef, candidate) {
			return algebra.OutputAddress{}, refuse(resolver.site(path+".source"), destinationName, KindAddress, ReasonForeign)
		}
		return algebra.ScalarSource(source), nil
	}
	// A routed output's Destination is the route-owned coordinate whose row
	// content names the published factor row. selectedRoute proves that this
	// column is paired with publication.Relation and its key; the address here
	// must nevertheless be the retained destination cell, never the candidate
	// or the factor's first address column by convention.
	projection, projectionErr := resolver.registry.Projection(resolver.site(path+".destination"), destinationName)
	if projectionErr != nil || projection.Role != member.Destination || projection.Column != destinationColumn || projection.Relation != destinationRelation || !destinationUsesWriterKeyCarrier(projection, publication) {
		return algebra.OutputAddress{}, refuse(resolver.site(path+".destination"), destinationName, KindAddress, ReasonForeign)
	}
	if !resolver.destinationCoordinate(path, destinationName, destinationRelation, destinationColumn, layout) {
		return algebra.OutputAddress{}, refuse(resolver.site(path+".destination"), destinationName, KindAddress, ReasonForeign)
	}
	sourceOrdinal, ordinalOK := resolver.routeSourceOrdinal(output, joinOccurrences, candidate, layout)
	source, sourceOK := resolver.columnSource(destinationColumn, sourceOrdinal, layout)
	if !ordinalOK || !sourceOK || !resolver.sourceRetainsRelation(layout, source, destinationRelation) {
		return algebra.OutputAddress{}, refuse(resolver.site(path+".source"), destinationName, KindAddress, ReasonUndeclared)
	}
	if address, addressOK := resolver.deliveryAddress(semantic, source, destinationRelation); addressOK {
		return address, nil
	}
	return algebra.OutputAddress{}, refuse(resolver.site(path+".source.delivery"), destinationName, KindAddress, ReasonForeign)
}

// destinationUsesWriterKeyCarrier authenticates the destination projection
// against the writer's sealed output signature.  Result is the row/key
// carrier, not the fact carrier stored in the writer column; comparing it to
// a writer fact identity would reject a canonical routed destination (or,
// worse, permit a projection that only happens to share a fact type).
func destinationUsesWriterKeyCarrier(projection ProjectionBinding, publication Publication) bool {
	return projection.Result.Available() && publication.Result.Available() && projection.Result == publication.Result
}

// destinationColumn resolves the owner-issued projection named by a Program
// output and returns the relation that owns that column. The registry's
// columnOwner map is the only relation/column identity join; no axis or
// candidate identity is reconstructed here.
func (resolver ruleResolver) destinationColumn(path string, name Name) (model.ColumnID, model.RelationID, bool) {
	projection, err := resolver.registry.Projection(resolver.site(path+".destination"), name)
	if err != nil {
		return model.ColumnID{}, model.RelationID{}, false
	}
	return projection.Column, projection.Relation, true
}

func projectionRelationName(registry *Registry, relation model.RelationID) Name {
	for name, entry := range registry.relations {
		if entry.id == relation {
			return name
		}
	}
	return Name{}
}

// columnSource finds the exact retained cell for an owner projection. A
// repeated relation or column is not collapsed: each tuple cell is matched by
// its sealed physical source ordinal and the authored destination column.
func (resolver ruleResolver) columnSource(column model.ColumnID, sourceOrdinal uint32, layout tupleLayout) (algebra.SlotSource, bool) {
	var found algebra.SlotSource
	seen := false
	for index, cell := range layout.cells {
		if cell.column != column || cell.source != sourceOrdinal {
			continue
		}
		found = algebra.NewSlotSource(0, uint32(index))
		if seen {
			// Two cells with the same owner column in one retained occurrence are
			// not one address; the owner must publish a unique physical cell.
			return algebra.SlotSource{}, false
		}
		seen = true
	}
	return found, seen
}

func candidateSourceOrdinal(occurrences []ReadOccurrence, candidate model.RelationID, layout tupleLayout) (uint32, bool) {
	for _, occurrence := range occurrences {
		if occurrence.Candidate() {
			// CandidateOccurrence is the sealed base source. Its physical source
			// is obtained from the common occurrence vocabulary rather than
			// repeated at each destination consumer.
			return resolveOccurrenceSource(occurrence, candidate, layout)
		}
	}
	return 0, false
}

// routeSourceOrdinal resolves the authored Program join to the retained
// physical tuple source. A Selected route may lower into more than one
// JoinSpec, so the first occurrence in the authored join group is the source
// whose row carries CoordinateDestination; no dense RouteJoin+1 guess is
// permitted after lowering.
func (resolver ruleResolver) routeSourceOrdinal(output ruleprogram.OutputDecl, groups [][]ReadOccurrence, candidate model.RelationID, layout tupleLayout) (uint32, bool) {
	if output.Mode != ruleprogram.ModeRoute || !output.RouteJoinPresent || int(output.RouteJoin) >= len(groups) {
		return 0, false
	}
	occurrences := groups[output.RouteJoin]
	if len(occurrences) == 0 {
		return 0, false
	}
	return resolveOccurrenceSource(occurrences[0], candidate, layout)
}

func (resolver ruleResolver) sourceRetainsRelation(layout tupleLayout, source algebra.SlotSource, relation model.RelationID) bool {
	index := int(source.Cell())
	return index >= 0 && index < len(layout.cells) && int(source.Child()) == 0 && int(layout.cells[index].source) < len(layout.sources) && layout.sources[layout.cells[index].source] == relation
}

// sourceSupplies proves that the destination cell is part of one declared
// Apply input occurrence. It is deliberately based on the sealed occurrence
// and semantic relation, not a relation-name scan that could pick a sibling
// occurrence. The source column itself may be a destination projection rather
// than the reducer's input column, so the proof checks its physical source
// against the authored occurrence and then checks delivery below.
func (resolver ruleResolver) sourceSupplies(semantic signature.Signature, occurrences []ReadOccurrence, layout tupleLayout, source algebra.SlotSource, relation model.RelationID, candidateRef member.CandidateRef, candidate model.RelationID) bool {
	cellIndex := int(source.Cell())
	if cellIndex < 0 || cellIndex >= len(layout.cells) {
		return false
	}
	physical := layout.cells[cellIndex].source
	// An exact fold may publish at the candidate row itself while its reducer
	// inputs are projections of that candidate's dependent relations. The
	// candidate row is an authored Apply source even when it is not a semantic
	// reducer argument; the candidate-provider equality above is the authority
	// that makes this exception sound.
	candidateSource, candidateOK := candidateSourceOrdinal([]ReadOccurrence{CandidateOccurrence()}, candidate, layout)
	if relation == candidate && candidateOK && physical == candidateSource && !candidateRef.Issued() && candidateRef.Available() {
		return true
	}
	for index, occurrence := range occurrences {
		if index >= semantic.InputLen() || !occurrence.available(len(layout.sources)-1) {
			continue
		}
		input, ok := semantic.InputAt(index)
		if !ok || input.Relation != relation {
			continue
		}
		inputSource, sourceOK := resolveOccurrenceSource(occurrence, candidate, layout)
		if !sourceOK || int(inputSource) >= len(layout.sources) || layout.sources[inputSource] != input.Relation {
			continue
		}
		if inputSource == physical {
			return true
		}
	}
	return false
}

// destinationCoordinate verifies the relation's semantic coordinate role.
// Route output declarations name CoordinateDestination, while exact output
// declarations name CoordinateAddress. The role is an owner statement and is
// checked against the exact projection column before it can become an Apply
// source.
func (resolver ruleResolver) destinationCoordinate(path string, name Name, relation model.RelationID, column model.ColumnID, layout tupleLayout) bool {
	relationName := projectionRelationName(resolver.registry, relation)
	declared, err := resolver.registry.Addressed(resolver.site(path+".destination"), relationName, CoordinateDestination)
	if err != nil || declared != column {
		return false
	}
	// The relation identity must actually be retained in the tuple. The
	// source membership check here prevents a detached projection from being
	// redeemed at runtime.
	for _, source := range layout.sources {
		if source == relation {
			return true
		}
	}
	return false
}

// deliveryAddress maps the retained destination cell's authored semantic
// delivery to the closed algebra source form. Exactly-one/optional scalar
// deliveries use a scalar source; complete bounded-many delivery uses a span.
// It never infers a form from cardinality alone.
func (resolver ruleResolver) deliveryAddress(semantic signature.Signature, source algebra.SlotSource, relation model.RelationID) (algebra.OutputAddress, bool) {
	for index := 0; index < semantic.InputLen(); index++ {
		input, ok := semantic.InputAt(index)
		if !ok || input.Relation != relation {
			continue
		}
		if input.Delivery.IsScalar() {
			return algebra.ScalarSource(source), true
		}
		if input.Delivery.IsComplete() && semantic.Cardinality().Kind() == model.BoundedMany {
			return algebra.SpanSource(source), true
		}
	}
	return algebra.OutputAddress{}, false
}

// addressForRelation is the common source proof for terminal and carried
// operations.  The target relation is already an owner-issued identity: the
// caller chooses it from the authored destination projection (ordinary write)
// or from the publication relation (routed/carry write).
func (resolver ruleResolver) addressForRelation(path string, semantic signature.Signature, occurrences []ReadOccurrence, candidate model.RelationID, joins []JoinSpec, target model.RelationID) (algebra.OutputAddress, error) {
	layout, ok := resolver.layout(candidate, joins)
	if !ok {
		return algebra.OutputAddress{}, refuse(resolver.site(path), Name{Entry: resolver.entry}, KindAddress, ReasonUndeclared)
	}
	sources, err := applySlotSources(occurrences, semantic, layout, len(joins))
	if err != nil {
		return algebra.OutputAddress{}, refuse(resolver.site(path), Name{Entry: resolver.entry}, KindAddress, ReasonUndeclared)
	}

	// The caller supplies the authored occurrence list. Resolve the target
	// source from that list rather than selecting the first retained source
	// whose nominal relation happens to match.
	targetSource := -1
	for _, occurrence := range occurrences {
		source, sourceOK := resolveOccurrenceSource(occurrence, candidate, layout)
		if !sourceOK || int(source) >= len(layout.sources) || layout.sources[source] != target {
			continue
		}
		if targetSource >= 0 && targetSource != int(source) {
			return algebra.OutputAddress{}, refuse(resolver.site(path+".source"), Name{Entry: resolver.entry}, KindAddress, ReasonForeign)
		}
		targetSource = int(source)
	}
	if targetSource < 0 {
		return algebra.OutputAddress{}, refuse(resolver.site(path+".source"), Name{Entry: resolver.entry}, KindAddress, ReasonUndeclared)
	}
	selected, selectedOK := resolver.addressSource(target, targetSource, layout)
	if !selectedOK {
		return algebra.OutputAddress{}, refuse(resolver.site(path+".source"), Name{Entry: resolver.entry}, KindAddress, ReasonUndeclared)
	}
	found := false
	for index := 0; index < semantic.InputLen(); index++ {
		input, inputOK := semantic.InputAt(index)
		if !inputOK || input.Relation != target {
			continue
		}
		candidateSource := sources[index]
		if int(candidateSource.Cell()) >= len(layout.cells) || int(layout.cells[candidateSource.Cell()].source) != targetSource {
			// Repeated nominal relations may represent distinct physical
			// occurrences. The operation's input must actually retain the
			// destination row, rather than merely sharing its nominal relation.
			continue
		}
		found = true
	}
	if !found {
		return algebra.OutputAddress{}, refuse(resolver.site(path+".source"), Name{Entry: resolver.entry}, KindAddress, ReasonUndeclared)
	}

	// A scalar destination is valid only for scalar delivery.  A complete
	// denominator may be consumed sequentially only when the sealed operation
	// also bounds its output; an unbounded or partial range cannot become a
	// destination by convention.
	for index := 0; index < semantic.InputLen(); index++ {
		input, inputOK := semantic.InputAt(index)
		if !inputOK || input.Relation != target || int(sources[index].Cell()) >= len(layout.cells) || int(layout.cells[sources[index].Cell()].source) != targetSource {
			continue
		}
		if input.Delivery.IsScalar() {
			return algebra.ScalarSource(selected), nil
		}
		if input.Delivery.IsComplete() && semantic.Cardinality().Kind() == model.BoundedMany {
			return algebra.SpanSource(selected), nil
		}
	}
	return algebra.OutputAddress{}, refuse(resolver.site(fmt.Sprintf("%s.source.delivery", path)), Name{Entry: resolver.entry}, KindAddress, ReasonForeign)
}

// addressSource finds the owner-declared address cell of one retained
// relation.  The coordinate is a semantic owner statement, not a dense slot
// convention; the tuple layout only supplies its already-sealed cell index.
func (resolver ruleResolver) addressSource(relation model.RelationID, sourceOrdinal int, layout tupleLayout) (algebra.SlotSource, bool) {
	var address model.ColumnID
	if sourceOrdinal < 0 || sourceOrdinal >= len(layout.sources) || layout.sources[sourceOrdinal] != relation {
		return algebra.SlotSource{}, false
	}
	for _, entry := range resolver.registry.relations {
		if entry.id != relation {
			continue
		}
		var ok bool
		address, ok = entry.coordinates[CoordinateAddress]
		if !ok {
			return algebra.SlotSource{}, false
		}
		for index, cell := range layout.cells {
			if int(cell.source) == sourceOrdinal && cell.column == address {
				return algebra.NewSlotSource(0, uint32(index)), true
			}
		}
		return algebra.SlotSource{}, false
	}
	return algebra.SlotSource{}, false
}

// layout reconstructs the cold tuple coordinates from the already-resolved
// relation identities.  It is deliberately the same layout vocabulary used
// by lowerExpression; it does not create a runtime index or a second source
// representation.
func (resolver ruleResolver) layout(candidate model.RelationID, joins []JoinSpec) (tupleLayout, bool) {
	columns := make(map[model.RelationID][]model.ColumnID, len(resolver.registry.relations))
	for _, entry := range resolver.registry.relations {
		columns[entry.id] = append([]model.ColumnID(nil), entry.columns...)
	}
	result, ok := inputLayout(candidate, columns)
	if !ok {
		return tupleLayout{}, false
	}
	for _, join := range joins {
		right, rightOK := inputLayout(join.Relation, columns)
		if !rightOK {
			return tupleLayout{}, false
		}
		result = joinLayout(result, right)
	}
	return result, true
}
