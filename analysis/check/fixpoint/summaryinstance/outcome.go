package summaryinstance

import (
	"bytes"
	"context"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/interproc"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

const (
	codecDomain  = "analysis.fixpoint.summaryinstance.portable-closed-outcome"
	codecVersion = 1

	resultDomain  = "analysis.fixpoint.summaryinstance.result"
	resultVersion = 1

	outerRecord          uint64 = 1
	valuesRecord         uint64 = 2
	outcomesRecord       uint64 = 3
	allocationsRecord    uint64 = 4
	residualsRecord      uint64 = 5
	calleesRecord        uint64 = 6
	dependenciesRecord   uint64 = 7
	resultDigestRecord   uint64 = 8
	factRecord           uint64 = 9
	allocationRecord     uint64 = 10
	residualRecord       uint64 = 11
	calleeRecord         uint64 = 12
	semanticResultRecord uint64 = 13
)

// FormatSchema is the immutable registry/domain authority accepted by one
// portable result codec. It deliberately contains content identities only;
// mutable registries and domains never cross the portable boundary.
type FormatSchema struct {
	RegistryID interproc.ContentID
	DomainID   interproc.ContentID
}

func NewFormatSchema(registryID, domainID interproc.ContentID) (FormatSchema, error) {
	schema := FormatSchema{RegistryID: registryID, DomainID: domainID}
	if !schema.Valid() {
		return FormatSchema{}, fmt.Errorf("summaryinstance: invalid registry/domain schema")
	}
	return schema, nil
}

func (s FormatSchema) Valid() bool { return s.RegistryID.Valid() && s.DomainID.Valid() }

// ID is the format authority embedded in each portable outcome.
func (s FormatSchema) ID() interproc.ContentID {
	if !s.Valid() {
		return interproc.ContentID{}
	}
	encoded := make([]byte, 0, 96)
	encoded = appendLengthDelimited(encoded, []byte("summaryinstance-format-schema/content-v1"))
	encoded = append(encoded, s.RegistryID[:]...)
	encoded = append(encoded, s.DomainID[:]...)
	return interproc.ContentIDFromCanonicalBytes(encoded)
}

// Fact is one closed value or outcome fact. Its bytes are source-owned
// semantic data; this codec neither interprets nor rebinds them.
type Fact struct {
	Key   string
	Value []byte
}

// AllocationTransport transports structural allocation template identities.
// It intentionally admits content IDs only, never a process-local allocation
// index or arena identity. Local rekeying happens after retrieval.
type AllocationTransport struct {
	TemplateID interproc.ContentID
	ResultID   interproc.ContentID
}

// ResidualDecision is a portable application decision. In particular there is
// no "possibly feasible" positive state: only ResidualFailing carries a
// positive proof and can later become caller publication.
type ResidualDecision uint8

const (
	ResidualSatisfied ResidualDecision = iota + 1
	ResidualInfeasible
	ResidualUndetermined
	ResidualFailing
)

// ApplicationResidual is proof material owned by a demanded body. BoundaryID
// identifies formal boundary information; it is not a caller binding, span,
// rendered message, callback, or mutable provider handle.
type ApplicationResidual struct {
	DescriptorID interproc.ContentID
	PredicateID  interproc.ContentID
	EvidenceID   interproc.ContentID
	GuardID      interproc.ContentID
	BoundaryID   interproc.ContentID
	Decision     ResidualDecision
	BoundStateID interproc.ContentID // required only for ResidualFailing
}

// InstanceKey is the portable identity of a callee instance. Retaining exact
// projection bytes alongside ProjectionID lets hash-table users confirm a hash
// collision without retaining a caller entry binding.
type InstanceKey struct {
	DemandedArtifactID      interproc.ContentID
	InstanceProjectionBytes []byte
	InstanceProjectionID    interproc.ContentID
}

// PortableClosedOutcome contains only closed semantic data. It is intentionally
// not an equation State, an arena handle, or a caller application context.
type PortableClosedOutcome struct {
	FormatSchemaID          interproc.ContentID
	DemandedArtifactID      interproc.ContentID
	InstanceProjectionBytes []byte
	InstanceProjectionID    interproc.ContentID
	Values                  []Fact
	Outcomes                []Fact
	AllocationTransport     []AllocationTransport
	ApplicationResiduals    []ApplicationResidual
	CalleeInstanceKeys      []InstanceKey
	DependencyIDs           []interproc.ContentID
	ResultDigest            interproc.ContentID
}

// CanonicalArtifact is an ownership-isolated portable outcome byte stream.
type CanonicalArtifact struct {
	Bytes    []byte
	Schema   interproc.ContentID
	Semantic interproc.ContentID
}

func (a CanonicalArtifact) Valid() bool {
	return len(a.Bytes) != 0 && a.Schema.Valid() && a.Semantic == interproc.ContentIDFromCanonicalBytes(a.Bytes)
}

// ResultDigestFor calculates the exact semantic-result digest required in an
// outcome. The input is normalized first, so this helper cannot bless an
// alternate ordering or duplicate spelling.
func ResultDigestFor(in PortableClosedOutcome) (interproc.ContentID, error) {
	normalized, err := normalize(in, false)
	if err != nil {
		return interproc.ContentID{}, err
	}
	bytes, err := encodeResult(context.Background(), normalized)
	if err != nil {
		return interproc.ContentID{}, err
	}
	return interproc.ContentIDFromCanonicalBytes(bytes), nil
}

// Encode serializes one complete portable outcome under exactly schema. All
// semantic lists are normalized and the caller-supplied digest must equal the
// resulting semantic closure digest.
func Encode(ctx context.Context, schema FormatSchema, in PortableClosedOutcome) (CanonicalArtifact, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return CanonicalArtifact{}, err
	}
	if !schema.Valid() || in.FormatSchemaID != schema.ID() {
		return CanonicalArtifact{}, fmt.Errorf("summaryinstance: foreign format schema")
	}
	normalized, err := normalize(in, true)
	if err != nil {
		return CanonicalArtifact{}, err
	}
	digest, err := ResultDigestFor(normalized)
	if err != nil {
		return CanonicalArtifact{}, err
	}
	if normalized.ResultDigest != digest {
		return CanonicalArtifact{}, fmt.Errorf("summaryinstance: result digest does not match semantic closure")
	}
	encoded, err := encodeOutcome(ctx, schema, normalized)
	if err != nil {
		return CanonicalArtifact{}, err
	}
	return CanonicalArtifact{Bytes: encoded, Schema: schema.ID(), Semantic: interproc.ContentIDFromCanonicalBytes(encoded)}, nil
}

