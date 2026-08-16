package program

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programstatic "github.com/wippyai/go-lua/analysis/program/static"
)

// valueSourceOccurrenceKind is private because the Program proof establishes
// only the local occurrence class. In particular, a TypeValue candidate does
// not claim that a Link resolver admits its static reference.
type valueSourceOccurrenceKind uint8

const (
	valueSourceOccurrenceInvalid valueSourceOccurrenceKind = iota
	valueSourceOccurrenceNil
	valueSourceOccurrenceBool
	valueSourceOccurrenceInteger
	valueSourceOccurrenceFloat
	valueSourceOccurrenceString
	valueSourceOccurrenceTypeValue
)

// ValueSourceAnchor is Source/Flow's opaque proof that one value-source term
// evaluates at an existing Program Finish site. The retained Span is either
// the term's direct span or the exact Source-root span selected at issuance;
// consumers do not repeat that choice.
type ValueSourceAnchor struct {
	input  TransformerInput
	source keyspace.Term
	root   keyspace.Term
	path   identity.ContentID
	span   Span
	direct bool
}

// ValueSourceOccurrence is a transient proof of one Program-local literal or
// executable TypeValue source candidate. Concrete Link Values, resolver
// admission, and analysis coordinates remain outside Program.
type ValueSourceOccurrence struct {
	input  TransformerInput
	kind   valueSourceOccurrenceKind
	cursor int
	term   keyspace.Term
	target keyspace.Term
	body   Body
	anchor ValueSourceAnchor
}

// StaticTypeReferenceID returns the detached identity of an executable
// TypeValue source's target static type. Literal source occurrences have no
// static target and return false. The target Term remains private to the
// Program proof and never crosses into an artifact row.
func (source ValueSourceOccurrence) StaticTypeReferenceID() (identity.ContentID, bool) {
	if !source.Available() || source.kind != valueSourceOccurrenceTypeValue || source.target == 0 {
		return identity.ContentID{}, false
	}
	ref, ok := source.input.owner.Static().StaticTypes().Ref(source.target)
	if !ok {
		return identity.ContentID{}, false
	}
	return StaticTypeReferenceID(source.input.programID, ref)
}

// StaticTypeValueName is the closed lexical name used by Static's TypeValue
// root quotient. It is issued while the exact Program static declaration
// owner is live and returns no source coordinate.
func (source ValueSourceOccurrence) StaticTypeValueName() (string, bool) {
	if !source.Available() || source.kind != valueSourceOccurrenceTypeValue || source.target == 0 {
		return "", false
	}
	view := source.input.owner.Static()
	if primitive, ok := view.Types().Primitives().Get(source.target); ok {
		name := map[programstatic.PrimitiveKind]string{
			programstatic.PrimitiveNil: "nil", programstatic.PrimitiveBoolean: "boolean", programstatic.PrimitiveNumber: "number",
			programstatic.PrimitiveInteger: "integer", programstatic.PrimitiveString: "string", programstatic.PrimitiveAny: "any",
			programstatic.PrimitiveUnknown: "unknown", programstatic.PrimitiveNever: "never",
		}[primitive]
		return name, name != ""
	}
	_, declaration, _, ok := view.References().Get(source.target)
	if !ok || declaration == 0 {
		return "", false
	}
	if _, _, key, _, alias := view.Declarations().Aliases().Get(declaration); alias {
		value, valueOK := source.input.owner.Source().Keys().Exact(key)
		return value.String, valueOK && value.Kind == keyspace.LiteralString && value.String != ""
	}
	if _, key, _, iface := view.Declarations().Interfaces().Get(declaration); iface {
		value, valueOK := source.input.owner.Source().Keys().Exact(key)
		return value.String, valueOK && value.Kind == keyspace.LiteralString && value.String != ""
	}
	return "", false
}

// StaticTypeValueRootID is the Program-local root quotient input. Static
// adds the Link mount identity when issuing the final mounted root, preserving
// exact same-name separation across lexical activations and mounts.
func (source ValueSourceOccurrence) StaticTypeValueRootID() (identity.ContentID, bool) {
	name, nameOK := source.StaticTypeValueName()
	body, bodyOK := source.Body()
	path := body.PathID()
	if !nameOK || !bodyOK || !path.Available() {
		return identity.ContentID{}, false
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("program/static-typevalue-root/v1"))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(source.input.programID[:])
	_, _ = hash.Write(path[:])
	var word [8]byte
	binary.BigEndian.PutUint64(word[:], uint64(len(name)))
	_, _ = hash.Write(word[:])
	_, _ = hash.Write([]byte(name))
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

// LiteralPayload returns the exact parent-issued literal payload for this
// source occurrence.  The payload is copied into ProgramArtifact at compile
// time; callers never receive the authored Term or Source catalog.
func (source ValueSourceOccurrence) LiteralPayload() (keyspace.Family, keyspace.LiteralValue, bool) {
	if !source.Available() {
		return keyspace.FamilyInvalid, keyspace.LiteralValue{}, false
	}
	ordinal := keyspace.TermOrdinal(source.term)
	if ordinal == 0 {
		return keyspace.FamilyInvalid, keyspace.LiteralValue{}, false
	}
	literals := source.input.owner.Source().Literals()
	switch keyspace.TermFamily(source.term) {
	case keyspace.FamilyNil:
		term, _, ok := literals.Nils().At(int(ordinal - 1))
		return keyspace.FamilyNil, keyspace.LiteralValue{}, ok && term == source.term
	case keyspace.FamilyBool:
		term, _, value, ok := literals.Bools().At(int(ordinal - 1))
		return keyspace.FamilyBool, keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: value}, ok && term == source.term
	case keyspace.FamilyInteger:
		term, _, value, ok := literals.Integers().At(int(ordinal - 1))
		return keyspace.FamilyInteger, keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}, ok && term == source.term
	case keyspace.FamilyFloat:
		term, _, value, ok := literals.Floats().At(int(ordinal - 1))
		return keyspace.FamilyFloat, keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: value}, ok && term == source.term
	case keyspace.FamilyString:
		term, _, value, ok := literals.Strings().At(int(ordinal - 1))
		return keyspace.FamilyString, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}, ok && term == source.term
	default:
		return keyspace.FamilyInvalid, keyspace.LiteralValue{}, false
	}
}

