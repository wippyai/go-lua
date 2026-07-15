package summary

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// DecodeCanonical materializes the exact evaluated-root summary subset from a
// canonical artifact. Publication is fenced by a full decode/encode byte
// roundtrip, so malformed, foreign-schema, or merely equivalent alternate
// spellings never cross the boundary.
func DecodeCanonical(ctx context.Context, reg *axis.Registry, artifact CanonicalArtifact) (Summary, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Summary{}, err
	}
	productSchema, schema, err := canonicalSummarySchema(ctx, reg)
	if err != nil {
		return Summary{}, err
	}
	if len(artifact.Bytes) == 0 || artifact.Schema == (CanonicalSchemaIdentity{}) ||
		artifact.Semantic == (CanonicalSemanticIdentity{}) || artifact.Schema != schema ||
		artifact.Semantic != CanonicalSemanticIdentity(sha256.Sum256(artifact.Bytes)) {
		return Summary{}, fmt.Errorf("summary: invalid or foreign canonical artifact authority")
	}

	var reader canonical.Reader
	if err := reader.Reset(ctx, artifact.Bytes, canonicalSummaryDomain, canonicalSummaryVersion); err != nil {
		return Summary{}, err
	}
	record, err := reader.Record()
	if err != nil || record != canonicalSummaryRecord {
		return Summary{}, canonicalSummaryDecodeError("summary record", err)
	}
	rawSchema, err := reader.Bytes()
	if err != nil || !bytes.Equal(rawSchema, schema[:]) {
		return Summary{}, canonicalSummaryDecodeError("summary schema", err)
	}

	var out Summary
	if out.Returns, err = decodeCanonicalProducts(ctx, &reader, reg, productSchema, canonicalReturnsRecord); err != nil {
		return Summary{}, err
	}
	if out.NormalReturnParams, err = decodeCanonicalProducts(ctx, &reader, reg, productSchema, canonicalNormalParamsRecord); err != nil {
		return Summary{}, err
	}
	if out.NormalReturnFacts.PathRefinements, err = decodeCanonicalPathRefinements(ctx, &reader, reg, productSchema); err != nil {
		return Summary{}, err
	}
	if out.NormalReturnFacts.BranchProofs, err = decodeCanonicalBranchProofs(&reader); err != nil {
		return Summary{}, err
	}
	if out.ReturnConditionParamRefinements, err = decodeCanonicalConditionParams(ctx, &reader, reg, productSchema); err != nil {
		return Summary{}, err
	}
	if out.ReturnParamPathAliases, err = decodeCanonicalReturnAliases(&reader); err != nil {
		return Summary{}, err
	}
	if out.ReturnFlows, err = decodeCanonicalReturnFlows(&reader); err != nil {
		return Summary{}, err
	}
	if err := reader.Finish(); err != nil {
		return Summary{}, err
	}
	if err := validateCanonicalDecodedSummary(reg, out); err != nil {
		return Summary{}, err
	}

	roundTrip, err := encodeCanonical(ctx, reg, out, true)
	if err != nil {
		return Summary{}, err
	}
	if roundTrip.Schema != artifact.Schema || roundTrip.Semantic != artifact.Semantic || !bytes.Equal(roundTrip.Bytes, artifact.Bytes) {
		return Summary{}, fmt.Errorf("summary: reconstructed artifact changed canonical bytes")
	}
	if err := ctx.Err(); err != nil {
		return Summary{}, err
	}
	return out, nil
}

