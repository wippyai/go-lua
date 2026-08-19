package source

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

func readSourceSpans(reader *framing.Reader, name string) ([keyspace.FamilyCount]uint32, []FamilySpans, error) {
	var counts [keyspace.FamilyCount]uint32
	families := make([]FamilySpans, 0, int(keyspace.FamilyCount-1))
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			// Outcome is the one derived family. Its empty authored row is
			// restored in Input at its canonical family position below.
			families = append(families, FamilySpans{Family: family})
			continue
		}
		tag, err := reader.Uint()
		if err != nil {
			return counts, nil, err
		}
		if tag != uint64(family) {
			return counts, nil, framing.ErrMalformed
		}
		count, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 8)
		if err != nil {
			return counts, nil, err
		}
		counts[family] = uint32(count)
		spans := make([]Span, count)
		for index := range spans {
			startLine, err := sourceUint32(reader)
			if err != nil {
				return counts, nil, err
			}
			startCol, err := sourceUint32(reader)
			if err != nil {
				return counts, nil, err
			}
			endLine, err := sourceUint32(reader)
			if err != nil {
				return counts, nil, err
			}
			endCol, err := sourceUint32(reader)
			if err != nil {
				return counts, nil, err
			}
			if _, ok := CoordinateFromParts(startLine, startCol, endLine, endCol); !ok {
				return counts, nil, framing.ErrMalformed
			}
			spans[index] = Span{
				File: name, StartLine: startLine, StartCol: startCol,
				EndLine: endLine, EndCol: endCol,
			}
		}
		families = append(families, FamilySpans{Family: family, Spans: spans})
	}
	return counts, families, nil
}

func sourceUint32(reader *framing.Reader) (uint32, error) {
	value, err := reader.Uint()
	if err != nil {
		return 0, err
	}
	if value > uint64(^uint32(0)) {
		return 0, framing.ErrMalformed
	}
	return uint32(value), nil
}

// preflightSourceSpans consumes one copied Reader without creating any span
// slice. The second pass in readSourceSpans is allowed to allocate only after
// this complete count, coordinate, and family-order proof succeeds.
func preflightSourceSpans(reader *framing.Reader, counts *[keyspace.FamilyCount]uint32) error {
	if reader == nil || counts == nil {
		return framing.ErrMalformed
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		tag, err := reader.Uint()
		if err != nil {
			return err
		}
		if tag != uint64(family) {
			return framing.ErrMalformed
		}
		count, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 8)
		if err != nil {
			return err
		}
		counts[family] = uint32(count)
		for index := 0; index < count; index++ {
			startLine, err := sourceUint32(reader)
			if err != nil {
				return err
			}
			startCol, err := sourceUint32(reader)
			if err != nil {
				return err
			}
			endLine, err := sourceUint32(reader)
			if err != nil {
				return err
			}
			endCol, err := sourceUint32(reader)
			if err != nil {
				return err
			}
			if _, ok := CoordinateFromParts(startLine, startCol, endLine, endCol); !ok {
				return framing.ErrMalformed
			}
		}
	}
	return nil
}
