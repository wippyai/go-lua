package boundary

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"math"
	"sort"

	"github.com/wippyai/go-lua/program"
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
	table := &valueTable{
		relations: make([]keyspace.ContentID, mounts.Count()),
		spans:     make(map[valueSpanKey]uint32),
		semantic:  make(map[valueSemanticKey]uint32),
		mounts:    make(map[keyspace.ContentID]uint32, mounts.Count()),
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
	input := p.TransformerInput()
	for _, pair := range pairs {
		span, spanOK := input.Span(keyspace.Term(pair.Key))
		if !spanOK {
			// Some Boundary values (notably storage Cells) are subjects rather
			// than evaluable occurrences and therefore own no Entry/Finish Span.
			continue
		}
		context := span.ContextID()
		if !input.OwnsSpan(span) || !context.Available() || uint64(pair.Value) >= uint64(len(table.rows)) {
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

// sealValueSemanticIDs builds the one mounted substitution directory used by
// artifact consumers.  Every key is a Program-issued opaque semantic ID;
// raw Terms are consulted only inside this Link construction transaction to
// recover an already-issued Boundary ordinal.  Once sealed, Pack and other
// consumers cannot reconstruct or submit a Term.
func sealValueSemanticIDs(table *valueTable, p *program.Program, mount uint32) error {
	if table == nil || p == nil || mount == 0 || table.semantic == nil {
		return errors.New("link/boundary: invalid semantic value projection")
	}
	input := p.TransformerInput()
	add := func(id keyspace.ContentID, ordinal uint32, ok bool) error {
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
	addSpan := func(label string, id keyspace.ContentID, span program.Span, spanOK bool) error {
		ordinal, ordinalOK := boundaryValueForProgramSpan(table, mount, span)
		if !spanOK || !ordinalOK {
			return errors.New("link/boundary: semantic " + label + " has no Boundary ordinal")
		}
		return add(id, ordinal, true)
	}
	values := input.Values()
	for index := 0; index < values.Count(); index++ {
		value, valueOK := values.At(index)
		span, spanOK := value.Span()
		if !valueOK || !input.OwnsValuesOccurrence(value) {
			return errors.New("link/boundary: malformed semantic Values receipt")
		}
		if spanOK {
			if err := addSpan("Values", value.ID(), span, true); err != nil {
				return err
			}
		}
		for memberIndex := 0; memberIndex < value.Count(); memberIndex++ {
			member, memberOK := value.At(memberIndex)
			memberSpan, memberSpanOK := member.Span()
			if !memberOK || !input.OwnsValuesMember(member) || !memberSpanOK {
				return errors.New("link/boundary: malformed semantic Values member")
			}
			if err := addSpan("Values member", member.ID(), memberSpan, true); err != nil {
				return err
			}
		}
		if tail, tailOK := value.Tail(); tailOK {
			tailSpan, tailSpanOK := tail.Span()
			if !input.OwnsTailProducer(tail) || !tailSpanOK {
				return errors.New("link/boundary: malformed semantic Values tail")
			}
			if err := addSpan("Values tail", tail.ContextID(), tailSpan, true); err != nil {
				return err
			}
		}
	}
	storage := p.Flow().Authored().Storage().Cells()
	if storage.Count() != input.StorageCellCount() {
		return errors.New("link/boundary: semantic Cell denominator mismatch")
	}
	for index := 0; index < input.StorageCellCount(); index++ {
		cell, cellOK := input.StorageCellAt(index)
		term, termOK := storage.At(index)
		ordinal, ordinalOK := table.index.Lookup(radix.Index(mount), uint32(term))
		if !cellOK || !termOK {
			return errors.New("link/boundary: malformed semantic Cell receipt")
		}
		if !ordinalOK || uint64(ordinal) >= uint64(len(table.rows)) {
			return errors.New("link/boundary: semantic Cell has no Boundary ordinal")
		}
		row := table.rows[ordinal]
		if row.shard != mount || row.term != term {
			return errors.New("link/boundary: semantic Cell Boundary ordinal is not exact")
		}
		if err := add(cell.ContextID(), ordinal, true); err != nil {
			return err
		}
	}
	for index := 0; index < input.CallCount(); index++ {
		call, callOK := input.CallAt(index)
		if !callOK {
			continue
		}
		span, spanOK := call.Span()
		callee, calleeOK := call.Callee()
		calleeSpan, calleeSpanOK := callee.Span()
		actuals, actualsOK := call.Actuals()
		actualsSpan, actualsSpanOK := actuals.Span()
		values, valuesOK := call.Values()
		valuesSpan, valuesSpanOK := values.Span()
		if !input.OwnsCallOccurrence(call) || !spanOK || !calleeOK || !calleeSpanOK || !actualsOK || !actualsSpanOK || !valuesOK || !valuesSpanOK {
			return errors.New("link/boundary: malformed semantic Call receipt")
		}
		for _, row := range []struct {
			id   keyspace.ContentID
			span program.Span
		}{{call.ContextID(), span}, {callee.ContextID(), calleeSpan}, {actuals.ContextID(), actualsSpan}, {values.ContextID(), valuesSpan}} {
			if err := addSpan("Call operand", row.id, row.span, true); err != nil {
				return err
			}
		}
		if receiver, receiverOK := call.Receiver(); receiverOK {
			receiverSpan, receiverSpanOK := receiver.Span()
			if !receiverSpanOK {
				return errors.New("link/boundary: malformed semantic Call receiver")
			}
			if err := addSpan("Call receiver", receiver.ContextID(), receiverSpan, true); err != nil {
				return err
			}
		}
		for argumentIndex := 0; argumentIndex < values.Count(); argumentIndex++ {
			argument, argumentOK := values.At(argumentIndex)
			argumentSpan, argumentSpanOK := argument.Span()
			if !argumentOK || !argumentSpanOK {
				return errors.New("link/boundary: malformed semantic Call argument")
			}
			if err := addSpan("Call argument", argument.ContextID(), argumentSpan, true); err != nil {
				return err
			}
		}
		if tail, tailOK := values.Tail(); tailOK {
			tailSpan, tailSpanOK := tail.Span()
			if !tailSpanOK {
				return errors.New("link/boundary: malformed semantic Call tail")
			}
			if err := addSpan("Call tail", tail.ContextID(), tailSpan, true); err != nil {
				return err
			}
		}
	}
	// Computation and return rows are artifact-issued from the exact authored
	// Span identities.  Publish the same semantic inverse here so mounted
	// consumers join by ModuleKey+occurrence ID rather than reopening Terms.
	for index := 0; index < input.UnaryOccurrenceCount(); index++ {
		row, ok := input.UnaryOccurrenceAt(index)
		span, spanOK := row.Span()
		operandSpan, operandSpanOK := row.OperandSpan()
		if !ok || !spanOK || !operandSpanOK || addSpan("Unary", row.ContextID(), span, true) != nil || addSpan("Unary operand", row.OperandID(), operandSpan, true) != nil {
			return errors.New("link/boundary: malformed semantic Unary receipt")
		}
	}
	for index := 0; index < input.SelectOccurrenceCount(); index++ {
		row, ok := input.SelectOccurrenceAt(index)
		span, spanOK := row.Span()
		leftSpan, leftSpanOK := row.LeftSpan()
		rightSpan, rightSpanOK := row.RightSpan()
		if !ok || !spanOK || !leftSpanOK || !rightSpanOK || addSpan("Select", row.ContextID(), span, true) != nil || addSpan("Select left", row.LeftID(), leftSpan, true) != nil || addSpan("Select right", row.RightID(), rightSpan, true) != nil {
			return errors.New("link/boundary: malformed semantic Select receipt")
		}
	}
	for index := 0; index < input.ClaimOccurrenceCount(); index++ {
		row, ok := input.ClaimOccurrenceAt(index)
		span, spanOK := row.Span()
		operandSpan, operandSpanOK := row.OperandSpan()
		if !ok || !spanOK || !operandSpanOK || addSpan("Claim", row.ContextID(), span, true) != nil || addSpan("Claim operand", row.OperandID(), operandSpan, true) != nil {
			return errors.New("link/boundary: malformed semantic Claim receipt")
		}
	}
	for index := 0; index < input.ReturnOccurrenceCount(); index++ {
		row, ok := input.ReturnOccurrenceAt(index)
		_, spanOK := row.Span()
		valuesSpan, valuesSpanOK := row.ValuesSpan()
		if !ok || !spanOK || !valuesSpanOK || !row.ContextID().Available() || !row.ValuesID().Available() {
			return errors.New("link/boundary: unavailable semantic Return receipt")
		}
		// A Return is an Outcome/control occurrence, rather than a member of
		// Boundary's value universe. Its ID is consumed by the artifact and
		// Residence boundary rows, which preserve that structural identity
		// directly. Only the returned Values occurrence needs a Boundary Value
		// ordinal for Value's return transfer rule. Attempting to map the
		// Return span itself through valuePairs is a category error: the value
		// universe intentionally contains the derived Outcome terminal, not the
		// authored Return control term.
		if err := addSpan("Return values", row.ValuesID(), valuesSpan, true); err != nil {
			return err
		}
	}
	type sourceFamily struct {
		count int
		at    func(int) (program.ValueSourceOccurrence, bool)
	}
	for _, family := range []sourceFamily{
		{input.NilSourceCount(), input.NilSourceAt}, {input.BoolSourceCount(), input.BoolSourceAt},
		{input.IntegerSourceCount(), input.IntegerSourceAt}, {input.FloatSourceCount(), input.FloatSourceAt},
		{input.StringSourceCount(), input.StringSourceAt}, {input.TypeValueSourceCount(), input.TypeValueSourceAt},
	} {
		for index := 0; index < family.count; index++ {
			source, ok := family.at(index)
			if !ok {
				continue
			}
			span, spanOK := source.Span()
			if !input.OwnsValueSourceOccurrence(source) || !spanOK || addSpan("ValueSource", source.ContextID(), span, true) != nil {
				return errors.New("link/boundary: malformed semantic ValueSource receipt")
			}
		}
	}
	for index := 0; index < input.StorageReadCount(); index++ {
		read, ok := input.StorageReadAt(index)
		if !ok {
			continue
		}
		span, spanOK := read.Span()
		if !input.OwnsStorageReadOccurrence(read) || !spanOK || addSpan("StorageRead", read.ContextID(), span, true) != nil {
			return errors.New("link/boundary: malformed semantic StorageRead receipt")
		}
	}
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

// ForProgramSpan resolves one exact opaque Program occurrence proof through
// Boundary's sole sealed span-to-Value inverse. The authored term remains in
// Program/Boundary construction; consumers cannot reconstruct or supply it.
func (v Values) ForProgramSpan(shard linkproject.Shard, span program.Span) (Value, bool) {
	if !v.live() || !span.Available() {
		return Value{}, false
	}
	mounts := v.component.authority.project.Mounts()
	mountIndex, mountOK := mounts.Index(shard)
	p, programOK := mounts.Program(shard)
	context := span.ContextID()
	if !mountOK || mountIndex < 0 || !programOK || p == nil || !p.TransformerInput().OwnsSpan(span) || !context.Available() {
		return Value{}, false
	}
	ordinal, ok := v.component.authority.valueTable.spans[valueSpanKey{mount: uint32(mountIndex + 1), context: context}]
	if !ok || uint64(ordinal) >= uint64(len(v.component.authority.valueTable.rows)) {
		return Value{}, false
	}
	row := v.component.authority.valueTable.rows[ordinal]
	value := Value{component: v.component, ordinal: ordinal}
	return value, row.shard == uint32(mountIndex+1) && v.valid(value)
}

// ForMountedSpan rebinds one reusable Program span identity through an exact
// Link mount. It is the artifact substitution lane: neither a Program handle
// nor an authored Term is accepted or reconstructed.
func (v Values) ForMountedSpan(moduleKey, spanContext keyspace.ContentID) (Value, bool) {
	if !v.live() || !moduleKey.Available() || !spanContext.Available() {
		return Value{}, false
	}
	mount := v.component.authority.valueTable.mounts[moduleKey]
	if mount == 0 || uint64(mount) > uint64(len(v.component.authority.valueTable.relations)) {
		return Value{}, false
	}
	ordinal, ok := v.component.authority.valueTable.spans[valueSpanKey{mount: mount, context: spanContext}]
	if !ok || uint64(ordinal) >= uint64(len(v.component.authority.valueTable.rows)) {
		return Value{}, false
	}
	row := v.component.authority.valueTable.rows[ordinal]
	value := Value{component: v.component, ordinal: ordinal}
	return value, row.shard == mount && v.valid(value)
}

// ForMountedSemantic rebinds one exact reusable Program semantic occurrence
// through a concrete ModuleKey. The inverse was sealed while Link owned the
// live Program proof; callers cannot provide a raw Term or fabricate a
// Value-to-occurrence join.
func (v Values) ForMountedSemantic(moduleKey, occurrenceID keyspace.ContentID) (Value, bool) {
	if !v.live() || !moduleKey.Available() || !occurrenceID.Available() {
		return Value{}, false
	}
	mount := v.component.authority.valueTable.mounts[moduleKey]
	if mount == 0 || uint64(mount) > uint64(len(v.component.authority.valueTable.relations)) {
		return Value{}, false
	}
	ordinal, ok := v.component.authority.valueTable.semantic[valueSemanticKey{mount: mount, id: occurrenceID}]
	if !ok || uint64(ordinal) >= uint64(len(v.component.authority.valueTable.rows)) {
		return Value{}, false
	}
	row := v.component.authority.valueTable.rows[ordinal]
	value := Value{component: v.component, ordinal: ordinal}
	return value, row.shard == mount && v.valid(value)
}

// VisitMountedSemantics visits the complete mounted semantic Value directory
// exactly once. Order is deliberately unspecified: semantic IDs are opaque
// lookup keys, not an authored sequence. The callback receives only the
// existing ModuleKey and Boundary Value association sealed by this owner.
func (v Values) VisitMountedSemantics(visit func(moduleKey, occurrenceID keyspace.ContentID, value Value) bool) bool {
	if !v.live() || visit == nil {
		return false
	}
	table := v.component.authority.valueTable
	mounts := v.component.authority.project.Mounts()
	if mounts.Count() != len(table.relations) {
		return false
	}
	for key, ordinal := range table.semantic {
		if key.mount == 0 || uint64(key.mount) > uint64(mounts.Count()) || !key.id.Available() || uint64(ordinal) >= uint64(len(table.rows)) {
			return false
		}
		shard, shardOK := mounts.At(int(key.mount - 1))
		module, moduleOK := v.component.authority.project.ModuleKey(shard)
		row := table.rows[ordinal]
		value := Value{component: v.component, ordinal: ordinal}
		if !shardOK || !moduleOK || !module.Available() || table.mounts[module] != key.mount || row.shard != key.mount || !v.valid(value) || !visit(module, key.id, value) {
			return false
		}
	}
	return true
}