// NilSourceCount is Source's exact Nil literal denominator.
func (input TransformerInput) NilSourceCount() int {
	if !input.Available() {
		return 0
	}
	return input.owner.Source().Literals().Nils().Count()
}

// NilSourceAt issues one occurrence in Source's Nil order.
func (input TransformerInput) NilSourceAt(cursor int) (ValueSourceOccurrence, bool) {
	result, ok := input.valueSourceOccurrence(valueSourceOccurrenceNil, cursor)
	return result, ok && result.Available()
}

// BoolSourceCount is Source's exact Bool literal denominator.
func (input TransformerInput) BoolSourceCount() int {
	if !input.Available() {
		return 0
	}
	return input.owner.Source().Literals().Bools().Count()
}

// BoolSourceAt issues one occurrence in Source's Bool order.
func (input TransformerInput) BoolSourceAt(cursor int) (ValueSourceOccurrence, bool) {
	result, ok := input.valueSourceOccurrence(valueSourceOccurrenceBool, cursor)
	return result, ok && result.Available()
}

// IntegerSourceCount is Source's exact Integer literal denominator.
func (input TransformerInput) IntegerSourceCount() int {
	if !input.Available() {
		return 0
	}
	return input.owner.Source().Literals().Integers().Count()
}

// IntegerSourceAt issues one occurrence in Source's Integer order.
func (input TransformerInput) IntegerSourceAt(cursor int) (ValueSourceOccurrence, bool) {
	result, ok := input.valueSourceOccurrence(valueSourceOccurrenceInteger, cursor)
	return result, ok && result.Available()
}

// FloatSourceCount is Source's exact Float literal denominator.
func (input TransformerInput) FloatSourceCount() int {
	if !input.Available() {
		return 0
	}
	return input.owner.Source().Literals().Floats().Count()
}

// FloatSourceAt issues one occurrence in Source's Float order.
func (input TransformerInput) FloatSourceAt(cursor int) (ValueSourceOccurrence, bool) {
	result, ok := input.valueSourceOccurrence(valueSourceOccurrenceFloat, cursor)
	return result, ok && result.Available()
}

// StringSourceCount is Source's exact String literal denominator.
func (input TransformerInput) StringSourceCount() int {
	if !input.Available() {
		return 0
	}
	return input.owner.Source().Literals().Strings().Count()
}

// StringSourceAt issues one occurrence in Source's String order.
func (input TransformerInput) StringSourceAt(cursor int) (ValueSourceOccurrence, bool) {
	result, ok := input.valueSourceOccurrence(valueSourceOccurrenceString, cursor)
	return result, ok && result.Available()
}

// TypeValueSourceCount is Flow's authored TypeValue candidate denominator.
// Dead candidates remain in this denominator and fail TypeValueSourceAt.
func (input TransformerInput) TypeValueSourceCount() int {
	if !input.Available() {
		return 0
	}
	return input.owner.Flow().Authored().TypeValues().Count()
}

// TypeValueSourceAt issues only an executable Program-local TypeValue whose
// Flow row and Static target agree. Link resolver/expression admission is a
// later, separate proof.
func (input TransformerInput) TypeValueSourceAt(cursor int) (ValueSourceOccurrence, bool) {
	result, ok := input.valueSourceOccurrence(valueSourceOccurrenceTypeValue, cursor)
	return result, ok && result.Available()
}

func (input TransformerInput) valueSourceOccurrence(kind valueSourceOccurrenceKind, cursor int) (ValueSourceOccurrence, bool) {
	if !input.Available() || cursor < 0 {
		return ValueSourceOccurrence{}, false
	}
	var term, owner, target keyspace.Term
	var ok bool
	switch kind {
	case valueSourceOccurrenceNil:
		term, owner, ok = input.owner.Source().Literals().Nils().At(cursor)
	case valueSourceOccurrenceBool:
		term, owner, _, ok = input.owner.Source().Literals().Bools().At(cursor)
	case valueSourceOccurrenceInteger:
		term, owner, _, ok = input.owner.Source().Literals().Integers().At(cursor)
	case valueSourceOccurrenceFloat:
		term, owner, _, ok = input.owner.Source().Literals().Floats().At(cursor)
	case valueSourceOccurrenceString:
		term, owner, _, ok = input.owner.Source().Literals().Strings().At(cursor)
	case valueSourceOccurrenceTypeValue:
		typeValues := input.owner.Flow().Authored().TypeValues()
		term, ok = typeValues.At(cursor)
		if !ok {
			return ValueSourceOccurrence{}, false
		}
		owner, ok = typeValues.Get(term)
		if !ok || !input.owner.Flow().Executable().Contains(term) {
			return ValueSourceOccurrence{}, false
		}
		target, ok = input.owner.Static().Operands().TypeValues().Target(term)
		if !ok {
			return ValueSourceOccurrence{}, false
		}
		ref, refOK := input.owner.Static().StaticTypes().Ref(target)
		if !refOK || ref.Term() != target {
			return ValueSourceOccurrence{}, false
		}
	default:
		return ValueSourceOccurrence{}, false
	}
	if !ok || term == 0 || owner == 0 {
		return ValueSourceOccurrence{}, false
	}
	// Literal/TypeValue rows already carry Source's exact execution owner.
	// That owner deliberately differs from syntactic containment for a Repeat
	// condition: the condition is authored under the Repeat statement but
	// evaluates in the loop Body so its body-local Cells remain visible. Do not
	// reopen Position as a second ownership authority here.
	body, bodyOK := input.Body(owner)
	anchor, anchorOK := input.ValueSourceAnchor(term)
	if !bodyOK || !input.OwnsBody(body) || !anchorOK {
		return ValueSourceOccurrence{}, false
	}
	return ValueSourceOccurrence{input: input, kind: kind, cursor: cursor, term: term, target: target, body: body, anchor: anchor}, true
}

