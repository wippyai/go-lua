package evidence

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

const canonicalValueRecord uint64 = 1

func canonicalDescriptor() axis.CanonicalDescriptor[Value] {
	return axis.ReadyCanonicalBidirectional("value.axis.evidence", 1, encodeCanonical, decodeCanonical)
}

func decodeCanonical(_ context.Context, reader *canonical.Reader) (Value, error) {
	record, err := reader.Record()
	if err != nil {
		return Bottom(), err
	}
	if record != canonicalValueRecord {
		return Bottom(), fmt.Errorf("evidence: invalid canonical record %d", record)
	}
	rawKind, err := reader.Uint()
	if err != nil {
		return Bottom(), err
	}
	if rawKind > uint64(top) {
		return Bottom(), fmt.Errorf("evidence: invalid canonical kind")
	}
	rawCount, err := reader.Uint()
	if err != nil {
		return Bottom(), err
	}
	if rawCount > maxOrigins {
		return Bottom(), fmt.Errorf("evidence: invalid canonical origin count %d", rawCount)
	}
	truncated, err := reader.Bool()
	if err != nil {
		return Bottom(), err
	}
	value := Value{kind: kind(rawKind), origins: originSet{count: uint8(rawCount), truncated: truncated}}
	for index := range value.origins.items {
		rawOriginKind, err := reader.Uint()
		if err != nil {
			return Bottom(), err
		}
		if rawOriginKind > uint64(OriginAnnotation) {
			return Bottom(), fmt.Errorf("evidence: invalid canonical origin kind")
		}
		id, err := reader.Uint()
		if err != nil {
			return Bottom(), err
		}
		value.origins.items[index] = Origin{Kind: OriginKind(rawOriginKind), ID: id}
	}
	if err := validateCanonicalValue(value); err != nil {
		return Bottom(), err
	}
	return value, nil
}

func validateCanonicalValue(value Value) error {
	if value.kind == bottom || value.kind == top {
		if value.origins != (originSet{}) {
			return fmt.Errorf("evidence: terminal canonical value carries origins")
		}
		return nil
	}
	if value.kind != gradualTop && value.kind != explicitTop {
		return fmt.Errorf("evidence: invalid canonical kind")
	}
	count := int(value.origins.count)
	if count > maxOrigins || value.origins.truncated && count != maxOrigins {
		return fmt.Errorf("evidence: invalid canonical origin metadata")
	}
	for index, origin := range value.origins.items {
		if index >= count {
			if origin != (Origin{}) {
				return fmt.Errorf("evidence: inactive canonical origin slot is nonzero")
			}
			continue
		}
		if origin.Kind <= OriginUnknown || origin.Kind > OriginAnnotation {
			return fmt.Errorf("evidence: invalid active canonical origin kind")
		}
		if index > 0 && !originLess(value.origins.items[index-1], origin) {
			return fmt.Errorf("evidence: canonical origins are not in strict order")
		}
	}
	return nil
}

// encodeCanonical writes the complete fixed carrier observed by Equal. Even
// inactive array slots are included because raw Value equality observes them.
func encodeCanonical(writer *canonical.Writer, v Value) error {
	if err := writer.Record(canonicalValueRecord); err != nil {
		return err
	}
	if err := writer.Uint(uint64(v.kind)); err != nil {
		return err
	}
	if err := writer.Uint(uint64(v.origins.count)); err != nil {
		return err
	}
	if err := writer.Bool(v.origins.truncated); err != nil {
		return err
	}
	for _, origin := range v.origins.items {
		if err := writer.Uint(uint64(origin.Kind)); err != nil {
			return err
		}
		if err := writer.Uint(origin.ID); err != nil {
			return err
		}
	}
	return nil
}
