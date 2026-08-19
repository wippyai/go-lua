package boundary

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link/internal/radix"
)

// sealValueSemanticIDs builds the one mounted substitution directory used by
// artifact consumers.  Every key is a Program-issued opaque semantic ID;
// raw Terms are consulted only inside this Link construction transaction to
// recover an already-issued Boundary ordinal.  Once sealed, Pack and other
// consumers cannot reconstruct or submit a Term.
func sealValueSemanticIDs(table *valueTable, p *program.Program, mount uint32) error {
	if table == nil || p == nil || mount == 0 || table.semantic == nil {
		return errors.New("link/boundary: invalid semantic value projection")
	}
	input := p
	add := func(id identity.ContentID, ordinal uint32, ok bool) error {
		if !id.Available() {
			return errors.New("link/boundary: unavailable semantic value identity")
		}
		if !ok || uint64(ordinal) >= uint64(len(table.rows)) {
			return errors.New("link/boundary: semantic value has no Boundary ordinal")
		}
		if table.rows[ordinal].shard != mount {
			return errors.New("link/boundary: semantic value crossed mount boundary")
		}
		key := valueSemanticKey{mount: mount, id: id}
		if existing, duplicate := table.semantic[key]; duplicate {
			if existing == ordinal {
				return nil
			}
			return errors.New("link/boundary: semantic value maps to distinct Boundary ordinals")
		}
		table.semantic[key] = ordinal
		return nil
	}
	addSpan := func(label string, id identity.ContentID, span program.Span, spanOK bool) error {
		ordinal, ordinalOK := boundaryValueForProgramSpan(table, mount, span)
		if !spanOK || !ordinalOK {
			return errors.New("link/boundary: semantic " + label + " has no Boundary ordinal")
		}
		return add(id, ordinal, true)
	}
	addTerm := func(label string, id identity.ContentID, term keyspace.Term, termOK bool) error {
		ordinal, ordinalOK := table.index.Lookup(radix.Index(mount), uint32(term))
		if !termOK || !ordinalOK || uint64(ordinal) >= uint64(len(table.rows)) {
			return errors.New("link/boundary: semantic " + label + " has no Boundary ordinal")
		}
		row := table.rows[ordinal]
		if row.shard != mount || row.term != term {
			return errors.New("link/boundary: semantic " + label + " crossed Boundary ownership")
		}
		return add(id, ordinal, true)
	}
	values := p.Flow().Authored().Values()
	for index := 0; index < values.Count(); index++ {
		term, termOK := values.At(index)
		valueID, valueOK := p.ValuesOccurrenceID(term)
		width, widthOK := values.Len(term)
		_, tailTerm, rowOK := values.Get(term)
		if !termOK || !valueOK || !widthOK || width < 0 || !rowOK {
			return fmt.Errorf("link/boundary: malformed semantic Values row=%d term=%d authored=%t identity=%t width=%d/%t relation=%t", index, term, termOK, valueOK, width, widthOK, rowOK)
		}
		if err := addTerm("Values", valueID, term, termOK); err != nil {
			return err
		}
		for memberIndex := 0; memberIndex < width; memberIndex++ {
			memberTerm, memberTermOK := values.Member(term, memberIndex)
			memberID, memberOK := p.ValuesMemberID(term, memberIndex)
			if !memberTermOK || !memberOK {
				return fmt.Errorf("link/boundary: malformed semantic Values member row=%d member=%d term=%d authored=%t identity=%t", index, memberIndex, memberTerm, memberTermOK, memberOK)
			}
			if err := addTerm("Values member", memberID, memberTerm, true); err != nil {
				return err
			}
		}
		if tailTerm != 0 {
			tailID, tailOK := p.ValuesTailID(term)
			if !tailOK {
				return errors.New("link/boundary: malformed semantic Values tail")
			}
			if err := addTerm("Values tail", tailID, tailTerm, true); err != nil {
				return err
			}
		}
	}
	storage := p.Flow().Authored().Storage().Cells()
	for index := 0; index < storage.Count(); index++ {
		term, termOK := storage.At(index)
		cellID, cellOK := p.StorageCellID(term)
		ordinal, ordinalOK := table.index.Lookup(radix.Index(mount), uint32(term))
		if !cellOK || !termOK {
			return errors.New("link/boundary: malformed semantic Cell row")
		}
		if !ordinalOK || uint64(ordinal) >= uint64(len(table.rows)) {
			return errors.New("link/boundary: semantic Cell has no Boundary ordinal")
		}
		row := table.rows[ordinal]
		if row.shard != mount || row.term != term {
			return errors.New("link/boundary: semantic Cell Boundary ordinal is not exact")
		}
		if err := add(cellID, ordinal, true); err != nil {
			return err
		}
	}
	calls := p.Flow().Authored().Calls()
	for index := 0; index < calls.Count(); index++ {
		callTerm, callTermOK := calls.At(index)
		_, calleeTerm, receiverTerm, actualsTerm, callRowOK := calls.Get(callTerm)
		callID, callOK := p.CallIDAt(index)
		if !callOK {
			// Calls without complete evaluation geometry do not produce an
			// Artifact call row and therefore have no mounted semantic inverse.
			continue
		}
		calleeID, calleeOK := p.CallCalleeIDAt(index)
		actualsID, actualsOK := p.CallActualsIDAt(index)
		valuesID, valuesOK := p.CallValuesIDAt(index)
		if !callTermOK || !callRowOK || !calleeOK || !actualsOK || !valuesOK ||
			!callID.Available() || !calleeID.Available() || !actualsID.Available() || !valuesID.Available() {
			return fmt.Errorf("link/boundary: malformed semantic Call row=%d term=%d authored=%t relation=%t call=%t callee=%t actuals=%t values=%t", index, callTerm, callTermOK, callRowOK, callOK, calleeOK, actualsOK, valuesOK)
		}
		for _, row := range []struct {
			id   identity.ContentID
			term keyspace.Term
		}{{callID, callTerm}, {calleeID, calleeTerm}, {actualsID, actualsTerm}, {valuesID, actualsTerm}} {
			if err := addTerm("Call operand", row.id, row.term, true); err != nil {
				return err
			}
		}
		if receiverTerm != 0 {
			receiverID, receiverOK := p.CallReceiverIDAt(index)
			if !receiverOK {
				return errors.New("link/boundary: malformed semantic Call receiver")
			}
			if err := addTerm("Call receiver", receiverID, receiverTerm, true); err != nil {
				return err
			}
		}
		width, widthOK := p.Flow().Authored().Values().Len(actualsTerm)
		if !widthOK || width < 0 {
			return errors.New("link/boundary: malformed semantic Call argument denominator")
		}
		for argumentIndex := 0; argumentIndex < width; argumentIndex++ {
			argumentTerm, argumentTermOK := p.Flow().Authored().Values().Member(actualsTerm, argumentIndex)
			argumentID, argumentOK := p.CallArgumentIDAt(index, argumentIndex)
			if !argumentTermOK || !argumentOK {
				return errors.New("link/boundary: malformed semantic Call argument")
			}
			if err := addTerm("Call argument", argumentID, argumentTerm, true); err != nil {
				return err
			}
		}
		_, tailTerm, valuesRowOK := p.Flow().Authored().Values().Get(actualsTerm)
		if !valuesRowOK {
			return errors.New("link/boundary: malformed semantic Call Values row")
		}
		if tailTerm != 0 {
			tailID, tailOK := p.ValuesTailID(actualsTerm)
			if !tailOK {
				return errors.New("link/boundary: malformed semantic Call tail")
			}
			if err := addTerm("Call tail", tailID, tailTerm, true); err != nil {
				return err
			}
		}
	}
	// Computation and return rows are artifact-issued from the exact authored
	// Span identities.  Publish the same semantic inverse here so mounted
	// consumers join by ModuleKey+occurrence ID rather than reopening Terms.
	// The rows are read directly from the canonical authored Flow relations;
	// only the transient Span/Body ownership proofs come from Program queries.
	computationSpan := func(term keyspace.Term) (program.Span, bool) {
		span, spanOK := input.Span(term)
		body, bodyOK := input.ContainingBody(term)
		return span, spanOK && bodyOK && input.OwnsSpan(span) && input.OwnsBody(body)
	}
	authored := p.Flow().Authored()
	executable := p.Flow().Executable()
	unaries := authored.Operators().Unaries()
	for index := 0; index < unaries.Count(); index++ {
		term, termOK := unaries.At(index)
		if !termOK || !executable.Contains(term) {
			return errors.New("link/boundary: malformed semantic Unary row")
		}
		_, _, operand, relationOK := unaries.Get(term)
		span, spanOK := computationSpan(term)
		operandSpan, operandSpanOK := input.Span(operand)
		if !relationOK || !spanOK || !operandSpanOK || !input.OwnsSpan(operandSpan) {
			return errors.New("link/boundary: malformed semantic Unary row")
		}
		if err := addSpan("Unary", span.ContextID(), span, true); err != nil {
			return errors.New("link/boundary: malformed semantic Unary row")
		}
		if err := addSpan("Unary operand", operandSpan.ContextID(), operandSpan, true); err != nil {
			return errors.New("link/boundary: malformed semantic Unary row")
		}
	}
	selects := authored.Operators().Selects()
	for index := 0; index < selects.Count(); index++ {
		term, termOK := selects.At(index)
		if !termOK || !executable.Contains(term) {
			return errors.New("link/boundary: malformed semantic Select row")
		}
		_, _, left, right, relationOK := selects.Get(term)
		span, spanOK := computationSpan(term)
		leftSpan, leftSpanOK := input.Span(left)
		rightSpan, rightSpanOK := input.Span(right)
		if !relationOK || !spanOK || !leftSpanOK || !rightSpanOK || !input.OwnsSpan(leftSpan) || !input.OwnsSpan(rightSpan) {
			return errors.New("link/boundary: malformed semantic Select row")
		}
		if err := addSpan("Select", span.ContextID(), span, true); err != nil {
			return errors.New("link/boundary: malformed semantic Select row")
		}
		if err := addSpan("Select left", leftSpan.ContextID(), leftSpan, true); err != nil {
			return errors.New("link/boundary: malformed semantic Select row")
		}
		if err := addSpan("Select right", rightSpan.ContextID(), rightSpan, true); err != nil {
			return errors.New("link/boundary: malformed semantic Select row")
		}
	}
	claims := authored.Claims()
	for index := 0; index < claims.Count(); index++ {
		term, termOK := claims.At(index)
		if !termOK || !executable.Contains(term) {
			return errors.New("link/boundary: malformed semantic Claim row")
		}
		_, operand, _, relationOK := claims.Get(term)
		span, spanOK := computationSpan(term)
		operandSpan, operandSpanOK := input.Span(operand)
		if !relationOK || !spanOK || !operandSpanOK || !input.OwnsSpan(operandSpan) {
			return errors.New("link/boundary: malformed semantic Claim row")
		}
		if err := addSpan("Claim", span.ContextID(), span, true); err != nil {
			return errors.New("link/boundary: malformed semantic Claim row")
		}
		if err := addSpan("Claim operand", operandSpan.ContextID(), operandSpan, true); err != nil {
			return errors.New("link/boundary: malformed semantic Claim row")
		}
	}
	returns := authored.Control().Returns()
	for index := 0; index < returns.Count(); index++ {
		term, termOK := returns.At(index)
		_, values, relationOK := returns.Get(term)
		if !termOK || !relationOK || term == 0 || values == 0 {
			return errors.New("link/boundary: unavailable semantic Return row")
		}
		if !executable.Contains(term) {
			continue
		}
		span, spanOK := computationSpan(term)
		valuesSpan, valuesSpanOK := input.Span(values)
		if !spanOK || !valuesSpanOK || !input.OwnsSpan(valuesSpan) || !span.ContextID().Available() || !valuesSpan.ContextID().Available() {
			return errors.New("link/boundary: unavailable semantic Return row")
		}
		// A Return is an Outcome/control occurrence, rather than a member of
		// Boundary's value universe. Its ID is consumed by the artifact and
		// Residence boundary rows, which preserve that structural identity
		// directly. Only the returned Values occurrence needs a Boundary Value
		// ordinal for Value's return transfer rule. Attempting to map the
		// Return span itself through valuePairs is a category error: the value
		// universe intentionally contains the derived Outcome terminal, not the
		// authored Return control term.
		if err := addSpan("Return values", valuesSpan.ContextID(), valuesSpan, true); err != nil {
			return err
		}
	}
	for _, family := range [...]keyspace.Family{
		keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
		keyspace.FamilyFloat, keyspace.FamilyString, keyspace.FamilyTypeValue,
	} {
		for index := 0; index < p.ValueSourceCount(family); index++ {
			sourceID, _, sourceTerm, ok := p.ValueSourceIDAt(family, index)
			if !ok {
				continue
			}
			if addTerm("ValueSource", sourceID, sourceTerm, true) != nil {
				return errors.New("link/boundary: malformed semantic ValueSource row")
			}
		}
	}
	reads := p.Flow().Authored().Storage().Reads()
	for index := 0; index < reads.Count(); index++ {
		readID, _, readTerm, ok := p.StorageReadIDAt(index)
		if !ok {
			continue
		}
		if addTerm("StorageRead", readID, readTerm, true) != nil {
			return errors.New("link/boundary: malformed semantic StorageRead row")
		}
	}
	return nil
}
