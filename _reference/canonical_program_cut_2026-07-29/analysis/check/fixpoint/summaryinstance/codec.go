package summaryinstance

import (
	"context"
	"fmt"
	"math"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/interproc"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

func encodeOutcome(ctx context.Context, schema FormatSchema, in PortableClosedOutcome) ([]byte, error) {
	var writer canonical.Writer
	if err := writer.ResetBuffer(ctx, codecDomain, codecVersion); err != nil {
		return nil, err
	}
	if err := writer.Record(outerRecord); err != nil {
		return nil, err
	}
	if err := writeID(&writer, schema.ID()); err != nil {
		return nil, err
	}
	if err := writeID(&writer, in.DemandedArtifactID); err != nil {
		return nil, err
	}
	if err := writer.Bytes(in.InstanceProjectionBytes); err != nil {
		return nil, err
	}
	if err := writeID(&writer, in.InstanceProjectionID); err != nil {
		return nil, err
	}
	if err := encodeFacts(&writer, valuesRecord, in.Values); err != nil {
		return nil, err
	}
	if err := encodeFacts(&writer, outcomesRecord, in.Outcomes); err != nil {
		return nil, err
	}
	if err := encodeAllocations(&writer, in.AllocationTransport); err != nil {
		return nil, err
	}
	if err := encodeResiduals(&writer, in.ApplicationResiduals); err != nil {
		return nil, err
	}
	if err := encodeCallees(&writer, in.CalleeInstanceKeys); err != nil {
		return nil, err
	}
	if err := encodeDependencies(&writer, in.DependencyIDs); err != nil {
		return nil, err
	}
	if err := writer.Record(resultDigestRecord); err != nil {
		return nil, err
	}
	if err := writeID(&writer, in.ResultDigest); err != nil {
		return nil, err
	}
	return writer.FinishBytes()
}

func encodeResult(ctx context.Context, in PortableClosedOutcome) ([]byte, error) {
	var writer canonical.Writer
	if err := writer.ResetBuffer(ctx, resultDomain, resultVersion); err != nil {
		return nil, err
	}
	if err := writer.Record(semanticResultRecord); err != nil {
		return nil, err
	}
	if err := writeID(&writer, in.DemandedArtifactID); err != nil {
		return nil, err
	}
	if err := writer.Bytes(in.InstanceProjectionBytes); err != nil {
		return nil, err
	}
	if err := writeID(&writer, in.InstanceProjectionID); err != nil {
		return nil, err
	}
	if err := encodeFacts(&writer, valuesRecord, in.Values); err != nil {
		return nil, err
	}
	if err := encodeFacts(&writer, outcomesRecord, in.Outcomes); err != nil {
		return nil, err
	}
	if err := encodeAllocations(&writer, in.AllocationTransport); err != nil {
		return nil, err
	}
	if err := encodeResiduals(&writer, in.ApplicationResiduals); err != nil {
		return nil, err
	}
	if err := encodeCallees(&writer, in.CalleeInstanceKeys); err != nil {
		return nil, err
	}
	if err := encodeDependencies(&writer, in.DependencyIDs); err != nil {
		return nil, err
	}
	return writer.FinishBytes()
}

func writeID(writer *canonical.Writer, id interproc.ContentID) error { return writer.Bytes(id[:]) }

func readID(reader *canonical.Reader) (interproc.ContentID, error) {
	raw, err := reader.Bytes()
	if err != nil {
		return interproc.ContentID{}, err
	}
	if len(raw) != len(interproc.ContentID{}) {
		return interproc.ContentID{}, fmt.Errorf("summaryinstance: malformed content ID")
	}
	var out interproc.ContentID
	copy(out[:], raw)
	if !out.Valid() {
		return interproc.ContentID{}, fmt.Errorf("summaryinstance: zero content ID")
	}
	return out, nil
}

func encodeFacts(writer *canonical.Writer, record uint64, facts []Fact) error {
	if err := writer.Record(record); err != nil {
		return err
	}
	if err := writer.Count(uint64(len(facts))); err != nil {
		return err
	}
	for _, fact := range facts {
		if err := writer.Record(factRecord); err != nil {
			return err
		}
		if err := writer.String(fact.Key); err != nil {
			return err
		}
		if err := writer.Bytes(fact.Value); err != nil {
			return err
		}
	}
	return nil
}

func decodeFacts(reader *canonical.Reader, want uint64) ([]Fact, error) {
	record, err := reader.Record()
	if err != nil || record != want {
		return nil, decodeError("fact list record", err)
	}
	count, err := decodeCount(reader)
	if err != nil {
		return nil, err
	}
	out := make([]Fact, count)
	for index := range out {
		record, err = reader.Record()
		if err != nil || record != factRecord {
			return nil, decodeError("fact record", err)
		}
		if out[index].Key, err = reader.String(); err != nil {
			return nil, err
		}
		if out[index].Value, err = reader.Bytes(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func encodeAllocations(writer *canonical.Writer, allocations []AllocationTransport) error {
	if err := writer.Record(allocationsRecord); err != nil {
		return err
	}
	if err := writer.Count(uint64(len(allocations))); err != nil {
		return err
	}
	for _, allocation := range allocations {
		if err := writer.Record(allocationRecord); err != nil {
			return err
		}
		if err := writeID(writer, allocation.TemplateID); err != nil {
			return err
		}
		if err := writeID(writer, allocation.ResultID); err != nil {
			return err
		}
	}
	return nil
}

func decodeAllocations(reader *canonical.Reader) ([]AllocationTransport, error) {
	record, err := reader.Record()
	if err != nil || record != allocationsRecord {
		return nil, decodeError("allocation list record", err)
	}
	count, err := decodeCount(reader)
	if err != nil {
		return nil, err
	}
	out := make([]AllocationTransport, count)
	for index := range out {
		record, err = reader.Record()
		if err != nil || record != allocationRecord {
			return nil, decodeError("allocation record", err)
		}
		if out[index].TemplateID, err = readID(reader); err != nil {
			return nil, decodeError("allocation template", err)
		}
		if out[index].ResultID, err = readID(reader); err != nil {
			return nil, decodeError("allocation result", err)
		}
	}
	return out, nil
}

func encodeResiduals(writer *canonical.Writer, residuals []ApplicationResidual) error {
	if err := writer.Record(residualsRecord); err != nil {
		return err
	}
	if err := writer.Count(uint64(len(residuals))); err != nil {
		return err
	}
	for _, residual := range residuals {
		if err := writer.Record(residualRecord); err != nil {
			return err
		}
		for _, id := range []interproc.ContentID{residual.DescriptorID, residual.PredicateID, residual.EvidenceID, residual.GuardID, residual.BoundaryID} {
			if err := writeID(writer, id); err != nil {
				return err
			}
		}
		if err := writer.Uint(uint64(residual.Decision)); err != nil {
			return err
		}
		if residual.Decision == ResidualFailing {
			if err := writeID(writer, residual.BoundStateID); err != nil {
				return err
			}
		} else if err := writer.Nil(); err != nil {
			return err
		}
	}
	return nil
}

func decodeResiduals(reader *canonical.Reader) ([]ApplicationResidual, error) {
	record, err := reader.Record()
	if err != nil || record != residualsRecord {
		return nil, decodeError("residual list record", err)
	}
	count, err := decodeCount(reader)
	if err != nil {
		return nil, err
	}
	out := make([]ApplicationResidual, count)
	for index := range out {
		record, err = reader.Record()
		if err != nil || record != residualRecord {
			return nil, decodeError("residual record", err)
		}
		ids := []*interproc.ContentID{&out[index].DescriptorID, &out[index].PredicateID, &out[index].EvidenceID, &out[index].GuardID, &out[index].BoundaryID}
		for _, target := range ids {
			if *target, err = readID(reader); err != nil {
				return nil, decodeError("residual content ID", err)
			}
		}
		decision, err := reader.Uint()
		if err != nil || decision > math.MaxUint8 {
			return nil, decodeError("residual decision", err)
		}
		out[index].Decision = ResidualDecision(decision)
		if out[index].Decision == ResidualFailing {
			if out[index].BoundStateID, err = readID(reader); err != nil {
				return nil, decodeError("positive feasibility proof", err)
			}
		} else if err := reader.Nil(); err != nil {
			return nil, decodeError("non-positive feasibility proof", err)
		}
	}
	return out, nil
}

func encodeCallees(writer *canonical.Writer, callees []InstanceKey) error {
	if err := writer.Record(calleesRecord); err != nil {
		return err
	}
	if err := writer.Count(uint64(len(callees))); err != nil {
		return err
	}
	for _, callee := range callees {
		if err := writer.Record(calleeRecord); err != nil {
			return err
		}
		if err := writeID(writer, callee.DemandedArtifactID); err != nil {
			return err
		}
		if err := writer.Bytes(callee.InstanceProjectionBytes); err != nil {
			return err
		}
		if err := writeID(writer, callee.InstanceProjectionID); err != nil {
			return err
		}
	}
	return nil
}

func decodeCallees(reader *canonical.Reader) ([]InstanceKey, error) {
	record, err := reader.Record()
	if err != nil || record != calleesRecord {
		return nil, decodeError("callee list record", err)
	}
	count, err := decodeCount(reader)
	if err != nil {
		return nil, err
	}
	out := make([]InstanceKey, count)
	for index := range out {
		record, err = reader.Record()
		if err != nil || record != calleeRecord {
			return nil, decodeError("callee record", err)
		}
		if out[index].DemandedArtifactID, err = readID(reader); err != nil {
			return nil, decodeError("callee artifact", err)
		}
		if out[index].InstanceProjectionBytes, err = reader.Bytes(); err != nil {
			return nil, err
		}
		if out[index].InstanceProjectionID, err = readID(reader); err != nil {
			return nil, decodeError("callee projection", err)
		}
	}
	return out, nil
}

func encodeDependencies(writer *canonical.Writer, dependencies []interproc.ContentID) error {
	if err := writer.Record(dependenciesRecord); err != nil {
		return err
	}
	if err := writer.Count(uint64(len(dependencies))); err != nil {
		return err
	}
	for _, id := range dependencies {
		if err := writeID(writer, id); err != nil {
			return err
		}
	}
	return nil
}

func decodeDependencies(reader *canonical.Reader) ([]interproc.ContentID, error) {
	record, err := reader.Record()
	if err != nil || record != dependenciesRecord {
		return nil, decodeError("dependency list record", err)
	}
	count, err := decodeCount(reader)
	if err != nil {
		return nil, err
	}
	out := make([]interproc.ContentID, count)
	for index := range out {
		if out[index], err = readID(reader); err != nil {
			return nil, decodeError("dependency content ID", err)
		}
	}
	return out, nil
}

func decodeCount(reader *canonical.Reader) (int, error) {
	count, err := reader.Count()
	if err != nil {
		return 0, err
	}
	if count > uint64(reader.RemainingBytes()) || count > uint64(math.MaxInt) {
		return 0, fmt.Errorf("summaryinstance: unreasonable record count")
	}
	return int(count), nil
}

func decodeError(part string, err error) error {
	if err != nil {
		return fmt.Errorf("summaryinstance: invalid %s: %w", part, err)
	}
	return fmt.Errorf("summaryinstance: invalid %s", part)
}
