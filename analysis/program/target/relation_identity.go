package target

import (
	"errors"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"sort"

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
	c.inputFormalRanges = make([]indexRange, len(c.operations))
	for operationIndex, row := range c.operations {
		start, err := checkedStoredRange("semantic input formal table", len(c.inputFormalIDs), c.ValuesCount(row.input))
		if err != nil {
			return err
		}
		op := vocabulary.Operation(operationIndex + 1)
		operationID := c.operationContentIDs[operationIndex]
		for formal := 0; formal < c.ValuesCount(row.input); formal++ {
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
			c.inputFormalIDs = append(c.inputFormalIDs, id)
			c.inputFormalIndex = append(c.inputFormalIndex, inputFormalIDRow{id: id, op: op, formal: selector})
		}
		c.inputFormalRanges[operationIndex] = start
	}

	c.outcomeResultRanges = make([]indexRange, len(c.outcomes))
	for outcomeIndex, row := range c.outcomes {
		count := c.ValuesCount(row.values)
		start, err := checkedStoredRange("semantic outcome result table", len(c.outcomeResultIDs), count)
		if err != nil {
			return err
		}
		owner := outcomeOwners[outcomeIndex]
		ordinal := outcomeOrdinals[outcomeIndex]
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
			c.outcomeResultIDs = append(c.outcomeResultIDs, id)
			c.outcomeResultIndex = append(c.outcomeResultIndex, outcomeResultIDRow{id: id, op: owner, outcome: ordinal, result: selector})
		}
		c.outcomeResultRanges[outcomeIndex] = start
	}
	if err := sortInputFormalIDs(c.inputFormalIndex); err != nil {
		return err
	}
	if err := sortOutcomeResultIDs(c.outcomeResultIndex); err != nil {
		return err
	}
	c.initialValueContentIDs = make([]identity.ContentID, len(c.initialValues))
	for index := range c.initialValues {
		value := vocabulary.InitialValue(index + 1)
		id, err := c.semanticID(semanticInitialValue, func(w *framing.Writer) error { return c.encodeInitialValueContent(w, value) })
		if err != nil {
			return err
		}
		c.initialValueContentIDs[index] = id
	}
	boot, err := c.semanticID(semanticBootRelation, func(w *framing.Writer) error { return c.encodeBootRelation(w) })
	if err != nil {
		return err
	}
	c.bootRelationID = boot
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
func sortInputFormalIDs(rows []inputFormalIDRow) error {
	sort.Slice(rows, func(i, j int) bool { return compareSemanticID(rows[i].id, rows[j].id) < 0 })
	for i := 1; i < len(rows); i++ {
		if compareSemanticID(rows[i-1].id, rows[i].id) == 0 {
			return errors.New("target: duplicate semantic input formal identity")
		}
	}
	return nil
}
func sortOutcomeResultIDs(rows []outcomeResultIDRow) error {
	sort.Slice(rows, func(i, j int) bool { return compareSemanticID(rows[i].id, rows[j].id) < 0 })
	for i := 1; i < len(rows); i++ {
		if compareSemanticID(rows[i-1].id, rows[i].id) == 0 {
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

// encodeInitialValueContent is a value relation, not an environment digest.
// Operation values use their binding/path anchor, so unrelated operation body
// edits do not churn boot cells that merely name the operation.
func (c *Contract) encodeInitialValueContent(w *framing.Writer, value vocabulary.InitialValue) error {
	row, ok := c.initialValue(value)
	if !ok {
		return errors.New("target: malformed initial value")
	}
	if err := w.Uint(uint64(row.kind)); err != nil {
		return err
	}
	switch row.kind {
	case vocabulary.InitialValueNil, vocabulary.InitialValueAbsent:
		return nil
	case vocabulary.InitialValueBoolean:
		return w.Bool(row.boolean)
	case vocabulary.InitialValueInteger:
		return w.Uint(uint64(row.integer))
	case vocabulary.InitialValueFloat:
		return w.Uint(row.floatBits)
	case vocabulary.InitialValueString:
		return w.String(row.string)
	case vocabulary.InitialValueRoot:
		if row.root == 0 || int(row.root) > len(c.initialRoots) {
			return errors.New("target: malformed initial value root")
		}
		return w.String(c.initialRoots[row.root-1].identity)
	case vocabulary.InitialValueOperation:
		anchor, ok := c.anchor(row.operation)
		if !ok {
			return errors.New("target: malformed initial value operation")
		}
		return w.Bytes(anchor[:])
	case vocabulary.InitialValueDeniedOperation:
		binding, ok := c.initialValueBinding(value)
		if !ok {
			return errors.New("target: malformed denied initial value")
		}
		if err := w.Uint(uint64(binding.namespace)); err != nil {
			return err
		}
		if err := w.Count(uint64(binding.ownerKeys.len())); err != nil {
			return err
		}
		for i := binding.ownerKeys.start; i < binding.ownerKeys.end; i++ {
			if err := encodeExactKey(w, c, c.bindingKeys[i]); err != nil {
				return err
			}
		}
		if err := w.Count(uint64(binding.memberKeys.len())); err != nil {
			return err
		}
		for i := binding.memberKeys.start; i < binding.memberKeys.end; i++ {
			if err := encodeExactKey(w, c, c.bindingKeys[i]); err != nil {
				return err
			}
		}
		return nil
	default:
		return errors.New("target: invalid initial value kind")
	}
}

// encodeBootRelation commits only the root topology Host must establish:
// root identities/shapes and metatable attachments. Global rows are composed
// later by Host from selected values and InitialValueContentID.
func (c *Contract) encodeBootRelation(w *framing.Writer) error {
	if err := w.Count(uint64(len(c.initialRoots))); err != nil {
		return err
	}
	for index, root := range c.initialRoots {
		if err := w.String(root.identity); err != nil {
			return err
		}
		if root.shape == 0 || int(root.shape) > len(c.bootShapes) {
			return errors.New("target: malformed boot root shape")
		}
		shape := c.bootShapes[root.shape-1]
		if shape.root != vocabulary.InitialRoot(index+1) {
			return errors.New("target: malformed boot root relation")
		}
		if err := w.Uint(uint64(shape.aggregate)); err != nil {
			return err
		}
		if err := w.Bool(shape.immutable); err != nil {
			return err
		}
		if shape.value == 0 || int(shape.value) > len(c.initialValueContentIDs) {
			return errors.New("target: malformed boot root value")
		}
		value := c.initialValueContentIDs[shape.value-1]
		if err := w.Bytes(value[:]); err != nil {
			return err
		}
	}
	if err := w.Count(uint64(len(c.initialMetatables))); err != nil {
		return err
	}
	for _, attachment := range c.initialMetatables {
		if attachment.metatable == 0 || int(attachment.metatable) > len(c.initialRoots) {
			return errors.New("target: malformed initial metatable")
		}
		if err := w.Uint(uint64(attachment.base)); err != nil {
			return err
		}
		if err := w.String(c.initialRoots[attachment.metatable-1].identity); err != nil {
			return err
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
	i := sort.Search(len(c.inputFormalIndex), func(i int) bool { return compareSemanticID(c.inputFormalIndex[i].id, id) >= 0 })
	if i >= len(c.inputFormalIndex) || compareSemanticID(c.inputFormalIndex[i].id, id) != 0 {
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
	i := sort.Search(len(c.outcomeResultIndex), func(i int) bool { return compareSemanticID(c.outcomeResultIndex[i].id, id) >= 0 })
	if i >= len(c.outcomeResultIndex) || compareSemanticID(c.outcomeResultIndex[i].id, id) != 0 {
		return 0, 0, 0, false
	}
	x := c.outcomeResultIndex[i]
	if _, ok := c.OutcomeResultID(x.op, int(x.outcome), int(x.result)); !ok {
		return 0, 0, 0, false
	}
	return x.op, int(x.outcome), int(x.result), true
}

// InitialValueContentID is a portable identity for one exact sealed boot
// value; it deliberately says nothing about which global row selected it.
func (c *Contract) InitialValueContentID(value vocabulary.InitialValue) (identity.ContentID, bool) {
	if c == nil || !c.sealed || value == 0 || int(value) > len(c.initialValueContentIDs) {
		return identity.ContentID{}, false
	}
	return c.initialValueContentIDs[value-1], true
}

// BootRelationID commits roots, identities, shapes, and metatable attachments.
func (c *Contract) BootRelationID() (identity.ContentID, bool) {
	if c == nil || !c.sealed || !c.bootRelationID.Available() {
		return identity.ContentID{}, false
	}
	return c.bootRelationID, true
}
