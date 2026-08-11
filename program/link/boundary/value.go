package boundary

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"math"
	"sort"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/flow"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link/internal/radix"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

const valueRelationVersion = 1

// sealValues constructs the complete canonical value universe and seals one
// sparse Term->ordinal relation per exact Project mount.  The temporary pair
// slices and all builders die at return; only the compact radix Store and
// immutable rows remain on the Boundary authority.
func sealValues(a *authority) error {
	if a == nil || a.project == nil || a.valueTable != nil {
		return errors.New("link/boundary: invalid value authority")
	}
	mounts := a.project.Mounts()
	table := &valueTable{relations: make([]keyspace.ContentID, mounts.Count())}
	var builder radix.Builder
	for mountIndex := 0; mountIndex < mounts.Count(); mountIndex++ {
		shard, ok := mounts.At(mountIndex)
		if !ok {
			return errors.New("link/boundary: malformed Project mount")
		}
		program, ok := mounts.Program(shard)
		name, nameOK := mounts.Name(shard)
		if !ok || program == nil || !nameOK || name == "" || !program.ContentID().Available() {
			return errors.New("link/boundary: unavailable mounted Program")
		}
		pairs, err := valuePairs(program, uint32(mountIndex+1), &table.rows)
		if err != nil {
			return err
		}
		relation := mountValueRelationID(name, program.ContentID(), pairs)
		if !relation.Available() {
			return errors.New("link/boundary: unavailable mount value relation identity")
		}
		table.relations[mountIndex] = relation
		if _, err := builder.AddSorted(pairs); err != nil {
			return errors.New("link/boundary: seal value index")
		}
	}
	index, err := builder.Seal()
	if err != nil {
		return errors.New("link/boundary: seal value indexes")
	}
	table.index = index
	table.content = valueAggregateID(table.relations)
	if !table.content.Available() {
		return errors.New("link/boundary: unavailable value relation identity")
	}
	if err := sealValueIDs(table); err != nil {
		return err
	}
	a.valueTable = table
	return nil
}

func sealValueIDs(table *valueTable) error {
	if table == nil || len(table.rows) > int(math.MaxUint32) {
		return errors.New("link/boundary: invalid value identity table")
	}
	table.ids = make([]valueIDRow, len(table.rows))
	for ordinal, row := range table.rows {
		if row.shard == 0 || uint64(row.shard) > uint64(len(table.relations)) {
			return errors.New("link/boundary: malformed value identity row")
		}
		id, ok := valueID(table.relations[row.shard-1], row.term)
		if !ok {
			return errors.New("link/boundary: unavailable value identity")
		}
		table.ids[ordinal] = valueIDRow{id: id, ordinal: uint32(ordinal)}
	}
	sort.Slice(table.ids, func(left, right int) bool { return bytes.Compare(table.ids[left].id[:], table.ids[right].id[:]) < 0 })
	for index := 1; index < len(table.ids); index++ {
		if table.ids[index-1].id == table.ids[index].id {
			return errors.New("link/boundary: duplicate value identity")
		}
	}
	return nil
}