// ValueSourceAnchor issues Source/Flow's sole direct-or-root evaluation
// anchor. The direct/root choice is made here, once, and thereafter travels as
// an exact-owner proof instead of being reconstructed by a child occurrence.
func (input TransformerInput) ValueSourceAnchor(term keyspace.Term) (ValueSourceAnchor, bool) {
	if !input.Available() || !input.ownsValueSourceTerm(term) {
		return ValueSourceAnchor{}, false
	}
	root := term
	span, direct := input.Span(root)
	ok := direct
	if !direct {
		root, ok = input.owner.Source().Index().Root(term)
		if !ok || root == 0 {
			return ValueSourceAnchor{}, false
		}
		span, ok = input.Span(root)
	}
	path, pathOK := input.owner.Flow().ValueSourcePath(term)
	anchor := ValueSourceAnchor{input: input, source: term, root: root, path: path, span: span, direct: direct}
	return anchor, ok && pathOK && anchor.Available()
}

func (anchor ValueSourceAnchor) Available() bool {
	if !anchor.input.Available() || !anchor.input.ownsValueSourceTerm(anchor.source) || anchor.root == 0 || !anchor.path.Available() ||
		anchor.span.input != anchor.input || !anchor.span.Available() {
		return false
	}
	authored, authoredOK := anchor.span.Authored()
	entry, entryOK := anchor.span.Entry()
	finish, finishOK := anchor.span.Finish()
	if !authoredOK || authored != anchor.root || !entryOK || !finishOK ||
		!anchor.input.OwnsSite(entry) || !anchor.input.OwnsSite(finish) {
		return false
	}
	if anchor.direct {
		if anchor.root != anchor.source {
			return false
		}
		path, pathOK := anchor.input.owner.Flow().ValueSourcePath(anchor.source)
		if !pathOK || path != anchor.path {
			return false
		}
		direct, ok := anchor.input.Span(anchor.source)
		return ok && direct.input == anchor.input && direct.Equal(anchor.span)
	}
	root, ok := anchor.input.owner.Source().Index().Root(anchor.source)
	path, pathOK := anchor.input.owner.Flow().ValueSourcePath(anchor.source)
	if !ok || root != anchor.root || !pathOK || path != anchor.path {
		return false
	}
	rootSpan, ok := anchor.input.Span(root)
	return ok && rootSpan.input == anchor.input && rootSpan.Equal(anchor.span)
}

func (anchor ValueSourceAnchor) ContextID() identity.ContentID {
	if !anchor.Available() {
		return identity.ContentID{}
	}
	return transformerSemanticID("program/transformer/value-source-anchor", func(writer *framing.Writer) bool {
		pathID := anchor.path
		return writer.Bool(anchor.direct) == nil && writer.Bytes(pathID[:]) == nil
	})
}

func (anchor ValueSourceAnchor) Finish() (flow.Site, bool) {
	if !anchor.Available() {
		return flow.Site{}, false
	}
	return anchor.span.Finish()
}

func (input TransformerInput) OwnsValueSourceAnchor(anchor ValueSourceAnchor) bool {
	return input.Available() && anchor.input == input && anchor.span.input == input && anchor.Available()
}

func (input TransformerInput) ownsValueSourceTerm(term keyspace.Term) bool {
	if !input.Available() {
		return false
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 || uint64(ordinal-1) > uint64(^uint(0)>>1) {
		return false
	}
	index := int(ordinal - 1)
	literals := input.owner.Source().Literals()
	var issued keyspace.Term
	var ok bool
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyNil:
		issued, _, ok = literals.Nils().At(index)
	case keyspace.FamilyBool:
		issued, _, _, ok = literals.Bools().At(index)
	case keyspace.FamilyInteger:
		issued, _, _, ok = literals.Integers().At(index)
	case keyspace.FamilyFloat:
		issued, _, _, ok = literals.Floats().At(index)
	case keyspace.FamilyString:
		issued, _, _, ok = literals.Strings().At(index)
	case keyspace.FamilyTypeValue:
		issued, ok = input.owner.Flow().Authored().TypeValues().At(index)
	default:
		return false
	}
	return ok && issued == term
}

