package contract

import (
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/framing"
)

func validIdentityRange(value indexRange, length int) bool {
	return uint64(value.start) <= uint64(value.end) && uint64(value.end) <= uint64(length)
}

// sealHostIdentityRelations is deliberately after operation/outcome identity
// finalization. Its indexes are sorted immutable value tables, while the
// direct paths remain dense operation/outcome ranges.
func (c *Contract) sealHostIdentityRelations(outcomeOwners []vocabulary.Operation, outcomeOrdinals []uint32) error {
	c.inputFormalRanges = make([]indexRange, c.Operations.OperationCount())
	for operationIndex := 0; operationIndex < c.Operations.OperationCount(); operationIndex++ {
		op := vocabulary.Operation(operationIndex + 1)
		input, inputOK := c.Operations.Input(op)
		if !inputOK {
			return errors.New("target: malformed semantic input")
		}
		start, err := checkedStoredRange("semantic input formal table", len(c.inputFormalIDs), c.Operations.ValuesCount(input))
		if err != nil {
			return err
		}
		operationID := c.operationContentIDs[operationIndex]
		for formal := 0; formal < c.Operations.ValuesCount(input); formal++ {
			selector := vocabulary.ValueFormal(formal)
			id, err := c.semanticID(semanticInputFormal, func(w *framing.Writer) error {
				if err := w.Bytes(operationID[:]); err != nil {
					return err
				}
				return w.Uint(uint64(selector))
			})
			if err != nil {
				return err
			}
			idIndex := start.start + uint32(formal)
			c.inputFormalIDs = append(c.inputFormalIDs, id)
			c.inputFormalIndex = append(c.inputFormalIndex, inputFormalIDRow{index: idIndex, op: op, formal: selector})
		}
		c.inputFormalRanges[operationIndex] = start
	}

	c.outcomeResultRanges = make([]indexRange, len(c.outcomeContentIDs))
	for outcomeIndex := range c.outcomeContentIDs {
		if outcomeIndex >= len(outcomeOwners) || outcomeIndex >= len(outcomeOrdinals) {
			return errors.New("target: malformed semantic outcome columns")
		}
		owner := outcomeOwners[outcomeIndex]
		ordinal := outcomeOrdinals[outcomeIndex]
		_, values, outcomeOK := c.Operations.OutcomeAt(owner, int(ordinal))
		if !outcomeOK {
			return errors.New("target: malformed semantic outcome")
		}
		count := c.Operations.ValuesCount(values)
		start, err := checkedStoredRange("semantic outcome result table", len(c.outcomeResultIDs), count)
		if err != nil {
			return err
		}
		if owner == 0 {
			return errors.New("target: malformed semantic outcome owner")
		}
		outcomeID := c.outcomeContentIDs[outcomeIndex]
		for result := 0; result < count; result++ {
			selector := uint32(result)
			id, err := c.semanticID(semanticOutcomeResult, func(w *framing.Writer) error {
				if err := w.Bytes(outcomeID[:]); err != nil {
					return err
				}
				return w.Uint(uint64(selector))
			})
			if err != nil {
				return err
			}
			idIndex := start.start + selector
			c.outcomeResultIDs = append(c.outcomeResultIDs, id)
			c.outcomeResultIndex = append(c.outcomeResultIndex, outcomeResultIDRow{index: idIndex, op: owner, outcome: ordinal, result: selector})
		}
		c.outcomeResultRanges[outcomeIndex] = start
	}
	if err := sortInputFormalIDs(c.inputFormalIDs, c.inputFormalIndex); err != nil {
		return err
	}
	if err := sortOutcomeResultIDs(c.outcomeResultIDs, c.outcomeResultIndex); err != nil {
		return err
	}
	if err := sortCallbackContentIDs(c.callbackContentIndex); err != nil {
		return err
	}
	if err := sortResumeContentIDs(c.resumeContentIndex); err != nil {
		return err
	}
	return nil
}

func compareSemanticID(left, right identity.ContentID) int {
	for i := range left {
		if left[i] < right[i] {
			return -1
		}
		if left[i] > right[i] {
			return 1
		}
	}
	return 0
}
func sortInputFormalIDs(ids []identity.ContentID, rows []inputFormalIDRow) error {
	for _, row := range rows {
		if uint64(row.index) >= uint64(len(ids)) {
			return errors.New("target: malformed semantic input formal index")
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		return compareSemanticID(ids[rows[i].index], ids[rows[j].index]) < 0
	})
	for i := 1; i < len(rows); i++ {
		if compareSemanticID(ids[rows[i-1].index], ids[rows[i].index]) == 0 {
			return errors.New("target: duplicate semantic input formal identity")
		}
	}
	return nil
}
func sortOutcomeResultIDs(ids []identity.ContentID, rows []outcomeResultIDRow) error {
	for _, row := range rows {
		if uint64(row.index) >= uint64(len(ids)) {
			return errors.New("target: malformed semantic outcome result index")
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		return compareSemanticID(ids[rows[i].index], ids[rows[j].index]) < 0
	})
	for i := 1; i < len(rows); i++ {
		if compareSemanticID(ids[rows[i-1].index], ids[rows[i].index]) == 0 {
			return errors.New("target: duplicate semantic outcome result identity")
		}
	}
	return nil
}

