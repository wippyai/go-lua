package static

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
)

// compactTypes owns the typed static-syntax relations. Field membership is
// intentionally deferred to its one combined containment seal because an
// Interface, owned by Declarations, can also claim a Field.
func compactTypes(component *Component, counts [keyspace.FamilyCount]uint32, input TypesInput) error {
	store := &component.types
	if !validLeafRows(store) {
		return errors.New("program/static: invalid primitive, literal, or field")
	}
	for _, row := range input.Optional {
		if !staticrole.Node(counts, row.Inner) {
			return errors.New("program/static: invalid optional child")
		}
	}
	if !validMembers(counts, input.Union) || !validIntersections(counts, input.Intersection) {
		return errors.New("program/static: invalid compound child")
	}
	for _, row := range input.Generic {
		if !hasFamily(counts, row.Base, keyspace.FamilyTypeRef) || len(row.Args) == 0 {
			return errors.New("program/static: invalid generic base or arity")
		}
		for _, arg := range row.Args {
			if !staticrole.Node(counts, arg) {
				return errors.New("program/static: invalid generic argument")
			}
		}
	}
	for _, row := range input.Array {
		if !staticrole.Node(counts, row.Element) {
			return errors.New("program/static: invalid array element")
		}
	}
	for _, row := range input.Map {
		if !staticrole.Node(counts, row.Key) || !staticrole.Node(counts, row.Value) {
			return errors.New("program/static: invalid map child")
		}
	}
	for _, row := range input.Record {
		for _, field := range row.Fields {
			if !hasFamily(counts, field, keyspace.FamilyTypeField) {
				return errors.New("program/static: invalid record field")
			}
		}
	}
	return appendTypePools(store, input)
}

func validLeafRows(store *typeStore) bool {
	for _, row := range store.primitive {
		if !row.Kind.valid() {
			return false
		}
	}
	for _, row := range store.literal {
		switch row.Kind {
		case keyspace.LiteralBool, keyspace.LiteralInteger, keyspace.LiteralString:
			if row.Exact == 0 || row.FloatBits != 0 {
				return false
			}
		case keyspace.LiteralFloat:
			if row.Exact != 0 {
				return false
			}
		default:
			return false
		}
	}
	for _, row := range store.field {
		if row.Key == 0 {
			return false
		}
	}
	return true
}

func appendTypePools(store *typeStore, input TypesInput) error {
	for _, row := range input.Union {
		range_, ok := appendTerms(&store.terms, row.Members)
		if !ok {
			return errors.New("program/static: oversized union")
		}
		store.union = append(store.union, range_)
	}
	for _, row := range input.Intersection {
		range_, ok := appendTerms(&store.terms, row.Members)
		if !ok {
			return errors.New("program/static: oversized intersection")
		}
		store.intersection = append(store.intersection, range_)
	}
	for _, row := range input.Generic {
		range_, ok := appendTerms(&store.terms, row.Args)
		if !ok {
			return errors.New("program/static: oversized generic")
		}
		store.generic = append(store.generic, genericRow{base: row.Base, args: range_})
	}
	for _, row := range input.Record {
		range_, ok := appendTerms(&store.fields, row.Fields)
		if !ok {
			return errors.New("program/static: oversized record")
		}
		store.record = append(store.record, recordRow{fields: range_, readOnly: row.ReadOnly})
	}
	return nil
}

func validMembers(counts [keyspace.FamilyCount]uint32, rows []Union) bool {
	for _, row := range rows {
		if len(row.Members) < 2 {
			return false
		}
		for _, term := range row.Members {
			if !staticrole.Node(counts, term) {
				return false
			}
		}
	}
	return true
}

func validIntersections(counts [keyspace.FamilyCount]uint32, rows []Intersection) bool {
	for _, row := range rows {
		if len(row.Members) < 2 {
			return false
		}
		for _, term := range row.Members {
			if !staticrole.Node(counts, term) {
				return false
			}
		}
	}
	return true
}

// staticNodeFamily is the one closed authored-static-type vocabulary. It is
// private because callers must use their owner-specific row relation rather
// than acquire a generic Static node API.
func staticNodeFamily(family keyspace.Family) bool {
	switch family {
	case keyspace.FamilyTypePrimitive, keyspace.FamilyTypeLiteral, keyspace.FamilyTypeOptional,
		keyspace.FamilyTypeUnion, keyspace.FamilyTypeIntersection, keyspace.FamilyTypeRef,
		keyspace.FamilyTypeGeneric, keyspace.FamilyTypeArray, keyspace.FamilyTypeMap,
		keyspace.FamilyTypeRecord, keyspace.FamilyTypeFunction, keyspace.FamilyTypeAsserts,
		keyspace.FamilyTypeOf, keyspace.FamilyTypeKeyOf, keyspace.FamilyTypeIndexAccess,
		keyspace.FamilyTypeConditional:
		return true
	default:
		return false
	}
}