func (source ValueSourceOccurrence) Available() bool {
	if !source.input.Available() || source.cursor < 0 || source.term == 0 || !source.input.OwnsBody(source.body) ||
		!source.input.OwnsValueSourceAnchor(source.anchor) {
		return false
	}
	expected, ok := source.input.valueSourceOccurrence(source.kind, source.cursor)
	return ok && expected.term == source.term && expected.target == source.target &&
		expected.body.Equal(source.body) && expected.anchor.input == source.anchor.input &&
		expected.anchor.source == source.anchor.source && expected.anchor.root == source.anchor.root &&
		expected.anchor.direct == source.anchor.direct && expected.anchor.span.Equal(source.anchor.span)
}

func (source ValueSourceOccurrence) ContextID() identity.ContentID {
	if !source.Available() {
		return identity.ContentID{}
	}
	bodyID, anchorID, bodyPath := source.body.ContextID(), source.anchor.ContextID(), source.body.PathID()
	return transformerSemanticID("program/transformer/value-source-occurrence", func(writer *framing.Writer) bool {
		return writer.Uint(uint64(source.kind)) == nil && writer.Bytes(bodyPath[:]) == nil &&
			writer.Bytes(bodyID[:]) == nil && writer.Bytes(anchorID[:]) == nil
	})
}

func (source ValueSourceOccurrence) Body() (Body, bool) {
	return source.body, source.Available()
}

func (source ValueSourceOccurrence) Finish() (flow.Site, bool) {
	if !source.Available() {
		return flow.Site{}, false
	}
	return source.anchor.Finish()
}

// Span returns the already-issued source evaluation span used by the
// occurrence. It is consumed during Link semantic inverse construction only.
func (source ValueSourceOccurrence) Span() (Span, bool) {
	if !source.Available() || !source.anchor.span.Available() {
		return Span{}, false
	}
	return source.anchor.span, true
}

func (input TransformerInput) OwnsValueSourceOccurrence(source ValueSourceOccurrence) bool {
	return input.Available() && source.input == input && source.Available()
}

// StorageReadOccurrence is one executable fixed-cell Read and its exact
// Entry-to-Finish evaluation geometry.
type StorageReadOccurrence struct {
	input         TransformerInput
	cursor        int
	term, source  keyspace.Term
	body          Body
	entry, finish flow.Site
}

func (input TransformerInput) StorageReadCount() int {
	if !input.Available() {
		return 0
	}
	return input.owner.Flow().Authored().Storage().Reads().Count()
}

// StorageReadAt preserves Flow's authored Read order. Dead or malformed
// candidates fail instead of being compacted into a second denominator.
func (input TransformerInput) StorageReadAt(cursor int) (StorageReadOccurrence, bool) {
	result, ok := input.storageReadOccurrence(cursor)
	return result, ok && result.Available()
}

func (input TransformerInput) storageReadOccurrence(cursor int) (StorageReadOccurrence, bool) {
	if !input.Available() || cursor < 0 {
		return StorageReadOccurrence{}, false
	}
	storage := input.owner.Flow().Authored().Storage()
	term, present := storage.Reads().At(cursor)
	owner, source, _, related := storage.Reads().Get(term)
	if !present || !related || !input.owner.Flow().Executable().Contains(term) {
		return StorageReadOccurrence{}, false
	}
	if _, _, _, ok := storage.Cells().Get(source); !ok {
		return StorageReadOccurrence{}, false
	}
	body, bodyOK := input.occurrenceBody(owner, term)
	entry, finish, sitesOK := input.occurrenceSites(term)
	if !bodyOK || !sitesOK {
		return StorageReadOccurrence{}, false
	}
	return StorageReadOccurrence{input: input, cursor: cursor, term: term, source: source, body: body, entry: entry, finish: finish}, true
}

func (read StorageReadOccurrence) Available() bool {
	if !read.input.Available() || read.cursor < 0 || read.term == 0 || read.source == 0 ||
		!read.input.OwnsBody(read.body) || !read.input.OwnsSite(read.entry) || !read.input.OwnsSite(read.finish) {
		return false
	}
	expected, ok := read.input.storageReadOccurrence(read.cursor)
	return ok && expected.term == read.term && expected.source == read.source && expected.body.Equal(read.body) &&
		expected.entry.Equal(read.entry) && expected.finish.Equal(read.finish)
}

func (read StorageReadOccurrence) ContextID() identity.ContentID {
	if !read.Available() {
		return identity.ContentID{}
	}
	bodyID, entryID, finishID := read.body.ContextID(), read.entry.ContextID(), read.finish.ContextID()
	return transformerRoleID("program/transformer/storage-read", read.input.programID, func(writer *framing.Writer) bool {
		pathID := read.body.PathID()
		return writer.Bytes(pathID[:]) == nil && writer.Bytes(bodyID[:]) == nil && writer.Bytes(entryID[:]) == nil && writer.Bytes(finishID[:]) == nil
	})
}

func (read StorageReadOccurrence) Body() (Body, bool) { return read.body, read.Available() }
func (read StorageReadOccurrence) Entry() (flow.Site, bool) {
	return read.entry, read.Available()
}
func (read StorageReadOccurrence) Finish() (flow.Site, bool) {
	return read.finish, read.Available()
}

func (read StorageReadOccurrence) Span() (Span, bool) {
	if !read.Available() {
		return Span{}, false
	}
	return read.input.Span(read.term)
}

// Cell returns the exact existing storage Cell read by this occurrence.
func (read StorageReadOccurrence) Cell() (Cell, bool) {
	if !read.Available() {
		return Cell{}, false
	}
	return read.input.storageCell(read.source)
}
func (input TransformerInput) OwnsStorageReadOccurrence(read StorageReadOccurrence) bool {
	return input.Available() && read.input == input && read.Available()
}