func valuePairs(p *program.Program, shard uint32, rows *[]valueRow) ([]radix.Pair, error) {
	if p == nil || rows == nil || shard == 0 {
		return nil, errors.New("link/boundary: malformed Program value source")
	}
	if uint64(len(*rows)) > uint64(math.MaxUint32) {
		return nil, errors.New("link/boundary: value handle overflow")
	}
	authored := p.Flow().Authored()
	literals := p.Source().Literals()
	storage := authored.Storage()
	values := authored.Values()
	outcomes := p.Flow().Outcomes()
	pairs := make([]radix.Pair, 0, valueFamilyCount(p))
	add := func(term keyspace.Term, ok bool) error {
		if !ok || term == 0 || uint64(len(*rows)) >= uint64(math.MaxUint32) {
			return errors.New("link/boundary: malformed or overflowing Program value universe")
		}
		ordinal := uint32(len(*rows))
		*rows = append(*rows, valueRow{shard: shard, term: term})
		pairs = append(pairs, radix.Pair{Key: uint32(term), Value: ordinal})
		return nil
	}
	addFamily := func(count int, at func(int) (keyspace.Term, bool)) error {
		for index := 0; index < count; index++ {
			term, ok := at(index)
			if err := add(term, ok); err != nil {
				return err
			}
		}
		return nil
	}
	// Keep the historical canonical family order. The retained relation is
	// sorted by raw Term after construction, while Value.At remains this dense
	// family order for stable replay and deterministic diagnostics.
	families := []struct {
		count int
		at    func(int) (keyspace.Term, bool)
	}{
		{literals.Nils().Count(), func(index int) (keyspace.Term, bool) { term, _, ok := literals.Nils().At(index); return term, ok }},
		{literals.Bools().Count(), func(index int) (keyspace.Term, bool) { term, _, _, ok := literals.Bools().At(index); return term, ok }},
		{literals.Integers().Count(), func(index int) (keyspace.Term, bool) {
			term, _, _, ok := literals.Integers().At(index)
			return term, ok
		}},
		{literals.Floats().Count(), func(index int) (keyspace.Term, bool) { term, _, _, ok := literals.Floats().At(index); return term, ok }},
		{literals.Strings().Count(), func(index int) (keyspace.Term, bool) { term, _, _, ok := literals.Strings().At(index); return term, ok }},
		{storage.Reads().Count(), storage.Reads().At},
		{storage.Varargs().Count(), storage.Varargs().At},
		{authored.Operators().Unaries().Count(), authored.Operators().Unaries().At},
		{authored.Operators().Binaries().Count(), authored.Operators().Binaries().At},
		{authored.Operators().Selects().Count(), authored.Operators().Selects().At},
		{authored.Functions().Count(), authored.Functions().At},
		{authored.Calls().Count(), authored.Calls().At},
		{authored.Tables().Count(), authored.Tables().At},
		{authored.TypeValues().Count(), authored.TypeValues().At},
		{authored.Claims().Count(), authored.Claims().At},
	}
	for _, family := range families {
		if err := addFamily(family.count, family.at); err != nil {
			return nil, err
		}
	}
	for index := 0; index < values.Count(); index++ {
		term, ok := values.At(index)
		if err := add(term, ok); err != nil {
			return nil, err
		}
	}
	if err := addFamily(storage.Cells().Count(), storage.Cells().At); err != nil {
		return nil, err
	}
	for _, wanted := range []flowkind.OutcomeKind{
		flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow,
		flowkind.OutcomeYield, flowkind.OutcomeCancel,
	} {
		for index := 0; index < outcomes.Count(); index++ {
			term, ok := outcomes.At(index)
			if !ok {
				return nil, errors.New("link/boundary: malformed Program outcome sequence")
			}
			outcome, ok := outcomes.Get(term)
			if !ok {
				return nil, errors.New("link/boundary: malformed Program outcome")
			}
			if outcome.Kind == wanted {
				if err := add(term, true); err != nil {
					return nil, err
				}
			}
		}
	}
	sort.Slice(pairs, func(left, right int) bool { return pairs[left].Key < pairs[right].Key })
	if !strictPairs(pairs) {
		return nil, errors.New("link/boundary: duplicate Program value term")
	}
	return pairs, nil
}

func valueFamilyCount(p *program.Program) int {
	if p == nil {
		return 0
	}
	authored := p.Flow().Authored()
	literals := p.Source().Literals()
	storage := authored.Storage()
	values := authored.Values()
	outcomes := p.Flow().Outcomes()
	count := literals.Nils().Count() + literals.Bools().Count() + literals.Integers().Count() + literals.Floats().Count() + literals.Strings().Count() +
		storage.Reads().Count() + storage.Varargs().Count() + authored.Operators().Unaries().Count() + authored.Operators().Binaries().Count() + authored.Operators().Selects().Count() +
		authored.Functions().Count() + authored.Calls().Count() + authored.Tables().Count() + authored.TypeValues().Count() + authored.Claims().Count() +
		values.Count() + storage.Cells().Count()
	for index := 0; index < outcomes.Count(); index++ {
		term, ok := outcomes.At(index)
		if !ok {
			return 0
		}
		outcome, ok := outcomes.Get(term)
		if !ok {
			return 0
		}
		switch outcome.Kind {
		case flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel:
			count++
		}
	}
	return count
}

func strictPairs(pairs []radix.Pair) bool {
	for index := 1; index < len(pairs); index++ {
		if pairs[index-1].Key >= pairs[index].Key {
			return false
		}
	}
	return true
}

