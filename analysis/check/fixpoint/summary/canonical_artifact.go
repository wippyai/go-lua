package summary

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
	"github.com/wippyai/go-lua/analysis/internal/registrycache"
)

const (
	canonicalSummaryDomain        = "analysis.fixpoint.summary.artifact"
	canonicalSummaryVersion       = 2
	canonicalSummarySchemaDomain  = "analysis.fixpoint.summary.artifact-schema"
	canonicalSummarySchemaVersion = 2

	canonicalSummaryRecord         uint64 = 1
	canonicalReturnsRecord         uint64 = 2
	canonicalNormalParamsRecord    uint64 = 3
	canonicalBranchProofsRecord    uint64 = 4
	canonicalConditionParamsRecord uint64 = 5
	canonicalProductPayloadRecord  uint64 = 6
	canonicalBranchProofRecord     uint64 = 7
	canonicalConditionParamRecord  uint64 = 8
	canonicalPathRecord            uint64 = 9
	canonicalPathSegmentRecord     uint64 = 10
	canonicalSummarySchemaRecord   uint64 = 11
	canonicalReturnAliasesRecord   uint64 = 12
	canonicalReturnAliasRecord     uint64 = 13
	canonicalReturnFlowsRecord     uint64 = 14
	canonicalReturnFlowRecord      uint64 = 15
	canonicalPathRefinementsRecord uint64 = 16
	canonicalPathRefinementRecord  uint64 = 17
)

// CanonicalSchemaIdentity names the exact accepted summary vocabulary and the
// authoritative product-axis schema used by every embedded value.
type CanonicalSchemaIdentity [sha256.Size]byte

// CanonicalSemanticIdentity is the collision-resistant identity of the exact
// canonical summary bytes.
type CanonicalSemanticIdentity [sha256.Size]byte

// CanonicalArtifact is an ownership-isolated canonical summary payload.
// A zero artifact carries no authority.
type CanonicalArtifact struct {
	Bytes    []byte
	Schema   CanonicalSchemaIdentity
	Semantic CanonicalSemanticIdentity
}

type canonicalSummarySchemaInfo struct {
	product axis.SchemaIdentity
	summary CanonicalSchemaIdentity
	err     error
}

var canonicalSummarySchemas registrycache.Cache[canonicalSummarySchemaInfo]

// NonportableCanonicalError reports a populated summary lane outside the
// explicitly reviewed evaluated-root vocabulary. The lane is never omitted or
// approximated; callers may fall back to concrete solving.
type NonportableCanonicalError struct {
	Lane   string
	Reason string
}

func (e *NonportableCanonicalError) Error() string {
	if e == nil {
		return "summary: nonportable canonical payload"
	}
	if e.Reason == "" {
		return fmt.Sprintf("summary: lane %q is not portable in the canonical artifact", e.Lane)
	}
	return fmt.Sprintf("summary: lane %q is not portable in the canonical artifact: %s", e.Lane, e.Reason)
}

// EncodeCanonical encodes the exact artifact-safe evaluated-root summary
// subset. Every other populated lane fails closed before a writer session
// begins.
//
// The returned artifact is zero on cancellation, nonportable product values,
// registry mismatch, or any encoding failure.
func EncodeCanonical(ctx context.Context, reg *axis.Registry, in Summary) (CanonicalArtifact, error) {
	return encodeCanonical(ctx, reg, in, false)
}

// SealCanonical proves that every embedded product can be independently
// materialized, in addition to producing the canonical summary bytes. Unlike
// EncodeCanonical it may admit pointer-backed values whose canonical decoder
// reconstructs an owned graph; encode-only axis descriptors still fail.
func SealCanonical(ctx context.Context, reg *axis.Registry, in Summary) (CanonicalArtifact, error) {
	artifact, err := encodeCanonical(ctx, reg, in, true)
	if err != nil {
		return CanonicalArtifact{}, err
	}
	if _, err := DecodeCanonical(ctx, reg, artifact); err != nil {
		return CanonicalArtifact{}, err
	}
	return artifact, nil
}