// Seal encodes and immediately decodes the result, proving both canonical
// transport and registry/domain compatibility before publication.
func Seal(ctx context.Context, schema FormatSchema, in PortableClosedOutcome) (CanonicalArtifact, error) {
	artifact, err := Encode(ctx, schema, in)
	if err != nil {
		return CanonicalArtifact{}, err
	}
	if _, err := Decode(ctx, schema, artifact); err != nil {
		return CanonicalArtifact{}, err
	}
	return artifact, nil
}

// Decode rejects foreign schemas, malformed IDs, ordering variants, trailing
// bytes, and any stream that does not reproduce byte-for-byte on re-encoding.
func Decode(ctx context.Context, schema FormatSchema, artifact CanonicalArtifact) (PortableClosedOutcome, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return PortableClosedOutcome{}, err
	}
	if !schema.Valid() || !artifact.Valid() || artifact.Schema != schema.ID() {
		return PortableClosedOutcome{}, fmt.Errorf("summaryinstance: invalid or foreign outcome artifact")
	}
	var reader canonical.Reader
	if err := reader.Reset(ctx, artifact.Bytes, codecDomain, codecVersion); err != nil {
		return PortableClosedOutcome{}, err
	}
	record, err := reader.Record()
	if err != nil || record != outerRecord {
		return PortableClosedOutcome{}, decodeError("outcome record", err)
	}
	rawSchema, err := readID(&reader)
	if err != nil || rawSchema != schema.ID() {
		return PortableClosedOutcome{}, decodeError("format schema", err)
	}
	out := PortableClosedOutcome{FormatSchemaID: rawSchema}
	if out.DemandedArtifactID, err = readID(&reader); err != nil {
		return PortableClosedOutcome{}, decodeError("demanded artifact", err)
	}
	if out.InstanceProjectionBytes, err = reader.Bytes(); err != nil {
		return PortableClosedOutcome{}, err
	}
	if out.InstanceProjectionID, err = readID(&reader); err != nil {
		return PortableClosedOutcome{}, decodeError("instance projection", err)
	}
	if out.Values, err = decodeFacts(&reader, valuesRecord); err != nil {
		return PortableClosedOutcome{}, err
	}
	if out.Outcomes, err = decodeFacts(&reader, outcomesRecord); err != nil {
		return PortableClosedOutcome{}, err
	}
	if out.AllocationTransport, err = decodeAllocations(&reader); err != nil {
		return PortableClosedOutcome{}, err
	}
	if out.ApplicationResiduals, err = decodeResiduals(&reader); err != nil {
		return PortableClosedOutcome{}, err
	}
	if out.CalleeInstanceKeys, err = decodeCallees(&reader); err != nil {
		return PortableClosedOutcome{}, err
	}
	if out.DependencyIDs, err = decodeDependencies(&reader); err != nil {
		return PortableClosedOutcome{}, err
	}
	record, err = reader.Record()
	if err != nil || record != resultDigestRecord {
		return PortableClosedOutcome{}, decodeError("result digest record", err)
	}
	if out.ResultDigest, err = readID(&reader); err != nil {
		return PortableClosedOutcome{}, decodeError("result digest", err)
	}
	if err := reader.Finish(); err != nil {
		return PortableClosedOutcome{}, err
	}
	normalized, err := normalize(out, true)
	if err != nil {
		return PortableClosedOutcome{}, err
	}
	digest, err := ResultDigestFor(normalized)
	if err != nil || digest != normalized.ResultDigest {
		return PortableClosedOutcome{}, fmt.Errorf("summaryinstance: decoded result digest does not match semantic closure")
	}
	encoded, err := encodeOutcome(ctx, schema, normalized)
	if err != nil {
		return PortableClosedOutcome{}, err
	}
	if !bytes.Equal(encoded, artifact.Bytes) {
		return PortableClosedOutcome{}, fmt.Errorf("summaryinstance: decoded outcome changed canonical bytes")
	}
	return normalized, nil
}