func validateCanonicalDecodedSummary(reg *axis.Registry, in Summary) error {
	for _, fact := range in.NormalReturnFacts.PathRefinements {
		if !fact.Path.IsPlaceholder() {
			return canonicalSummaryDecodeError("non-placeholder path refinement", nil)
		}
		if err := validateCanonicalPathShape(fact.Path); err != nil {
			return err
		}
	}
	for _, proof := range in.NormalReturnFacts.BranchProofs {
		admitted, ok := admitBranchProof(proof)
		if !ok || admitted.Kind != proof.Kind || admitted.Presence != proof.Presence ||
			admitted.Path.Key() != proof.Path.Key() || admitted.Other.Key() != proof.Other.Key() {
			return canonicalSummaryDecodeError("branch proof enum or payload", nil)
		}
		if proof.Kind == pathevidence.BranchProofPathPresence && proof.Presence != presence.Present() && proof.Presence != presence.Absent() {
			return canonicalSummaryDecodeError("branch proof presence enum", nil)
		}
		if err := validateCanonicalPathShape(proof.Path); err != nil {
			return err
		}
		if proof.Kind != pathevidence.BranchProofPathPresence {
			if err := validateCanonicalPathShape(proof.Other); err != nil {
				return err
			}
		}
	}
	for _, fact := range in.ReturnConditionParamRefinements {
		if fact.ReturnIndex < 0 || !fact.Target.IsPlaceholder() || !usefulReturnConditionValue(reg, fact.Value) {
			return canonicalSummaryDecodeError("return condition payload", nil)
		}
		if err := validateCanonicalPathShape(fact.Target); err != nil {
			return err
		}
	}
	for _, alias := range in.ReturnParamPathAliases {
		if alias.ReturnIndex < 0 || !alias.Source.Valid() || alias.Member != "" && !alias.Member.Valid() {
			return canonicalSummaryDecodeError("return alias payload", nil)
		}
	}
	for _, flow := range in.ReturnFlows {
		normalized, ok := normalizeReturnFlow(flow, false)
		if !ok || normalized.ReturnIndex != flow.ReturnIndex || normalized.Kind != flow.Kind || normalized.Param != flow.Param ||
			!canonicalSegmentsEqual(normalized.Path, flow.Path) {
			return canonicalSummaryDecodeError("return flow enum or payload", nil)
		}
	}
	return nil
}

func validateCanonicalPathShape(path pathdom.Path) error {
	if path.Symbol == 0 {
		if path.Root == "" || path.Version != 0 {
			return canonicalSummaryDecodeError("root path identity", nil)
		}
	} else if path.Root != "" {
		return canonicalSummaryDecodeError("symbol path identity", nil)
	}
	for _, item := range path.Segments {
		if !validCanonicalSegment(item) {
			return canonicalSummaryDecodeError("path segment enum or payload", nil)
		}
	}
	return nil
}

func validCanonicalSegment(item segment.Segment) bool {
	switch item.Kind {
	case segment.SegmentField:
		return item.Name != "" && item.Index == 0
	case segment.SegmentIndexString:
		return item.Index == 0
	case segment.SegmentIndexInt:
		return item.Name == ""
	default:
		return false
	}
}

func canonicalSegmentsEqual(left, right []segment.Segment) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] || !validCanonicalSegment(left[index]) {
			return false
		}
	}
	return true
}

