package source

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

// preflightSourceLiterals validates every literal row from a Reader copy.
// String payloads are inspected through StringBytes, so no Go string is copied
// until the allocation/fill pass begins.
func preflightSourceLiterals(reader *framing.Reader, counts [keyspace.FamilyCount]uint32) error {
	if reader == nil {
		return framing.ErrMalformed
	}
	for _, family := range []keyspace.Family{keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger, keyspace.FamilyFloat, keyspace.FamilyString} {
		if err := readSourceLiteralTag(reader, family); err != nil {
			return err
		}
		count, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 4)
		if err != nil {
			return err
		}
		if uint32(count) != counts[family] {
			return framing.ErrMalformed
		}
		for index := 0; index < count; index++ {
			if _, err := readBoundTerm(reader, counts, keyspace.FamilyBody, false); err != nil {
				return err
			}
			switch family {
			case keyspace.FamilyNil:
			case keyspace.FamilyBool:
				if _, err := reader.Bool(); err != nil {
					return err
				}
			case keyspace.FamilyInteger, keyspace.FamilyFloat:
				if _, err := reader.Uint(); err != nil {
					return err
				}
			case keyspace.FamilyString:
				if _, err := sourceStringBytes(reader); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func readSourceLiterals(reader *framing.Reader, input *Input, counts [keyspace.FamilyCount]uint32) error {
	if input == nil {
		return framing.ErrMalformed
	}
	if err := readSourceLiteralTag(reader, keyspace.FamilyNil); err != nil {
		return err
	}
	nilCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 4)
	if err != nil {
		return err
	}
	if uint32(nilCount) != counts[keyspace.FamilyNil] {
		return framing.ErrMalformed
	}
	input.Nil = make([]NilLiteral, nilCount)
	for index := range input.Nil {
		owner, err := readBoundTerm(reader, counts, keyspace.FamilyBody, false)
		if err != nil {
			return err
		}
		input.Nil[index] = NilLiteral{Owner: owner}
	}

	if err := readSourceLiteralTag(reader, keyspace.FamilyBool); err != nil {
		return err
	}
	boolCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 4)
	if err != nil {
		return err
	}
	if uint32(boolCount) != counts[keyspace.FamilyBool] {
		return framing.ErrMalformed
	}
	input.Bool = make([]BoolLiteral, boolCount)
	for index := range input.Bool {
		owner, err := readBoundTerm(reader, counts, keyspace.FamilyBody, false)
		if err != nil {
			return err
		}
		value, err := reader.Bool()
		if err != nil {
			return err
		}
		input.Bool[index] = BoolLiteral{Owner: owner, Value: value}
	}

	if err := readSourceLiteralTag(reader, keyspace.FamilyInteger); err != nil {
		return err
	}
	integerCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 4)
	if err != nil {
		return err
	}
	if uint32(integerCount) != counts[keyspace.FamilyInteger] {
		return framing.ErrMalformed
	}
	input.Integer = make([]IntegerLiteral, integerCount)
	for index := range input.Integer {
		owner, err := readBoundTerm(reader, counts, keyspace.FamilyBody, false)
		if err != nil {
			return err
		}
		value, err := reader.Uint()
		if err != nil {
			return err
		}
		input.Integer[index] = IntegerLiteral{Owner: owner, Value: int64(value)}
	}

	if err := readSourceLiteralTag(reader, keyspace.FamilyFloat); err != nil {
		return err
	}
	floatCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 4)
	if err != nil {
		return err
	}
	if uint32(floatCount) != counts[keyspace.FamilyFloat] {
		return framing.ErrMalformed
	}
	input.Float = make([]FloatLiteral, floatCount)
	for index := range input.Float {
		owner, err := readBoundTerm(reader, counts, keyspace.FamilyBody, false)
		if err != nil {
			return err
		}
		bits, err := reader.Uint()
		if err != nil {
			return err
		}
		input.Float[index] = FloatLiteral{Owner: owner, Bits: bits}
	}

	if err := readSourceLiteralTag(reader, keyspace.FamilyString); err != nil {
		return err
	}
	stringCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 4)
	if err != nil {
		return err
	}
	if uint32(stringCount) != counts[keyspace.FamilyString] {
		return framing.ErrMalformed
	}
	input.String = make([]StringLiteral, stringCount)
	for index := range input.String {
		owner, err := readBoundTerm(reader, counts, keyspace.FamilyBody, false)
		if err != nil {
			return err
		}
		value, err := sourceString(reader)
		if err != nil {
			return err
		}
		input.String[index] = StringLiteral{Owner: owner, Value: value}
	}
	return nil
}

func readSourceLiteralTag(reader *framing.Reader, family keyspace.Family) error {
	tag, err := reader.Uint()
	if err != nil {
		return err
	}
	if tag != uint64(family) {
		return framing.ErrMalformed
	}
	return nil
}