func mountValueRelationID(name string, programID keyspace.ContentID, pairs []radix.Pair) (id keyspace.ContentID) {
	if name == "" || !programID.Available() {
		return keyspace.ContentID{}
	}
	h := sha256.New()
	var writer canonical.Writer
	if writer.Reset(h, "program/link/boundary/value-mount", valueRelationVersion) != nil ||
		writer.Record(1) != nil || writer.String(name) != nil || writer.Bytes(programID[:]) != nil || writer.Count(uint64(len(pairs))) != nil {
		return keyspace.ContentID{}
	}
	for _, pair := range pairs {
		if pair.Key == 0 || writer.Uint(uint64(pair.Key)) != nil {
			return keyspace.ContentID{}
		}
	}
	if writer.Finish() != nil {
		return keyspace.ContentID{}
	}
	if sum := h.Sum(id[:0]); len(sum) != len(id) {
		return keyspace.ContentID{}
	}
	return id
}

func valueAggregateID(relations []keyspace.ContentID) (id keyspace.ContentID) {
	h := sha256.New()
	var writer canonical.Writer
	if writer.Reset(h, "program/link/boundary/values", valueRelationVersion) != nil || writer.Record(1) != nil || writer.Count(uint64(len(relations))) != nil {
		return keyspace.ContentID{}
	}
	for _, relation := range relations {
		if !relation.Available() || writer.Bytes(relation[:]) != nil {
			return keyspace.ContentID{}
		}
	}
	if writer.Finish() != nil {
		return keyspace.ContentID{}
	}
	if sum := h.Sum(id[:0]); len(sum) != len(id) {
		return keyspace.ContentID{}
	}
	return id
}

func valueID(relation keyspace.ContentID, term keyspace.Term) (id keyspace.ContentID, ok bool) {
	if !relation.Available() || term == 0 {
		return keyspace.ContentID{}, false
	}
	h := sha256.New()
	var writer canonical.Writer
	if writer.Reset(h, "program/link/boundary/value", valueRelationVersion) != nil || writer.Record(1) != nil || writer.Bytes(relation[:]) != nil || writer.Uint(uint64(term)) != nil || writer.Finish() != nil {
		return keyspace.ContentID{}, false
	}
	sum := h.Sum(id[:0])
	return id, len(sum) == len(id)
}

func (v Values) live() bool {
	return v.component != nil && v.component.authority != nil && v.component.authority.valueTable != nil
}

func (v Values) valid(value Value) bool {
	return v.live() && v.component.authority.valueTable != nil && value.component == v.component && value.ordinal < uint32(len(v.component.authority.valueTable.rows))
}

// Count reports the complete canonical Boundary value universe.
func (v Values) Count() int {
	if !v.live() {
		return 0
	}
	return len(v.component.authority.valueTable.rows)
}

// At issues one owner-fenced Boundary Value in canonical mount/family order.
func (v Values) At(index int) (Value, bool) {
	if !v.live() || index < 0 || index >= len(v.component.authority.valueTable.rows) {
		return Value{}, false
	}
	return Value{component: v.component, ordinal: uint32(index)}, true
}

// Origin returns the exact Project Shard and Program Term named by value.
func (v Values) Origin(value Value) (linkproject.Shard, keyspace.Term, bool) {
	if !v.valid(value) {
		return linkproject.Shard{}, 0, false
	}
	row := v.component.authority.valueTable.rows[value.ordinal]
	mounts := v.component.authority.project.Mounts()
	shard, ok := mounts.At(int(row.shard) - 1)
	if !ok || row.term == 0 {
		return linkproject.Shard{}, 0, false
	}
	return shard, row.term, true
}

// ID returns a replay-stable identity derived only from the source mount's
// value relation and semantic Program Term.  Target, Host, Module, Static,
// and enclosing Boundary/Link identities are intentionally absent.
func (v Values) ID(value Value) (keyspace.ContentID, bool) {
	if !v.valid(value) {
		return keyspace.ContentID{}, false
	}
	row := v.component.authority.valueTable.rows[value.ordinal]
	if row.shard == 0 || uint64(row.shard) > uint64(len(v.component.authority.valueTable.relations)) {
		return keyspace.ContentID{}, false
	}
	return valueID(v.component.authority.valueTable.relations[row.shard-1], row.term)
}