// StorageBind is one executable Bind and the existing ordered Source bind
// range from which fixed storage transfers may be issued.
type StorageBind struct {
	input         TransformerInput
	cursor        int
	term, values  keyspace.Term
	width         int
	body          Body
	entry, finish flow.Site
}

// StorageBindOccurrence is one fixed position in a StorageBind. Its source
// Value and destination Cell coordinates remain private Program scalars.
type StorageBindOccurrence struct {
	bind          StorageBind
	position      int
	value, target keyspace.Term
}

func (input TransformerInput) StorageBindCount() int {
	if !input.Available() {
		return 0
	}
	return input.owner.Flow().Authored().Storage().Binds().Count()
}

func (input TransformerInput) StorageBindAt(cursor int) (StorageBind, bool) {
	result, ok := input.storageBind(cursor)
	return result, ok && result.Available()
}

func (input TransformerInput) storageBind(cursor int) (StorageBind, bool) {
	if !input.Available() || cursor < 0 {
		return StorageBind{}, false
	}
	flowView := input.owner.Flow()
	storage := flowView.Authored().Storage()
	term, present := storage.Binds().At(cursor)
	owner, values, related := storage.Binds().Get(term)
	width, sized := input.owner.Source().Binds().Len(term)
	if !present || !related || !sized || width < 0 || !flowView.Executable().Contains(term) {
		return StorageBind{}, false
	}
	if _, _, valuesOK := flowView.Authored().Values().Get(values); !valuesOK {
		return StorageBind{}, false
	}
	body, bodyOK := input.occurrenceBody(owner, term)
	entry, finish, sitesOK := input.occurrenceSites(term)
	if !bodyOK || !sitesOK {
		return StorageBind{}, false
	}
	return StorageBind{input: input, cursor: cursor, term: term, values: values, width: width, body: body, entry: entry, finish: finish}, true
}

func (bind StorageBind) Available() bool {
	if !bind.input.Available() || bind.cursor < 0 || bind.term == 0 || bind.values == 0 || bind.width < 0 ||
		!bind.input.OwnsBody(bind.body) || !bind.input.OwnsSite(bind.entry) || !bind.input.OwnsSite(bind.finish) {
		return false
	}
	expected, ok := bind.input.storageBind(bind.cursor)
	return ok && expected.term == bind.term && expected.values == bind.values && expected.width == bind.width &&
		expected.body.Equal(bind.body) && expected.entry.Equal(bind.entry) && expected.finish.Equal(bind.finish)
}

func (bind StorageBind) ContextID() identity.ContentID {
	if !bind.Available() {
		return identity.ContentID{}
	}
	bodyID, entryID, finishID := bind.body.ContextID(), bind.entry.ContextID(), bind.finish.ContextID()
	return transformerRoleID("program/transformer/storage-bind", bind.input.programID, func(writer *framing.Writer) bool {
		pathID := bind.body.PathID()
		return writer.Bytes(pathID[:]) == nil && writer.Count(uint64(bind.width)) == nil && writer.Bytes(bodyID[:]) == nil &&
			writer.Bytes(entryID[:]) == nil && writer.Bytes(finishID[:]) == nil
	})
}

func (bind StorageBind) Body() (Body, bool) { return bind.body, bind.Available() }
func (bind StorageBind) Entry() (flow.Site, bool) {
	return bind.entry, bind.Available()
}
func (bind StorageBind) Finish() (flow.Site, bool) {
	return bind.finish, bind.Available()
}

// Values is the exact owner-issued source Values occurrence for this bind.
func (bind StorageBind) Values() (ValuesOccurrence, bool) {
	if !bind.Available() {
		return ValuesOccurrence{}, false
	}
	values, ok := bind.input.valuesForTerm(bind.values)
	return values, ok && bind.input.OwnsValuesOccurrence(values)
}
func (bind StorageBind) TransferCount() int {
	if !bind.Available() {
		return 0
	}
	return bind.width
}

// CellAt returns the exact ordered destination Cell for this bind.  Unlike
// TransferAt it remains available when the source Values row has no fixed
// member at that position: the destination denominator belongs to the
// authored bind, not to the currently fixed source prefix.  Artifact
// compilers use this to retain the complete boundary shape without reopening
// Source after Program compilation.
func (bind StorageBind) CellAt(position int) (Cell, bool) {
	if !bind.Available() || position < 0 || position >= bind.width {
		return Cell{}, false
	}
	term, ok := bind.input.owner.Source().Binds().At(bind.term, position)
	if !ok {
		return Cell{}, false
	}
	cell, cellOK := bind.input.storageCell(term)
	return cell, cellOK && bind.input.OwnsCell(cell)
}

func (bind StorageBind) TransferAt(position int) (StorageBindOccurrence, bool) {
	result, ok := bind.storageBindOccurrence(position)
	return result, ok && result.Available()
}

func (bind StorageBind) storageBindOccurrence(position int) (StorageBindOccurrence, bool) {
	if !bind.Available() || position < 0 || position >= bind.width {
		return StorageBindOccurrence{}, false
	}
	cell, bound := bind.input.owner.Source().Binds().At(bind.term, position)
	value, fixed := bind.input.owner.Flow().Authored().Values().Member(bind.values, position)
	if !bound || !fixed {
		return StorageBindOccurrence{}, false
	}
	if _, _, _, ok := bind.input.owner.Flow().Authored().Storage().Cells().Get(cell); !ok {
		return StorageBindOccurrence{}, false
	}
	return StorageBindOccurrence{bind: bind, position: position, value: value, target: cell}, true
}