func sortCallbackContentIDs(rows []callbackContentIDRow) error {
	sort.Slice(rows, func(i, j int) bool { return compareSemanticID(rows[i].id, rows[j].id) < 0 })
	for i := 1; i < len(rows); i++ {
		if compareSemanticID(rows[i-1].id, rows[i].id) == 0 {
			return errors.New("target: duplicate semantic callback content identity")
		}
	}
	return nil
}

func sortResumeContentIDs(rows []resumeContentIDRow) error {
	sort.Slice(rows, func(i, j int) bool { return compareSemanticID(rows[i].id, rows[j].id) < 0 })
	for i := 1; i < len(rows); i++ {
		if compareSemanticID(rows[i-1].id, rows[i].id) == 0 {
			return errors.New("target: duplicate semantic resume content identity")
		}
	}
	return nil
}

// InputFormalID identifies one exact fixed input ABI slot.
func (c *Contract) InputFormalID(op vocabulary.Operation, formal vocabulary.ValueFormal) (identity.ContentID, bool) {
	if c == nil || !c.sealed || op == 0 || int(op) > len(c.inputFormalRanges) {
		return identity.ContentID{}, false
	}
	r := c.inputFormalRanges[op-1]
	if uint64(formal) >= uint64(r.len()) {
		return identity.ContentID{}, false
	}
	return c.inputFormalIDs[r.start+uint32(formal)], true
}

// FindInputFormalID is the allocation-free O(log n) inverse over this Contract.
func (c *Contract) FindInputFormalID(id identity.ContentID) (vocabulary.Operation, vocabulary.ValueFormal, bool) {
	if c == nil || !c.sealed || !id.Available() {
		return 0, 0, false
	}
	i := sort.Search(len(c.inputFormalIndex), func(i int) bool {
		row := c.inputFormalIndex[i]
		if uint64(row.index) >= uint64(len(c.inputFormalIDs)) {
			return true
		}
		return compareSemanticID(c.inputFormalIDs[row.index], id) >= 0
	})
	if i >= len(c.inputFormalIndex) || uint64(c.inputFormalIndex[i].index) >= uint64(len(c.inputFormalIDs)) || compareSemanticID(c.inputFormalIDs[c.inputFormalIndex[i].index], id) != 0 {
		return 0, 0, false
	}
	x := c.inputFormalIndex[i]
	if _, ok := c.InputFormalID(x.op, x.formal); !ok {
		return 0, 0, false
	}
	return x.op, x.formal, true
}

// OutcomeResultID identifies one fixed result slot of an exact Outcome case.
func (c *Contract) OutcomeResultID(op vocabulary.Operation, outcome, result int) (identity.ContentID, bool) {
	if c == nil || !c.sealed {
		return identity.ContentID{}, false
	}
	i, ok := c.outcomeIndex(op, outcome)
	if !ok || i >= len(c.outcomeResultRanges) {
		return identity.ContentID{}, false
	}
	r := c.outcomeResultRanges[i]
	if result < 0 || result >= r.len() {
		return identity.ContentID{}, false
	}
	return c.outcomeResultIDs[r.start+uint32(result)], true
}

// FindOutcomeResultID is the allocation-free O(log n) inverse over this Contract.
func (c *Contract) FindOutcomeResultID(id identity.ContentID) (vocabulary.Operation, int, int, bool) {
	if c == nil || !c.sealed || !id.Available() {
		return 0, 0, 0, false
	}
	i := sort.Search(len(c.outcomeResultIndex), func(i int) bool {
		row := c.outcomeResultIndex[i]
		if uint64(row.index) >= uint64(len(c.outcomeResultIDs)) {
			return true
		}
		return compareSemanticID(c.outcomeResultIDs[row.index], id) >= 0
	})
	if i >= len(c.outcomeResultIndex) || uint64(c.outcomeResultIndex[i].index) >= uint64(len(c.outcomeResultIDs)) || compareSemanticID(c.outcomeResultIDs[c.outcomeResultIndex[i].index], id) != 0 {
		return 0, 0, 0, false
	}
	x := c.outcomeResultIndex[i]
	if _, ok := c.OutcomeResultID(x.op, int(x.outcome), int(x.result)); !ok {
		return 0, 0, 0, false
	}
	return x.op, int(x.outcome), int(x.result), true
}