// FindID rebinds one portable existing Value identity through this exact
// finalized Boundary authority. The sealed ID index is sorted and retains no
// map or replay builder.
func (v Values) FindID(id keyspace.ContentID) (Value, bool) {
	if !v.live() || !id.Available() {
		return Value{}, false
	}
	rows := v.component.authority.valueTable.ids
	index := sort.Search(len(rows), func(index int) bool { return bytes.Compare(rows[index].id[:], id[:]) >= 0 })
	if index >= len(rows) || rows[index].id != id || uint64(rows[index].ordinal) >= uint64(len(v.component.authority.valueTable.rows)) {
		return Value{}, false
	}
	return Value{component: v.component, ordinal: rows[index].ordinal}, true
}

// Compare orders two values from this exact Boundary.
func (v Values) Compare(left, right Value) (int, bool) {
	if !v.valid(left) || !v.valid(right) {
		return 0, false
	}
	if left.ordinal < right.ordinal {
		return -1, true
	}
	if left.ordinal > right.ordinal {
		return 1, true
	}
	return 0, true
}

// Of resolves one existing Program occurrence through an exact Project Shard.
func (v Values) Of(shard linkproject.Shard, term keyspace.Term) (Value, bool) {
	if !v.live() || term == 0 {
		return Value{}, false
	}
	mounts := v.component.authority.project.Mounts()
	mountIndex, ok := mounts.Index(shard)
	if !ok || mountIndex < 0 || mountIndex >= mounts.Count() {
		return Value{}, false
	}
	ordinal, ok := v.component.authority.valueTable.index.Lookup(radix.Index(mountIndex+1), uint32(term))
	if !ok || uint64(ordinal) >= uint64(len(v.component.authority.valueTable.rows)) {
		return Value{}, false
	}
	value := Value{component: v.component, ordinal: ordinal}
	row := v.component.authority.valueTable.rows[ordinal]
	return value, row.shard == uint32(mountIndex+1) && row.term == term
}

// Callee returns the exact existing Program callee Value of one ordinary
// Project Call application. It is a direct projection only: Boundary retains
// neither a call table nor a derived callee identity.
func (v Calls) Callee(application linkproject.Application) (Value, bool) {
	if v.component == nil || v.component.authority == nil {
		return Value{}, false
	}
	applications := v.component.authority.project.Applications()
	shard, term, ok := applications.Call(application)
	if !ok || term == 0 {
		return Value{}, false
	}
	p, ok := v.component.authority.project.Mounts().Program(shard)
	if !ok || p == nil {
		return Value{}, false
	}
	_, callee, _, _, ok := p.Flow().Authored().Calls().Get(term)
	if !ok || callee == 0 {
		return Value{}, false
	}
	return (Values{component: v.component}).Of(shard, callee)
}

// CallOperands returns the exact existing Program operands of one ordinary
// Project Call Application. Receiver presence is encoded by CallForm; a zero
// Value is not interpreted as a semantic form.
func (v Calls) CallOperands(application linkproject.Application) (form flow.CallForm, receiver, actuals Value, ok bool) {
	if v.component == nil || v.component.authority == nil {
		return 0, Value{}, Value{}, false
	}
	applications := v.component.authority.project.Applications()
	shard, term, ok := applications.Call(application)
	if !ok || term == 0 {
		return 0, Value{}, Value{}, false
	}
	p, ok := v.component.authority.project.Mounts().Program(shard)
	if !ok || p == nil {
		return 0, Value{}, Value{}, false
	}
	_, _, receiverTerm, actualsTerm, ok := p.Flow().Authored().Calls().Get(term)
	if !ok || actualsTerm == 0 {
		return 0, Value{}, Value{}, false
	}
	values := Values{component: v.component}
	actuals, ok = values.Of(shard, actualsTerm)
	if !ok {
		return 0, Value{}, Value{}, false
	}
	if receiverTerm == 0 {
		return flow.CallFormPlain, Value{}, actuals, true
	}
	receiver, ok = values.Of(shard, receiverTerm)
	if !ok {
		return 0, Value{}, Value{}, false
	}
	return flow.CallFormMethod, receiver, actuals, true
}
