package program

import "github.com/wippyai/go-lua/program/keyspace"

// returnCatalog is the Program-issued executable denominator for Return
// occurrences. Authored Returns are intentionally broader: they retain dead
// source structure. Transformer consumers must never infer liveness by
// treating an authored index that fails to issue as absence or corruption.
// Publication therefore validates the authored relation once and retains only
// the exact executable rows in dense order.
type returnCatalog struct {
	owner  *Program
	input  TransformerInput
	rows   []returnCatalogRow
	sealed bool
}

type returnCatalogRow struct {
	term       keyspace.Term
	span       Span
	valuesSpan Span
	body       Body
}

func buildReturnCatalog(owner *Program) (*returnCatalog, bool) {
	if owner == nil {
		return nil, false
	}
	input := owner.TransformerInput()
	if !input.Available() {
		return nil, false
	}
	returns := owner.Flow().Authored().Control().Returns()
	catalog := &returnCatalog{owner: owner, input: input, rows: make([]returnCatalogRow, 0, returns.Count())}
	for index := 0; index < returns.Count(); index++ {
		term, termOK := returns.At(index)
		_, values, relationOK := returns.Get(term)
		if !termOK || !relationOK || term == 0 || values == 0 {
			return nil, false
		}
		if !owner.Flow().Executable().Contains(term) {
			continue
		}
		span, body, occurrenceOK := input.computationSpan(term)
		valuesSpan, valuesOK := input.Span(values)
		row := returnCatalogRow{term: term, span: span, valuesSpan: valuesSpan, body: body}
		if !occurrenceOK || !valuesOK || !input.OwnsSpan(valuesSpan) || !validReturnCatalogRow(input, row) {
			return nil, false
		}
		catalog.rows = append(catalog.rows, row)
	}
	// Install before the final self-fence: ReturnOccurrence is intentionally
	// issued only through Program's single sealed catalog, never through a
	// caller-owned reconstruction.
	owner.returnCatalog = catalog
	catalog.sealed = true
	return catalog, catalog.valid()
}

func validReturnCatalogRow(input TransformerInput, row returnCatalogRow) bool {
	return row.term != 0 && input.Available() && input.OwnsSpan(row.span) &&
		input.OwnsSpan(row.valuesSpan) && input.OwnsBody(row.body) &&
		row.span.ContextID().Available() && row.valuesSpan.ContextID().Available()
}

func (catalog *returnCatalog) valid() bool {
	return catalog != nil && catalog.sealed && catalog.owner != nil &&
		catalog.owner.returnCatalog == catalog && catalog.input.owner == catalog.owner &&
		catalog.input.Available()
}