// writeTypesContent owns the exact authored scalar order of the Types
// vertical. Pool ranges are storage layout and are intentionally re-expanded
// into their semantic sequences here.
func writeTypesContent(writer *framing.Writer, store typeStore) error {
	if err := writer.Count(uint64(len(store.primitive))); err != nil {
		return err
	}
	for _, row := range store.primitive {
		if err := writer.Uint(uint64(row.Kind)); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.literal))); err != nil {
		return err
	}
	for _, row := range store.literal {
		if err := writer.Uint(uint64(row.Kind)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Exact)); err != nil {
			return err
		}
		if err := writer.Uint(row.FloatBits); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.optional))); err != nil {
		return err
	}
	for _, row := range store.optional {
		if err := writer.Uint(uint64(row.Inner)); err != nil {
			return err
		}
	}
	if err := writeTypeTermRangesContent(writer, store.union, store.terms); err != nil {
		return err
	}
	if err := writeTypeTermRangesContent(writer, store.intersection, store.terms); err != nil {
		return err
	}
	if err := writer.Count(uint64(len(store.generic))); err != nil {
		return err
	}
	for _, row := range store.generic {
		if err := writer.Uint(uint64(row.base)); err != nil {
			return err
		}
		if err := writeTypeTermsContent(writer, store.terms[row.args.Start:row.args.End]); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.array))); err != nil {
		return err
	}
	for _, row := range store.array {
		if err := writer.Uint(uint64(row.Element)); err != nil {
			return err
		}
		if err := writer.Bool(row.ReadOnly); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.mapType))); err != nil {
		return err
	}
	for _, row := range store.mapType {
		if err := writer.Uint(uint64(row.Key)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Value)); err != nil {
			return err
		}
		if err := writer.Bool(row.ReadOnly); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.record))); err != nil {
		return err
	}
	for _, row := range store.record {
		if err := writer.Bool(row.readOnly); err != nil {
			return err
		}
		if err := writeTypeTermsContent(writer, store.fields[row.fields.Start:row.fields.End]); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.field))); err != nil {
		return err
	}
	for _, row := range store.field {
		if err := writer.Uint(uint64(row.Key)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Type)); err != nil {
			return err
		}
		if err := writer.Bool(row.Optional); err != nil {
			return err
		}
	}
	return nil
}

func writeTypeTermRangesContent(writer *framing.Writer, rows []poolRange, terms []keyspace.Term) error {
	if err := writer.Count(uint64(len(rows))); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writeTypeTermsContent(writer, terms[row.Start:row.End]); err != nil {
			return err
		}
	}
	return nil
}

func writeTypeTermsContent(writer *framing.Writer, terms []keyspace.Term) error {
	if err := writer.Count(uint64(len(terms))); err != nil {
		return err
	}
	for _, term := range terms {
		if err := writer.Uint(uint64(term)); err != nil {
			return err
		}
	}
	return nil
}

// emitTypesContainment owns the concrete containment written by the Types
// vertical. Record-to-Field membership remains distinct from Field-to-Type.
func emitTypesContainment(component *Component, check *containment) bool {
	store := &component.types
	for index, row := range store.optional {
		if !check.attach(keyspace.MakeTerm(keyspace.FamilyTypeOptional, uint32(index+1)), row.Inner) {
			return false
		}
	}
	for index, row := range store.union {
		parent := keyspace.MakeTerm(keyspace.FamilyTypeUnion, uint32(index+1))
		for _, child := range store.terms[row.Start:row.End] {
			if !check.attach(parent, child) {
				return false
			}
		}
	}
	for index, row := range store.intersection {
		parent := keyspace.MakeTerm(keyspace.FamilyTypeIntersection, uint32(index+1))
		for _, child := range store.terms[row.Start:row.End] {
			if !check.attach(parent, child) {
				return false
			}
		}
	}
	for index, row := range store.generic {
		parent := keyspace.MakeTerm(keyspace.FamilyTypeGeneric, uint32(index+1))
		if !check.attach(parent, row.base) {
			return false
		}
		for _, child := range store.terms[row.args.Start:row.args.End] {
			if !check.attach(parent, child) {
				return false
			}
		}
	}
	for index, row := range store.array {
		if !check.attach(keyspace.MakeTerm(keyspace.FamilyTypeArray, uint32(index+1)), row.Element) {
			return false
		}
	}
	for index, row := range store.mapType {
		parent := keyspace.MakeTerm(keyspace.FamilyTypeMap, uint32(index+1))
		if !check.attach(parent, row.Key) || !check.attach(parent, row.Value) {
			return false
		}
	}
	for index, row := range store.field {
		if !check.attach(keyspace.MakeTerm(keyspace.FamilyTypeField, uint32(index+1)), row.Type) {
			return false
		}
	}
	for index, row := range store.record {
		owner := keyspace.MakeTerm(keyspace.FamilyTypeRecord, uint32(index+1))
		for _, field := range store.fields[row.fields.Start:row.fields.End] {
			if !check.claimField(owner, field) {
				return false
			}
		}
	}
	return true
}