func encodeCanonical(ctx context.Context, reg *axis.Registry, in Summary, materializable bool) (CanonicalArtifact, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return CanonicalArtifact{}, err
	}
	if err := validateCanonicalLaneInventory(ctx, reg, in, materializable); err != nil {
		return CanonicalArtifact{}, err
	}

	normalized, err := NormalizeContext(ctx, reg, in)
	if err != nil {
		return CanonicalArtifact{}, err
	}
	if err := validateCanonicalLaneInventory(ctx, reg, normalized, materializable); err != nil {
		return CanonicalArtifact{}, err
	}
	productAuthority, schema, err := canonicalSummarySchema(ctx, reg)
	if err != nil {
		return CanonicalArtifact{}, err
	}

	var writer canonical.Writer
	if err := writer.ResetBuffer(ctx, canonicalSummaryDomain, canonicalSummaryVersion); err != nil {
		return CanonicalArtifact{}, err
	}
	if err := writer.Record(canonicalSummaryRecord); err != nil {
		return CanonicalArtifact{}, err
	}
	if err := writer.Bytes(schema[:]); err != nil {
		return CanonicalArtifact{}, err
	}
	if err := encodeCanonicalProducts(ctx, &writer, reg, productAuthority, canonicalReturnsRecord, normalized.Returns); err != nil {
		return CanonicalArtifact{}, err
	}
	if err := encodeCanonicalProducts(ctx, &writer, reg, productAuthority, canonicalNormalParamsRecord, normalized.NormalReturnParams); err != nil {
		return CanonicalArtifact{}, err
	}
	if err := encodeCanonicalPathRefinements(ctx, &writer, reg, productAuthority, normalized.NormalReturnFacts.PathRefinements); err != nil {
		return CanonicalArtifact{}, err
	}
	if err := encodeCanonicalBranchProofs(&writer, normalized.NormalReturnFacts.BranchProofs); err != nil {
		return CanonicalArtifact{}, err
	}
	if err := encodeCanonicalConditionParams(ctx, &writer, reg, productAuthority, normalized.ReturnConditionParamRefinements); err != nil {
		return CanonicalArtifact{}, err
	}
	if err := encodeCanonicalReturnAliases(&writer, normalized.ReturnParamPathAliases); err != nil {
		return CanonicalArtifact{}, err
	}
	if err := encodeCanonicalReturnFlows(&writer, normalized.ReturnFlows); err != nil {
		return CanonicalArtifact{}, err
	}
	encoded, err := writer.FinishBytes()
	if err != nil {
		return CanonicalArtifact{}, err
	}
	return CanonicalArtifact{
		Bytes: encoded, Schema: schema, Semantic: CanonicalSemanticIdentity(sha256.Sum256(encoded)),
	}, nil
}

func validateCanonicalLaneInventory(ctx context.Context, reg *axis.Registry, in Summary, materializable bool) error {
	for _, descriptor := range summaryFactDescriptors {
		if descriptor.Ops.empty(in) {
			continue
		}
		switch string(descriptor.Kind) {
		case "Returns", "NormalReturnParams", "NormalReturnFacts", "ReturnConditionParamRefinements", "ReturnParamPathAliases", "ReturnFlows":
		default:
			return &NonportableCanonicalError{Lane: string(descriptor.Kind), Reason: "outside evaluated-root subset"}
		}
	}
	for _, lane := range callboundary.NormalReturnFactLanes() {
		if lane.Len(in.NormalReturnFacts) == 0 || lane.ID() == callboundary.LaneBranchProofs || lane.ID() == callboundary.LanePathRefinements {
			continue
		}
		return &NonportableCanonicalError{
			Lane: "NormalReturnFacts." + lane.FieldName(), Reason: "outside evaluated-root subset",
		}
	}
	if in.HeapKeySpace != nil {
		return &NonportableCanonicalError{Lane: "HeapKeySpace", Reason: "keyspace provenance is unsupported without the heap lane"}
	}
	if err := validateCanonicalProducts(ctx, reg, in.Returns, materializable); err != nil {
		return canonicalProductLaneError("Returns", err)
	}
	if err := validateCanonicalProducts(ctx, reg, in.NormalReturnParams, materializable); err != nil {
		return canonicalProductLaneError("NormalReturnParams", err)
	}
	for _, fact := range in.ReturnConditionParamRefinements {
		if err := validateCanonicalProducts(ctx, reg, []product.Value{fact.Value}, materializable); err != nil {
			return canonicalProductLaneError("ReturnConditionParamRefinements", err)
		}
	}
	for _, fact := range in.NormalReturnFacts.PathRefinements {
		if !fact.Path.IsPlaceholder() {
			return &NonportableCanonicalError{Lane: "NormalReturnFacts.PathRefinements", Reason: "invalid placeholder path"}
		}
		if err := validateCanonicalProducts(ctx, reg, []product.Value{fact.Value}, materializable); err != nil {
			return canonicalProductLaneError("NormalReturnFacts.PathRefinements", err)
		}
	}
	for _, flow := range in.ReturnFlows {
		if _, ok := normalizeReturnFlow(flow, false); !ok {
			return &NonportableCanonicalError{Lane: "ReturnFlows", Reason: "invalid return-flow identity"}
		}
	}
	return nil
}