func normalize(in PortableClosedOutcome, requireDigest bool) (PortableClosedOutcome, error) {
	if !in.FormatSchemaID.Valid() || !in.DemandedArtifactID.Valid() || !in.InstanceProjectionID.Valid() ||
		len(in.InstanceProjectionBytes) == 0 || interproc.ContentIDFromCanonicalBytes(in.InstanceProjectionBytes) != in.InstanceProjectionID {
		return PortableClosedOutcome{}, fmt.Errorf("summaryinstance: invalid outcome identity")
	}
	if err := interproc.ValidateEntryProjectionCanonicalBytes(in.InstanceProjectionBytes); err != nil {
		return PortableClosedOutcome{}, fmt.Errorf("summaryinstance: invalid instance projection: %w", err)
	}
	out := in
	out.InstanceProjectionBytes = append([]byte(nil), in.InstanceProjectionBytes...)
	var err error
	if out.Values, err = normalizeFacts(in.Values); err != nil {
		return PortableClosedOutcome{}, err
	}
	if out.Outcomes, err = normalizeFacts(in.Outcomes); err != nil {
		return PortableClosedOutcome{}, err
	}
	if out.AllocationTransport, err = normalizeAllocations(in.AllocationTransport); err != nil {
		return PortableClosedOutcome{}, err
	}
	if out.ApplicationResiduals, err = normalizeResiduals(in.ApplicationResiduals); err != nil {
		return PortableClosedOutcome{}, err
	}
	if out.CalleeInstanceKeys, err = normalizeCallees(in.CalleeInstanceKeys); err != nil {
		return PortableClosedOutcome{}, err
	}
	if out.DependencyIDs, err = normalizeIDs(in.DependencyIDs); err != nil {
		return PortableClosedOutcome{}, err
	}
	if requireDigest && !out.ResultDigest.Valid() {
		return PortableClosedOutcome{}, fmt.Errorf("summaryinstance: missing result digest")
	}
	return out, nil
}