func (occurrence StorageBindOccurrence) Available() bool {
	if !occurrence.bind.Available() || occurrence.position < 0 || occurrence.value == 0 || occurrence.target == 0 {
		return false
	}
	expected, ok := occurrence.bind.storageBindOccurrence(occurrence.position)
	return ok && expected.value == occurrence.value && expected.target == occurrence.target
}

func (occurrence StorageBindOccurrence) ContextID() identity.ContentID {
	if !occurrence.Available() {
		return identity.ContentID{}
	}
	bindID := occurrence.bind.ContextID()
	return transformerRoleID("program/transformer/storage-bind-transfer", occurrence.bind.input.programID, func(writer *framing.Writer) bool {
		return writer.Bytes(bindID[:]) == nil && writer.Uint(uint64(occurrence.position)) == nil
	})
}

func (occurrence StorageBindOccurrence) Body() (Body, bool) {
	return occurrence.bind.body, occurrence.Available()
}
func (occurrence StorageBindOccurrence) Entry() (flow.Site, bool) {
	return occurrence.bind.entry, occurrence.Available()
}
func (occurrence StorageBindOccurrence) Finish() (flow.Site, bool) {
	return occurrence.bind.finish, occurrence.Available()
}

// Value returns this transfer's exact ordered Values member.
func (occurrence StorageBindOccurrence) Value() (ValuesMember, bool) {
	if !occurrence.Available() {
		return ValuesMember{}, false
	}
	values, valuesOK := occurrence.bind.Values()
	member, memberOK := values.At(occurrence.position)
	return member, valuesOK && memberOK && occurrence.bind.input.OwnsValuesMember(member)
}

// Cell returns this transfer's exact existing destination storage Cell.
func (occurrence StorageBindOccurrence) Cell() (Cell, bool) {
	if !occurrence.Available() {
		return Cell{}, false
	}
	return occurrence.bind.input.storageCell(occurrence.target)
}
func (input TransformerInput) OwnsStorageBind(bind StorageBind) bool {
	return input.Available() && bind.input == input && bind.Available()
}
func (input TransformerInput) OwnsStorageBindOccurrence(occurrence StorageBindOccurrence) bool {
	return input.Available() && occurrence.bind.input == input && occurrence.Available()
}

// StorageAssignment is one executable assignment and Flow's existing ordered
// Write range. It does not synthesize a Write inverse from the flat Write
// denominator.
type StorageAssignment struct {
	input        TransformerInput
	cursor       int
	term, values keyspace.Term
	width        int
	body         Body
	span         Span
}

// StorageWriteOccurrence is one fixed assignment position. Write has only a
// Finish site; its input is the opaque, owner-issued assignment predecessor.
type StorageWriteOccurrence struct {
	assignment  StorageAssignment
	position    int
	term        keyspace.Term
	value       keyspace.Term
	target      keyspace.Term
	finish      flow.Site
	predecessor AssignmentPredecessor
	eligible    bool
}

// AssignmentPredecessor wraps Flow's final reverse-commit Successor without
// exposing its endpoint Terms or a generic route mapper.
type AssignmentPredecessor struct {
	input    TransformerInput
	write    keyspace.Term
	finish   flow.Site
	identity flow.RouteIdentity
	route    identity.ContentID
}

func (input TransformerInput) StorageAssignmentCount() int {
	if !input.Available() {
		return 0
	}
	return input.owner.Flow().Authored().Storage().Assigns().Count()
}

func (input TransformerInput) StorageAssignmentAt(cursor int) (StorageAssignment, bool) {
	result, ok := input.storageAssignment(cursor)
	return result, ok && result.Available()
}

func (input TransformerInput) storageAssignment(cursor int) (StorageAssignment, bool) {
	if !input.Available() || cursor < 0 {
		return StorageAssignment{}, false
	}
	flowView := input.owner.Flow()
	assigns := flowView.Authored().Storage().Assigns()
	term, present := assigns.At(cursor)
	owner, values, related := assigns.Get(term)
	width, widthOK := assigns.WriteCount(term)
	if !present || !related || !widthOK || width <= 0 || !flowView.Executable().Contains(term) {
		return StorageAssignment{}, false
	}
	if _, _, valuesOK := flowView.Authored().Values().Get(values); !valuesOK {
		return StorageAssignment{}, false
	}
	body, bodyOK := input.occurrenceBody(owner, term)
	span, spanOK := input.Span(term)
	if !bodyOK || !spanOK {
		return StorageAssignment{}, false
	}
	return StorageAssignment{input: input, cursor: cursor, term: term, values: values, width: width, body: body, span: span}, true
}

func (assignment StorageAssignment) Available() bool {
	if !assignment.input.Available() || assignment.cursor < 0 || assignment.term == 0 || assignment.values == 0 ||
		assignment.width <= 0 || !assignment.input.OwnsBody(assignment.body) || !exactSpan(assignment.input, assignment.span, assignment.term) {
		return false
	}
	expected, ok := assignment.input.storageAssignment(assignment.cursor)
	return ok && expected.term == assignment.term && expected.values == assignment.values && expected.width == assignment.width && expected.body.Equal(assignment.body) && expected.span.Equal(assignment.span)
}

