package engine

// provenanceRow retains the exact observation identities for one live
// materialized Product or Query row. Refinement links to its source row;
// freezing later exposes the identities in read-major columns.
type provenanceRow struct {
	prefix *provenanceNode
}

// provenanceNode is one persistent extension of an exact materialized row.
// There is one node per materialized (read, row) pair.
type provenanceNode struct {
	previous *provenanceNode
	read     int
	id       uint64
}

func extendProvenance(row provenanceRow, read int, id uint64) provenanceRow {
	return provenanceRow{prefix: &provenanceNode{previous: row.prefix, read: read, id: id}}
}

// provenanceID resolves a row's exact identity before or after freezing.
func provenanceID(rows []provenanceRow, columns [][]uint64, row, read, readCount int) (uint64, bool) {
	if row < 0 || row >= len(rows) || read < 0 || read >= readCount {
		return 0, false
	}
	if len(columns) != 0 {
		if read >= len(columns) || row >= len(columns[read]) || columns[read][row] == 0 {
			return 0, false
		}
		return columns[read][row], true
	}
	for prefix := rows[row].prefix; prefix != nil; prefix = prefix.previous {
		if prefix.read == read {
			return prefix.id, prefix.id != 0
		}
		if prefix.read < read {
			break
		}
	}
	return 0, false
}

// freezeProvenanceColumns converts a prefix forest into compact read-major
// columns exactly once. It keeps precisely one identity per (read, row) pair.
func freezeProvenanceColumns(checkpoint func() bool, rows []provenanceRow, readCount int) ([][]uint64, bool) {
	if checkpoint == nil || readCount < 0 || !checkpoint() {
		return nil, false
	}
	if readCount == 0 {
		return nil, true
	}
	columns := make([][]uint64, readCount)
	for read := range columns {
		columns[read] = make([]uint64, len(rows))
	}
	for row := range rows {
		if !checkpoint() {
			return nil, false
		}
		for prefix := rows[row].prefix; prefix != nil; prefix = prefix.previous {
			if prefix.read < 0 || prefix.read >= len(columns) || prefix.id == 0 || columns[prefix.read][row] != 0 {
				return nil, false
			}
			columns[prefix.read][row] = prefix.id
		}
		for read := range columns {
			if columns[read][row] == 0 {
				return nil, false
			}
		}
		rows[row].prefix = nil
	}
	return columns, true
}
