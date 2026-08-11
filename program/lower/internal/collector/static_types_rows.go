package collector

import (
	"errors"
	"math"

	"github.com/wippyai/go-lua/program/keyspace"
	programstatic "github.com/wippyai/go-lua/program/static"
)

// Scalar and structural type rows. Terms are supplied by the canonical
// collector, so these methods only validate dense identity and append data.
func (rows *staticRows) Primitive(term keyspace.Term, kind programstatic.PrimitiveKind) error {
	if term == 0 || keyspace.TermFamily(term) != keyspace.FamilyTypePrimitive || keyspace.TermOrdinal(term) != uint32(len(rows.primitive)+1) || !kindValid(kind) {
		return errors.New("program/lower/collector: invalid primitive row")
	}
	rows.primitive = append(rows.primitive, programstatic.Primitive{Kind: kind})
	return nil
}

func kindValid(kind programstatic.PrimitiveKind) bool {
	return kind >= programstatic.PrimitiveNil && kind <= programstatic.PrimitiveSelf
}

func (rows *staticRows) LiteralBool(term keyspace.Term, value bool) error {
	return staticLiteralRow(rows, term, keyspace.LiteralBool, keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: value}, 0)
}

func (rows *staticRows) LiteralInteger(term keyspace.Term, value int64) error {
	return staticLiteralRow(rows, term, keyspace.LiteralInteger, keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}, 0)
}

func (rows *staticRows) LiteralFloat(term keyspace.Term, bits uint64) error {
	if math.IsNaN(math.Float64frombits(bits)) {
		return errors.New("program/lower/collector: NaN static float literal")
	}
	return staticLiteralRow(rows, term, keyspace.LiteralFloat, keyspace.LiteralValue{}, bits)
}

func (rows *staticRows) LiteralString(term keyspace.Term, value string) error {
	return staticLiteralRow(rows, term, keyspace.LiteralString, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}, 0)
}

func staticLiteralRow(rows *staticRows, term keyspace.Term, kind keyspace.LiteralKind, exact keyspace.LiteralValue, bits uint64) error {
	if rows == nil || term == 0 || keyspace.TermFamily(term) != keyspace.FamilyTypeLiteral || keyspace.TermOrdinal(term) != uint32(len(rows.literal)+1) {
		return errors.New("program/lower/collector: invalid static literal row")
	}
	row := staticRawLiteral{kind: kind, floatBits: bits}
	if kind != keyspace.LiteralFloat {
		key, err := rawLiteral(exact)
		if err != nil {
			return err
		}
		row.exact = key
	} else if bits == 0 {
		// +0 is a valid float payload. Do not treat zero as absence.
		row.floatBits = bits
	}
	rows.literal = append(rows.literal, row)
	return nil
}

func (rows *staticRows) Optional(term, inner keyspace.Term) error {
	if err := staticDenseAppendTerm(term, keyspace.FamilyTypeOptional, len(rows.optional)); err != nil {
		return err
	}
	rows.optional = append(rows.optional, programstatic.Optional{Inner: inner})
	return nil
}

func (rows *staticRows) Union(term keyspace.Term, members []keyspace.Term) error {
	if err := staticDenseAppendTerm(term, keyspace.FamilyTypeUnion, len(rows.union)); err != nil {
		return err
	}
	if len(members) < 2 {
		return errors.New("program/lower/collector: union requires two members")
	}
	rows.union = append(rows.union, programstatic.Union{Members: append([]keyspace.Term(nil), members...)})
	return nil
}

func (rows *staticRows) Intersection(term keyspace.Term, members []keyspace.Term) error {
	if err := staticDenseAppendTerm(term, keyspace.FamilyTypeIntersection, len(rows.intersection)); err != nil {
		return err
	}
	if len(members) < 2 {
		return errors.New("program/lower/collector: intersection requires two members")
	}
	rows.intersection = append(rows.intersection, programstatic.Intersection{Members: append([]keyspace.Term(nil), members...)})
	return nil
}

func (rows *staticRows) Generic(term, base keyspace.Term, args []keyspace.Term) error {
	if err := staticDenseAppendTerm(term, keyspace.FamilyTypeGeneric, len(rows.generic)); err != nil {
		return err
	}
	if len(args) == 0 {
		return errors.New("program/lower/collector: generic requires arguments")
	}
	rows.generic = append(rows.generic, staticRawGeneric{base: base, args: append([]keyspace.Term(nil), args...)})
	return nil
}

func (rows *staticRows) Array(term, element keyspace.Term, readonly bool) error {
	if err := staticDenseAppendTerm(term, keyspace.FamilyTypeArray, len(rows.array)); err != nil {
		return err
	}
	rows.array = append(rows.array, programstatic.Array{Element: element, ReadOnly: readonly})
	return nil
}

func (rows *staticRows) Map(term, key, value keyspace.Term, readonly bool) error {
	if err := staticDenseAppendTerm(term, keyspace.FamilyTypeMap, len(rows.mapType)); err != nil {
		return err
	}
	rows.mapType = append(rows.mapType, programstatic.Map{Key: key, Value: value, ReadOnly: readonly})
	return nil
}

func (rows *staticRows) Field(term keyspace.Term, key string, typ keyspace.Term, optional bool) error {
	raw, err := rawString(key)
	if err != nil {
		return err
	}
	return rows.FieldRaw(term, raw.value, typ, optional)
}

func (rows *staticRows) FieldRaw(term keyspace.Term, key keyspace.LiteralValue, typ keyspace.Term, optional bool) error {
	if err := staticDenseAppendTerm(term, keyspace.FamilyTypeField, len(rows.field)); err != nil {
		return err
	}
	raw, err := rawLiteral(key)
	if err != nil {
		return err
	}
	if key.Kind != keyspace.LiteralString || key.String == "" {
		return errors.New("program/lower/collector: invalid static field key")
	}
	rows.field = append(rows.field, staticRawField{key: raw, typ: typ, optional: optional})
	return nil
}

func (rows *staticRows) Record(term keyspace.Term, fields []keyspace.Term, readonly bool) error {
	if err := staticDenseAppendTerm(term, keyspace.FamilyTypeRecord, len(rows.record)); err != nil {
		return err
	}
	rows.record = append(rows.record, programstatic.Record{Fields: append([]keyspace.Term(nil), fields...), ReadOnly: readonly})
	return nil
}