func (assignment StorageAssignment) ContextID() identity.ContentID {
	if !assignment.Available() {
		return identity.ContentID{}
	}
	pathID := assignment.body.PathID()
	structuralID, structuralOK := assignment.input.owner.Flow().StorageAssignmentPath(assignment.term)
	if !structuralOK {
		return identity.ContentID{}
	}
	return transformerSemanticID("program/transformer/storage-assignment", func(writer *framing.Writer) bool {
		return writer.Bytes(pathID[:]) == nil && writer.Bytes(structuralID[:]) == nil && writer.Count(uint64(assignment.width)) == nil
	})
}

func (assignment StorageAssignment) Body() (Body, bool) {
	return assignment.body, assignment.Available()
}
func (assignment StorageAssignment) Span() (Span, bool) {
	if !assignment.Available() {
		return Span{}, false
	}
	return assignment.span, true
}

// Values is the exact owner-issued source Values occurrence for this
// assignment. It retains authored order without exposing its raw coordinate.
func (assignment StorageAssignment) Values() (ValuesOccurrence, bool) {
	if !assignment.Available() {
		return ValuesOccurrence{}, false
	}
	values, ok := assignment.input.valuesForTerm(assignment.values)
	// Assignment width is the destination denominator. Source Values may end
	// in a Call/Vararg tail and therefore legitimately expose fewer fixed
	// members; TransferAt issues only the positions the parent proves fixed.
	return values, ok && assignment.input.OwnsValuesOccurrence(values)
}
func (assignment StorageAssignment) TransferCount() int {
	if !assignment.Available() {
		return 0
	}
	return assignment.width
}

func (assignment StorageAssignment) TransferAt(position int) (StorageWriteOccurrence, bool) {
	result, ok := assignment.storageWriteOccurrence(position)
	return result, ok && result.Available()
}

func (assignment StorageAssignment) storageWriteOccurrence(position int) (StorageWriteOccurrence, bool) {
	if !assignment.Available() || position < 0 || position >= assignment.width {
		return StorageWriteOccurrence{}, false
	}
	flowView := assignment.input.owner.Flow()
	storage := flowView.Authored().Storage()
	write, writeOK := storage.Assigns().WriteAt(assignment.term, position)
	actualAssignment, target, related := storage.Writes().Get(write)
	value, fixed := flowView.Authored().Values().Member(assignment.values, position)
	if !writeOK || !related || actualAssignment != assignment.term || !fixed {
		return StorageWriteOccurrence{}, false
	}
	if _, _, _, ok := storage.Cells().Get(target); !ok {
		return StorageWriteOccurrence{}, false
	}
	finish, finishOK := assignment.input.occurrenceFinish(write)
	if !finishOK {
		return StorageWriteOccurrence{}, false
	}
	predecessor, predecessorOK := assignment.input.assignmentPredecessor(write, finish)
	if !predecessorOK {
		return StorageWriteOccurrence{}, false
	}
	exactLens := flowView.Authored().Access().Exact()
	dynamicLens := flowView.Authored().Access().Dynamic()
	_, _, _, _, eligibleExact := exactLens.Get(target)
	_, _, _, eligibleDynamic := dynamicLens.Get(target)
	return StorageWriteOccurrence{
		assignment: assignment, position: position, term: write, value: value, target: target,
		finish: finish, predecessor: predecessor, eligible: eligibleExact || eligibleDynamic,
	}, true
}

// Eligible reports whether this write targets an exact or dynamic Lens. The
// classification is copied from the sealed Program proof for artifact rows.
func (write StorageWriteOccurrence) Eligible() bool { return write.Available() && write.eligible }

func (write StorageWriteOccurrence) Available() bool {
	if !write.assignment.Available() || write.position < 0 || write.term == 0 || write.value == 0 || write.target == 0 ||
		write.predecessor.input != write.assignment.input || !write.assignment.input.OwnsSite(write.finish) ||
		!write.assignment.input.OwnsAssignmentPredecessor(write.predecessor) {
		return false
	}
	expected, ok := write.assignment.storageWriteOccurrence(write.position)
	return ok && expected.term == write.term && expected.value == write.value && expected.target == write.target &&
		expected.eligible == write.eligible &&
		expected.finish.Equal(write.finish) && expected.predecessor.input == write.predecessor.input &&
		expected.predecessor.write == write.predecessor.write && expected.predecessor.finish.Equal(write.predecessor.finish) &&
		expected.predecessor.identity.Equal(write.predecessor.identity)
}

func (write StorageWriteOccurrence) ContextID() identity.ContentID {
	if !write.Available() {
		return identity.ContentID{}
	}
	assignmentID, finishID, predecessorID := write.assignment.ContextID(), write.finish.ContextID(), write.predecessor.ContextID()
	return transformerRoleID("program/transformer/storage-write-transfer", write.assignment.input.programID, func(writer *framing.Writer) bool {
		return writer.Bytes(assignmentID[:]) == nil && writer.Uint(uint64(write.position)) == nil &&
			writer.Bytes(finishID[:]) == nil && writer.Bytes(predecessorID[:]) == nil
	})
}

func (write StorageWriteOccurrence) Body() (Body, bool) {
	return write.assignment.body, write.Available()
}
func (write StorageWriteOccurrence) Finish() (flow.Site, bool) {
	return write.finish, write.Available()
}
func (write StorageWriteOccurrence) Predecessor() (AssignmentPredecessor, bool) {
	return write.predecessor, write.Available()
}

// Value returns this write's exact ordered source Values member.
func (write StorageWriteOccurrence) Value() (ValuesMember, bool) {
	if !write.Available() {
		return ValuesMember{}, false
	}
	values, valuesOK := write.assignment.Values()
	member, memberOK := values.At(write.position)
	return member, valuesOK && memberOK && write.assignment.input.OwnsValuesMember(member)
}