func normalizeFacts(in []Fact) ([]Fact, error) {
	out := append([]Fact(nil), in...)
	for index := range out {
		if out[index].Key == "" {
			return nil, fmt.Errorf("summaryinstance: malformed semantic fact")
		}
		out[index].Value = append([]byte(nil), out[index].Value...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	unique := out[:0]
	for _, fact := range out {
		if len(unique) != 0 && unique[len(unique)-1].Key == fact.Key {
			if !bytes.Equal(unique[len(unique)-1].Value, fact.Value) {
				return nil, fmt.Errorf("summaryinstance: conflicting semantic fact %q", fact.Key)
			}
			continue
		}
		unique = append(unique, fact)
	}
	return append([]Fact(nil), unique...), nil
}

func normalizeAllocations(in []AllocationTransport) ([]AllocationTransport, error) {
	out := append([]AllocationTransport(nil), in...)
	for _, item := range out {
		if !item.TemplateID.Valid() || !item.ResultID.Valid() {
			return nil, fmt.Errorf("summaryinstance: malformed allocation transport")
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TemplateID != out[j].TemplateID {
			return string(out[i].TemplateID[:]) < string(out[j].TemplateID[:])
		}
		return string(out[i].ResultID[:]) < string(out[j].ResultID[:])
	})
	for index := 1; index < len(out); index++ {
		if out[index-1].TemplateID == out[index].TemplateID && out[index-1].ResultID != out[index].ResultID {
			return nil, fmt.Errorf("summaryinstance: conflicting allocation transport")
		}
	}
	unique := out[:0]
	for _, item := range out {
		if len(unique) != 0 && unique[len(unique)-1] == item {
			continue
		}
		unique = append(unique, item)
	}
	return append([]AllocationTransport(nil), unique...), nil
}

func normalizeResiduals(in []ApplicationResidual) ([]ApplicationResidual, error) {
	out := append([]ApplicationResidual(nil), in...)
	for _, item := range out {
		if !item.DescriptorID.Valid() || !item.PredicateID.Valid() || !item.EvidenceID.Valid() || !item.GuardID.Valid() || !item.BoundaryID.Valid() {
			return nil, fmt.Errorf("summaryinstance: malformed application residual")
		}
		switch item.Decision {
		case ResidualSatisfied, ResidualInfeasible, ResidualUndetermined:
			if item.BoundStateID.Valid() {
				return nil, fmt.Errorf("summaryinstance: non-positive residual carries feasibility proof")
			}
		case ResidualFailing:
			if !item.BoundStateID.Valid() {
				return nil, fmt.Errorf("summaryinstance: failing residual lacks positive feasibility proof")
			}
		default:
			return nil, fmt.Errorf("summaryinstance: unknown residual decision")
		}
	}
	sort.Slice(out, func(i, j int) bool { return residualLess(out[i], out[j]) })
	for index := 1; index < len(out); index++ {
		if residualEqual(out[index-1], out[index]) {
			return nil, fmt.Errorf("summaryinstance: duplicate application residual")
		}
	}
	return out, nil
}

func residualLess(left, right ApplicationResidual) bool {
	leftBytes, rightBytes := residualSortBytes(left), residualSortBytes(right)
	return bytes.Compare(leftBytes, rightBytes) < 0
}
func residualEqual(left, right ApplicationResidual) bool {
	return bytes.Equal(residualSortBytes(left), residualSortBytes(right))
}
func residualSortBytes(in ApplicationResidual) []byte {
	out := make([]byte, 0, 32*6+1)
	out = append(out, in.DescriptorID[:]...)
	out = append(out, in.PredicateID[:]...)
	out = append(out, in.EvidenceID[:]...)
	out = append(out, in.GuardID[:]...)
	out = append(out, in.BoundaryID[:]...)
	out = append(out, byte(in.Decision))
	out = append(out, in.BoundStateID[:]...)
	return out
}

func normalizeCallees(in []InstanceKey) ([]InstanceKey, error) {
	out := append([]InstanceKey(nil), in...)
	for index := range out {
		if !out[index].DemandedArtifactID.Valid() || !out[index].InstanceProjectionID.Valid() || len(out[index].InstanceProjectionBytes) == 0 ||
			interproc.ContentIDFromCanonicalBytes(out[index].InstanceProjectionBytes) != out[index].InstanceProjectionID ||
			interproc.ValidateEntryProjectionCanonicalBytes(out[index].InstanceProjectionBytes) != nil {
			return nil, fmt.Errorf("summaryinstance: malformed callee instance key")
		}
		out[index].InstanceProjectionBytes = append([]byte(nil), out[index].InstanceProjectionBytes...)
	}
	sort.Slice(out, func(i, j int) bool { return calleeLess(out[i], out[j]) })
	for index := 1; index < len(out); index++ {
		if !calleeLess(out[index-1], out[index]) {
			return nil, fmt.Errorf("summaryinstance: duplicate callee instance key")
		}
	}
	return out, nil
}
func calleeLess(left, right InstanceKey) bool {
	if left.DemandedArtifactID != right.DemandedArtifactID {
		return string(left.DemandedArtifactID[:]) < string(right.DemandedArtifactID[:])
	}
	return bytes.Compare(left.InstanceProjectionBytes, right.InstanceProjectionBytes) < 0
}

func normalizeIDs(in []interproc.ContentID) ([]interproc.ContentID, error) {
	out := append([]interproc.ContentID(nil), in...)
	for _, id := range out {
		if !id.Valid() {
			return nil, fmt.Errorf("summaryinstance: malformed dependency content ID")
		}
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i][:]) < string(out[j][:]) })
	for index := 1; index < len(out); index++ {
		if out[index-1] == out[index] {
			return nil, fmt.Errorf("summaryinstance: duplicate dependency content ID")
		}
	}
	return out, nil
}

func appendLengthDelimited(out, value []byte) []byte {
	length := uint64(len(value))
	return append(append(out, byte(length>>56), byte(length>>48), byte(length>>40), byte(length>>32), byte(length>>24), byte(length>>16), byte(length>>8), byte(length)), value...)
}
