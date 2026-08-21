package boundary

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"math"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link/internal/radix"
	"github.com/wippyai/go-lua/internal/framing"
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
	table := &valueTable{
		relations: make([]identity.ContentID, mounts.Count()),
		spans:     make(map[valueSpanKey]uint32),
		semantic:  make(map[valueSemanticKey]uint32),
		mounts:    make(map[identity.ContentID]uint32, mounts.Count()),
	}
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
		moduleKey, moduleOK := a.project.ModuleKey(shard)
		if !moduleOK || !moduleKey.Available() {
			return errors.New("link/boundary: unavailable mounted Program identity")
		}
		if _, duplicate := table.mounts[moduleKey]; duplicate {
			return errors.New("link/boundary: duplicate mounted Program identity")
		}
		table.mounts[moduleKey] = uint32(mountIndex + 1)
		pairs, err := valuePairs(program, uint32(mountIndex+1), &table.rows)
		if err != nil {
			return err
		}
		if err := sealValueSpans(table, program, uint32(mountIndex+1), pairs); err != nil {
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
	// Semantic projections depend on the finalized Term->ordinal quotient.
	// Build them only after every mounted relation has been indexed; they are
	// still construction-only and retain no Program proof after this function.
	for mountIndex := 0; mountIndex < mounts.Count(); mountIndex++ {
		shard, shardOK := mounts.At(mountIndex)
		program, programOK := mounts.Program(shard)
		if !shardOK || !programOK || program == nil {
			return errors.New("link/boundary: unavailable mounted semantic Program")
		}
		if err := sealValueSemanticIDs(table, program, uint32(mountIndex+1)); err != nil {
			return err
		}
	}
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

func sealValueSpans(table *valueTable, p *program.Program, mount uint32, pairs []radix.Pair) error {
	if table == nil || p == nil || mount == 0 || table.spans == nil {
		return errors.New("link/boundary: invalid Program span projection")
	}
	for _, pair := range pairs {
		context, _, _, spanOK := p.EvaluationSpan(keyspace.Term(pair.Key))
		if !spanOK {
			// Some Boundary values (notably storage Cells) are subjects rather
			// than evaluable occurrences and therefore own no Entry/Finish Span.
			continue
		}
		if !context.Available() || uint64(pair.Value) >= uint64(len(table.rows)) {
			return errors.New("link/boundary: unavailable Program value span")
		}
		key := valueSpanKey{mount: mount, context: context}
		if _, duplicate := table.spans[key]; duplicate {
			return errors.New("link/boundary: duplicate Program value span")
		}
		table.spans[key] = pair.Value
	}
	return nil
}

func boundaryValueForProgramSpan(table *valueTable, mount uint32, span program.Span) (uint32, bool) {
	if table == nil || table.spans == nil || mount == 0 || !span.Available() {
		return 0, false
	}
	context := span.ContextID()
	ordinal, ok := table.spans[valueSpanKey{mount: mount, context: context}]
	if !ok || uint64(ordinal) >= uint64(len(table.rows)) {
		return 0, false
	}
	row := table.rows[ordinal]
	return ordinal, context.Available() && row.shard == mount
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
			_, outcomeKind, _, ok := outcomes.Get(term)
			if !ok {
				return nil, errors.New("link/boundary: malformed Program outcome")
			}
			if outcomeKind == wanted {
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
		_, outcomeKind, _, ok := outcomes.Get(term)
		if !ok {
			return 0
		}
		switch outcomeKind {
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

func mountValueRelationID(name string, programID identity.ContentID, pairs []radix.Pair) (id identity.ContentID) {
	if name == "" || !programID.Available() {
		return identity.ContentID{}
	}
	h := sha256.New()
	var writer framing.Writer
	if writer.Reset(h, "program/link/boundary/value-mount", valueRelationVersion) != nil ||
		writer.Record(1) != nil || writer.String(name) != nil || writer.Bytes(programID[:]) != nil || writer.Count(uint64(len(pairs))) != nil {
		return identity.ContentID{}
	}
	for _, pair := range pairs {
		if pair.Key == 0 || writer.Uint(uint64(pair.Key)) != nil {
			return identity.ContentID{}
		}
	}
	if writer.Finish() != nil {
		return identity.ContentID{}
	}
	if sum := h.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

func valueAggregateID(relations []identity.ContentID) (id identity.ContentID) {
	h := sha256.New()
	var writer framing.Writer
	if writer.Reset(h, "program/link/boundary/values", valueRelationVersion) != nil || writer.Record(1) != nil || writer.Count(uint64(len(relations))) != nil {
		return identity.ContentID{}
	}
	for _, relation := range relations {
		if !relation.Available() || writer.Bytes(relation[:]) != nil {
			return identity.ContentID{}
		}
	}
	if writer.Finish() != nil {
		return identity.ContentID{}
	}
	if sum := h.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

func valueID(relation identity.ContentID, term keyspace.Term) (id identity.ContentID, ok bool) {
	if !relation.Available() || term == 0 {
		return identity.ContentID{}, false
	}
	h := sha256.New()
	var writer framing.Writer
	if writer.Reset(h, "program/link/boundary/value", valueRelationVersion) != nil || writer.Record(1) != nil || writer.Bytes(relation[:]) != nil || writer.Uint(uint64(term)) != nil || writer.Finish() != nil {
		return identity.ContentID{}, false
	}
	sum := h.Sum(id[:0])
	return id, len(sum) == len(id)
}