// Cell returns this write's exact existing destination storage Cell.
func (write StorageWriteOccurrence) Cell() (Cell, bool) {
	if !write.Available() {
		return Cell{}, false
	}
	return write.assignment.input.storageCell(write.target)
}
func (input TransformerInput) OwnsStorageAssignment(assignment StorageAssignment) bool {
	return input.Available() && assignment.input == input && assignment.Available()
}
func (input TransformerInput) OwnsStorageWriteOccurrence(write StorageWriteOccurrence) bool {
	return input.Available() && write.assignment.input == input && write.Available()
}

func (input TransformerInput) assignmentPredecessor(write keyspace.Term, finish flow.Site) (AssignmentPredecessor, bool) {
	finishTerm, finishOK := input.ownedSite(finish)
	portFinish, portFinishOK := input.owner.Flow().Ports().Finish(write)
	successor, successorOK := input.owner.Flow().Causal().Successors().AssignmentPredecessor(write)
	identity, identityOK := successor.Identity()
	route, routeOK := successor.SemanticID()
	if !finishOK || !portFinishOK || portFinish != finishTerm || !successorOK || !identityOK || !routeOK || !route.Available() || successor.To != finishTerm || successor.Arm != flow.BoundaryLocal ||
		identity.To() != finishTerm || identity.Arm() != flow.BoundaryLocal || identity.Provenance() != input.owner.Flow().Provenance() {
		return AssignmentPredecessor{}, false
	}
	result := AssignmentPredecessor{input: input, write: write, finish: finish, identity: identity, route: route}
	return result, true
}

func (predecessor AssignmentPredecessor) Available() bool {
	if !predecessor.input.Available() || predecessor.write == 0 || !predecessor.input.OwnsSite(predecessor.finish) || !predecessor.identity.Available() || !predecessor.route.Available() {
		return false
	}
	expected, ok := predecessor.input.assignmentPredecessor(predecessor.write, predecessor.finish)
	return ok && expected.identity.Equal(predecessor.identity) && expected.route == predecessor.route
}

func (predecessor AssignmentPredecessor) ContextID() identity.ContentID {
	if !predecessor.Available() {
		return identity.ContentID{}
	}
	finishID, digest := predecessor.finish.ContextID(), predecessor.identity.Digest()
	return transformerRoleID("program/transformer/assignment-predecessor", predecessor.input.programID, func(writer *framing.Writer) bool {
		return writer.Bytes(finishID[:]) == nil && writer.Bytes(predecessor.route[:]) == nil && writer.Bytes(digest[:]) == nil
	})
}

func (predecessor AssignmentPredecessor) RouteDigest() (identity.ContentID, bool) {
	if !predecessor.Available() {
		return identity.ContentID{}, false
	}
	return predecessor.identity.Digest(), true
}

// RouteID is the parent final-route semantic identity. It is the exact join
// key used by reusable artifacts to retain the predecessor's guard, reset,
// and source point without exposing endpoint Terms.
func (predecessor AssignmentPredecessor) RouteID() (identity.ContentID, bool) {
	if !predecessor.Available() {
		return identity.ContentID{}, false
	}
	return predecessor.route, true
}

func (input TransformerInput) OwnsAssignmentPredecessor(predecessor AssignmentPredecessor) bool {
	return input.Available() && predecessor.input == input && predecessor.Available()
}

func (input TransformerInput) occurrenceBody(owner, occurrence keyspace.Term) (Body, bool) {
	if !input.Available() || owner == 0 || occurrence == 0 {
		return Body{}, false
	}
	body, bodyOK := input.Body(owner)
	// Storage's authored row owns the execution Body. A Repeat condition is
	// syntactically contained by the Repeat statement while its reads execute
	// in the loop Body; Position is therefore not an equivalent ownership
	// proof and must not be reopened as a second authority here.
	return body, bodyOK && input.OwnsBody(body)
}

func (input TransformerInput) occurrenceSites(term keyspace.Term) (entry, finish flow.Site, ok bool) {
	if !input.Available() || term == 0 {
		return flow.Site{}, flow.Site{}, false
	}
	ports, sites := input.owner.Flow().Ports(), input.owner.Flow().Causal().Sites()
	entryTerm, entryOK := ports.Entry(term)
	finishTerm, finishOK := ports.Finish(term)
	entry, entrySiteOK := sites.ForTerm(entryTerm)
	finish, finishSiteOK := sites.ForTerm(finishTerm)
	return entry, finish, entryOK && finishOK && entrySiteOK && finishSiteOK && input.OwnsSite(entry) && input.OwnsSite(finish)
}

func (input TransformerInput) occurrenceFinish(term keyspace.Term) (flow.Site, bool) {
	if !input.Available() || term == 0 {
		return flow.Site{}, false
	}
	finishTerm, finishOK := input.owner.Flow().Ports().Finish(term)
	finish, siteOK := input.owner.Flow().Causal().Sites().ForTerm(finishTerm)
	return finish, finishOK && siteOK && input.OwnsSite(finish)
}

func writeTransformerTerm(writer *framing.Writer, term keyspace.Term) bool {
	return writer != nil && keyspace.TermFamily(term) != keyspace.FamilyInvalid && keyspace.TermOrdinal(term) != 0 &&
		writer.Uint(uint64(keyspace.TermFamily(term))) == nil && writer.Uint(uint64(keyspace.TermOrdinal(term))) == nil
}