func decodeCanonicalProducts(ctx context.Context, reader *canonical.Reader, reg *axis.Registry, schema axis.SchemaIdentity, wantRecord uint64) ([]product.Value, error) {
	record, err := reader.Record()
	if err != nil || record != wantRecord {
		return nil, canonicalSummaryDecodeError("product list record", err)
	}
	count, err := canonicalDecodeCount(reader)
	if err != nil {
		return nil, err
	}
	out := make([]product.Value, count)
	for index := range out {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		record, err = reader.Record()
		if err != nil || record != canonicalProductPayloadRecord {
			return nil, canonicalSummaryDecodeError("product payload record", err)
		}
		encoded, err := reader.Bytes()
		if err != nil {
			return nil, err
		}
		out[index], err = product.DecodeCanonical(ctx, reg, encoded, schema)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func decodeCanonicalPathRefinements(ctx context.Context, reader *canonical.Reader, reg *axis.Registry, schema axis.SchemaIdentity) ([]callboundary.PathValueFact, error) {
	record, err := reader.Record()
	if err != nil || record != canonicalPathRefinementsRecord {
		return nil, canonicalSummaryDecodeError("path refinements record", err)
	}
	count, err := canonicalDecodeCount(reader)
	if err != nil {
		return nil, err
	}
	out := make([]callboundary.PathValueFact, count)
	for index := range out {
		record, err = reader.Record()
		if err != nil || record != canonicalPathRefinementRecord {
			return nil, canonicalSummaryDecodeError("path refinement record", err)
		}
		if out[index].Path, err = decodeCanonicalPath(reader); err != nil {
			return nil, err
		}
		record, err = reader.Record()
		if err != nil || record != canonicalProductPayloadRecord {
			return nil, canonicalSummaryDecodeError("path refinement product record", err)
		}
		encoded, err := reader.Bytes()
		if err != nil {
			return nil, err
		}
		if out[index].Value, err = product.DecodeCanonical(ctx, reg, encoded, schema); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func decodeCanonicalBranchProofs(reader *canonical.Reader) ([]callboundary.BranchProof, error) {
	record, err := reader.Record()
	if err != nil || record != canonicalBranchProofsRecord {
		return nil, canonicalSummaryDecodeError("branch proofs record", err)
	}
	count, err := canonicalDecodeCount(reader)
	if err != nil {
		return nil, err
	}
	out := make([]callboundary.BranchProof, count)
	for index := range out {
		record, err = reader.Record()
		if err != nil || record != canonicalBranchProofRecord {
			return nil, canonicalSummaryDecodeError("branch proof record", err)
		}
		kind, err := reader.Uint()
		if err != nil {
			return nil, err
		}
		out[index].Kind = pathevidence.BranchProofKind(kind)
		if out[index].Path, err = decodeCanonicalPath(reader); err != nil {
			return nil, err
		}
		rawPresence, err := reader.Uint()
		if err != nil {
			return nil, err
		}
		out[index].Presence = presence.Value(rawPresence)
		if out[index].Other, err = decodeCanonicalPath(reader); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func decodeCanonicalConditionParams(ctx context.Context, reader *canonical.Reader, reg *axis.Registry, schema axis.SchemaIdentity) ([]ReturnConditionParamRefinement, error) {
	record, err := reader.Record()
	if err != nil || record != canonicalConditionParamsRecord {
		return nil, canonicalSummaryDecodeError("condition params record", err)
	}
	count, err := canonicalDecodeCount(reader)
	if err != nil {
		return nil, err
	}
	out := make([]ReturnConditionParamRefinement, count)
	for index := range out {
		record, err = reader.Record()
		if err != nil || record != canonicalConditionParamRecord {
			return nil, canonicalSummaryDecodeError("condition param record", err)
		}
		rawIndex, err := reader.Int()
		if err != nil {
			return nil, err
		}
		out[index].ReturnIndex = int(rawIndex)
		if out[index].ReturnValue, err = reader.Bool(); err != nil {
			return nil, err
		}
		if out[index].Target, err = decodeCanonicalPath(reader); err != nil {
			return nil, err
		}
		record, err = reader.Record()
		if err != nil || record != canonicalProductPayloadRecord {
			return nil, canonicalSummaryDecodeError("condition param product record", err)
		}
		encoded, err := reader.Bytes()
		if err != nil {
			return nil, err
		}
		if out[index].Value, err = product.DecodeCanonical(ctx, reg, encoded, schema); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func decodeCanonicalReturnAliases(reader *canonical.Reader) ([]ReturnParamPathAlias, error) {
	record, err := reader.Record()
	if err != nil || record != canonicalReturnAliasesRecord {
		return nil, canonicalSummaryDecodeError("return aliases record", err)
	}
	count, err := canonicalDecodeCount(reader)
	if err != nil {
		return nil, err
	}
	out := make([]ReturnParamPathAlias, count)
	for index := range out {
		record, err = reader.Record()
		if err != nil || record != canonicalReturnAliasRecord {
			return nil, canonicalSummaryDecodeError("return alias record", err)
		}
		rawIndex, err := reader.Int()
		if err != nil {
			return nil, err
		}
		out[index].ReturnIndex = int(rawIndex)
		segments, err := decodeCanonicalSegments(reader)
		if err != nil {
			return nil, err
		}
		if len(segments) != 0 {
			out[index].Member, _ = pathaddr.RelativeStaticMemberSuffixKey(segments)
		}
		source, err := decodeCanonicalPath(reader)
		if err != nil {
			return nil, err
		}
		out[index].Source, _ = pathaddr.PlaceholderKeyFromPath(source)
	}
	return out, nil
}

func decodeCanonicalReturnFlows(reader *canonical.Reader) ([]ReturnFlow, error) {
	record, err := reader.Record()
	if err != nil || record != canonicalReturnFlowsRecord {
		return nil, canonicalSummaryDecodeError("return flows record", err)
	}
	count, err := canonicalDecodeCount(reader)
	if err != nil {
		return nil, err
	}
	out := make([]ReturnFlow, count)
	for index := range out {
		record, err = reader.Record()
		if err != nil || record != canonicalReturnFlowRecord {
			return nil, canonicalSummaryDecodeError("return flow record", err)
		}
		rawReturn, err := reader.Int()
		if err != nil {
			return nil, err
		}
		rawKind, err := reader.Uint()
		if err != nil {
			return nil, err
		}
		rawParam, err := reader.Int()
		if err != nil {
			return nil, err
		}
		out[index].ReturnIndex, out[index].Kind, out[index].Param = int(rawReturn), ReturnFlowKind(rawKind), int(rawParam)
		if out[index].Path, err = decodeCanonicalSegments(reader); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func decodeCanonicalPath(reader *canonical.Reader) (pathdom.Path, error) {
	record, err := reader.Record()
	if err != nil || record != canonicalPathRecord {
		return pathdom.Path{}, canonicalSummaryDecodeError("path record", err)
	}
	hasSymbol, err := reader.Bool()
	if err != nil {
		return pathdom.Path{}, err
	}
	var out pathdom.Path
	if hasSymbol {
		rawSymbol, err := reader.Uint()
		if err != nil {
			return pathdom.Path{}, err
		}
		rawVersion, err := reader.Int()
		if err != nil {
			return pathdom.Path{}, err
		}
		out.Symbol, out.Version = symbol.ID(rawSymbol), int(rawVersion)
	} else if out.Root, err = reader.String(); err != nil {
		return pathdom.Path{}, err
	}
	if out.Segments, err = decodeCanonicalSegments(reader); err != nil {
		return pathdom.Path{}, err
	}
	return out, nil
}

func decodeCanonicalSegments(reader *canonical.Reader) ([]segment.Segment, error) {
	count, err := canonicalDecodeCount(reader)
	if err != nil {
		return nil, err
	}
	out := make([]segment.Segment, count)
	for index := range out {
		record, err := reader.Record()
		if err != nil || record != canonicalPathSegmentRecord {
			return nil, canonicalSummaryDecodeError("path segment record", err)
		}
		rawKind, err := reader.Uint()
		if err != nil {
			return nil, err
		}
		out[index].Kind = segment.SegmentKind(rawKind)
		if out[index].Name, err = reader.String(); err != nil {
			return nil, err
		}
		rawIndex, err := reader.Int()
		if err != nil {
			return nil, err
		}
		out[index].Index = int(rawIndex)
	}
	return out, nil
}

func canonicalDecodeCount(reader *canonical.Reader) (int, error) {
	count, err := reader.Count()
	if err != nil {
		return 0, err
	}
	// Every list member has at least one unread event. This is an input-size
	// proof against impossible allocation, not a semantic or work budget.
	if count > uint64(reader.RemainingBytes()) || count > uint64(^uint(0)>>1) {
		return 0, canonicalSummaryDecodeError("impossible list count", nil)
	}
	return int(count), nil
}

func canonicalSummaryDecodeError(field string, err error) error {
	if err != nil {
		return fmt.Errorf("summary: malformed canonical %s: %w", field, err)
	}
	return fmt.Errorf("summary: malformed canonical %s", field)
}