func validateCanonicalProducts(ctx context.Context, reg *axis.Registry, values []product.Value, materializable bool) error {
	for _, value := range values {
		if !materializable {
			if !product.RetentionSafe(reg, value) {
				return fmt.Errorf("registry mismatch or unsafe retained value")
			}
			continue
		}
		artifact, err := product.SealCanonical(ctx, reg, value)
		if err != nil {
			return err
		}
		if !artifact.Valid() {
			return fmt.Errorf("canonical materialization returned no artifact")
		}
	}
	return nil
}

func canonicalProductLaneError(lane string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return &NonportableCanonicalError{Lane: lane, Reason: err.Error()}
}

func canonicalSummarySchema(ctx context.Context, reg *axis.Registry) (axis.SchemaIdentity, CanonicalSchemaIdentity, error) {
	if err := ctx.Err(); err != nil {
		return axis.SchemaIdentity{}, CanonicalSchemaIdentity{}, err
	}
	var info canonicalSummarySchemaInfo
	if reg != nil && reg.Frozen() {
		info = canonicalSummarySchemas.GetFor(reg, buildCanonicalSummarySchema)
	} else {
		info = buildCanonicalSummarySchema(reg)
	}
	if err := ctx.Err(); err != nil {
		return axis.SchemaIdentity{}, CanonicalSchemaIdentity{}, err
	}
	return info.product, info.summary, info.err
}

func buildCanonicalSummarySchema(reg *axis.Registry) canonicalSummarySchemaInfo {
	plan, err := reg.CanonicalPlan()
	if err != nil {
		return canonicalSummarySchemaInfo{err: fmt.Errorf("summary: canonical registry plan: %w", err)}
	}
	productAuthority, ok := plan.AuthorityIdentity()
	if !ok {
		return canonicalSummarySchemaInfo{err: &NonportableCanonicalError{
			Lane: "product-schema", Reason: "canonical axis authority is unavailable",
		}}
	}
	var writer canonical.Writer
	if err := writer.ResetBuffer(context.Background(), canonicalSummarySchemaDomain, canonicalSummarySchemaVersion); err != nil {
		return canonicalSummarySchemaInfo{err: err}
	}
	if err := writer.Record(canonicalSummarySchemaRecord); err != nil {
		return canonicalSummarySchemaInfo{err: err}
	}
	if err := writer.Bytes(productAuthority[:]); err != nil {
		return canonicalSummarySchemaInfo{err: err}
	}
	if err := writer.Uint(BoundaryLaneSchemaVersion); err != nil {
		return canonicalSummarySchemaInfo{err: err}
	}
	accepted := []string{
		"Returns", "NormalReturnParams", "NormalReturnFacts.PathRefinements", "NormalReturnFacts.BranchProofs", "ReturnConditionParamRefinements", "ReturnParamPathAliases", "ReturnFlows",
	}
	if err := writer.Count(uint64(len(accepted))); err != nil {
		return canonicalSummarySchemaInfo{err: err}
	}
	for _, lane := range accepted {
		if err := writer.String(lane); err != nil {
			return canonicalSummarySchemaInfo{err: err}
		}
	}
	encoded, err := writer.FinishBytes()
	if err != nil {
		return canonicalSummarySchemaInfo{err: err}
	}
	return canonicalSummarySchemaInfo{
		product: productAuthority,
		summary: CanonicalSchemaIdentity(sha256.Sum256(encoded)),
	}
}

func encodeCanonicalReturnAliases(writer *canonical.Writer, aliases []ReturnParamPathAlias) error {
	if err := writer.Record(canonicalReturnAliasesRecord); err != nil {
		return err
	}
	if err := writer.Count(uint64(len(aliases))); err != nil {
		return err
	}
	for _, alias := range aliases {
		if alias.ReturnIndex < 0 || !alias.Source.Valid() || alias.Member != "" && !alias.Member.Valid() {
			return &NonportableCanonicalError{Lane: "ReturnParamPathAliases", Reason: "invalid return, member, or source identity"}
		}
		if err := writer.Record(canonicalReturnAliasRecord); err != nil {
			return err
		}
		if err := writer.Int(int64(alias.ReturnIndex)); err != nil {
			return err
		}
		memberSegments := []segment.Segment(nil)
		if alias.Member != "" {
			var ok bool
			memberSegments, ok = pathaddr.RelativeStaticMemberSuffixSegments(alias.Member)
			if !ok {
				return &NonportableCanonicalError{Lane: "ReturnParamPathAliases", Reason: "invalid member identity"}
			}
		}
		if err := encodeCanonicalSegments(writer, memberSegments); err != nil {
			return err
		}
		source, ok := alias.Source.Path()
		if !ok {
			return &NonportableCanonicalError{Lane: "ReturnParamPathAliases", Reason: "invalid source identity"}
		}
		if err := encodeCanonicalPath(writer, source); err != nil {
			return err
		}
	}
	return nil
}

func encodeCanonicalReturnFlows(writer *canonical.Writer, flows []ReturnFlow) error {
	if err := writer.Record(canonicalReturnFlowsRecord); err != nil {
		return err
	}
	if err := writer.Count(uint64(len(flows))); err != nil {
		return err
	}
	for _, flow := range flows {
		normalized, ok := normalizeReturnFlow(flow, false)
		if !ok {
			return &NonportableCanonicalError{Lane: "ReturnFlows", Reason: "invalid return-flow identity"}
		}
		if err := writer.Record(canonicalReturnFlowRecord); err != nil {
			return err
		}
		if err := writer.Int(int64(normalized.ReturnIndex)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(normalized.Kind)); err != nil {
			return err
		}
		if err := writer.Int(int64(normalized.Param)); err != nil {
			return err
		}
		if err := encodeCanonicalSegments(writer, normalized.Path); err != nil {
			return err
		}
	}
	return nil
}

func encodeCanonicalSegments(writer *canonical.Writer, segments []segment.Segment) error {
	if err := writer.Count(uint64(len(segments))); err != nil {
		return err
	}
	for _, item := range segments {
		if err := writer.Record(canonicalPathSegmentRecord); err != nil {
			return err
		}
		if err := writer.Uint(uint64(item.Kind)); err != nil {
			return err
		}
		if err := writer.String(item.Name); err != nil {
			return err
		}
		if err := writer.Int(int64(item.Index)); err != nil {
			return err
		}
	}
	return nil
}

func encodeCanonicalProducts(
	ctx context.Context,
	writer *canonical.Writer,
	reg *axis.Registry,
	authority axis.SchemaIdentity,
	record uint64,
	values []product.Value,
) error {
	if err := writer.Record(record); err != nil {
		return err
	}
	if err := writer.Count(uint64(len(values))); err != nil {
		return err
	}
	for _, value := range values {
		encoded, schema, err := product.EncodeCanonical(ctx, reg, value)
		if err != nil {
			return err
		}
		if schema != authority {
			return fmt.Errorf("summary: product codec returned mismatched schema authority")
		}
		if err := writer.Record(canonicalProductPayloadRecord); err != nil {
			return err
		}
		if err := writer.Bytes(encoded); err != nil {
			return err
		}
	}
	return nil
}

func encodeCanonicalPathRefinements(
	ctx context.Context,
	writer *canonical.Writer,
	reg *axis.Registry,
	authority axis.SchemaIdentity,
	facts []callboundary.PathValueFact,
) error {
	if err := writer.Record(canonicalPathRefinementsRecord); err != nil {
		return err
	}
	if err := writer.Count(uint64(len(facts))); err != nil {
		return err
	}
	for _, fact := range facts {
		if !fact.Path.IsPlaceholder() || !product.BelongsToRegistry(reg, fact.Value) {
			return &NonportableCanonicalError{Lane: "NormalReturnFacts.PathRefinements", Reason: "invalid path or unsafe retained value"}
		}
		if err := writer.Record(canonicalPathRefinementRecord); err != nil {
			return err
		}
		if err := encodeCanonicalPath(writer, fact.Path); err != nil {
			return err
		}
		encoded, schema, err := product.EncodeCanonical(ctx, reg, fact.Value)
		if err != nil {
			return err
		}
		if schema != authority {
			return fmt.Errorf("summary: product codec returned mismatched schema authority")
		}
		if err := writer.Record(canonicalProductPayloadRecord); err != nil {
			return err
		}
		if err := writer.Bytes(encoded); err != nil {
			return err
		}
	}
	return nil
}

func encodeCanonicalBranchProofs(writer *canonical.Writer, proofs []callboundary.BranchProof) error {
	if err := writer.Record(canonicalBranchProofsRecord); err != nil {
		return err
	}
	if err := writer.Count(uint64(len(proofs))); err != nil {
		return err
	}
	for _, proof := range proofs {
		if err := writer.Record(canonicalBranchProofRecord); err != nil {
			return err
		}
		if err := writer.Uint(uint64(proof.Kind)); err != nil {
			return err
		}
		if err := encodeCanonicalPath(writer, proof.Path); err != nil {
			return err
		}
		if err := writer.Uint(uint64(proof.Presence)); err != nil {
			return err
		}
		if err := encodeCanonicalPath(writer, proof.Other); err != nil {
			return err
		}
	}
	return nil
}

func encodeCanonicalConditionParams(
	ctx context.Context,
	writer *canonical.Writer,
	reg *axis.Registry,
	authority axis.SchemaIdentity,
	facts []ReturnConditionParamRefinement,
) error {
	if err := writer.Record(canonicalConditionParamsRecord); err != nil {
		return err
	}
	if err := writer.Count(uint64(len(facts))); err != nil {
		return err
	}
	for _, fact := range facts {
		if err := writer.Record(canonicalConditionParamRecord); err != nil {
			return err
		}
		if err := writer.Int(int64(fact.ReturnIndex)); err != nil {
			return err
		}
		if err := writer.Bool(fact.ReturnValue); err != nil {
			return err
		}
		if err := encodeCanonicalPath(writer, fact.Target); err != nil {
			return err
		}
		encoded, schema, err := product.EncodeCanonical(ctx, reg, fact.Value)
		if err != nil {
			return err
		}
		if schema != authority {
			return fmt.Errorf("summary: product codec returned mismatched schema authority")
		}
		if err := writer.Record(canonicalProductPayloadRecord); err != nil {
			return err
		}
		if err := writer.Bytes(encoded); err != nil {
			return err
		}
	}
	return nil
}

func encodeCanonicalPath(writer *canonical.Writer, path pathdom.Path) error {
	if err := writer.Record(canonicalPathRecord); err != nil {
		return err
	}
	hasSymbol := path.Symbol != 0
	if err := writer.Bool(hasSymbol); err != nil {
		return err
	}
	if hasSymbol {
		if err := writer.Uint(uint64(path.Symbol)); err != nil {
			return err
		}
		if err := writer.Int(int64(path.Version)); err != nil {
			return err
		}
	} else if err := writer.String(path.Root); err != nil {
		return err
	}
	if err := writer.Count(uint64(len(path.Segments))); err != nil {
		return err
	}
	for _, segment := range path.Segments {
		if err := writer.Record(canonicalPathSegmentRecord); err != nil {
			return err
		}
		if err := writer.Uint(uint64(segment.Kind)); err != nil {
			return err
		}
		if err := writer.String(segment.Name); err != nil {
			return err
		}
		if err := writer.Int(int64(segment.Index)); err != nil {
			return err
		}
	}
	return nil
}
