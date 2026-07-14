package summary

import (
	"context"
	"crypto/sha256"
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
	"github.com/wippyai/go-lua/analysis/internal/registrycache"
)

const (
	canonicalSummaryDomain        = "analysis.fixpoint.summary.artifact"
	canonicalSummaryVersion       = 1
	canonicalSummarySchemaDomain  = "analysis.fixpoint.summary.artifact-schema"
	canonicalSummarySchemaVersion = 1

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
// subset. It currently admits Returns, NormalReturnParams,
// NormalReturnFacts.BranchProofs, and ReturnConditionParamRefinements. Every
// other populated lane fails closed before a writer session begins.
//
// The returned artifact is zero on cancellation, nonportable product values,
// registry mismatch, or any encoding failure.
func EncodeCanonical(ctx context.Context, reg *axis.Registry, in Summary) (CanonicalArtifact, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return CanonicalArtifact{}, err
	}
	if err := validateCanonicalLaneInventory(reg, in); err != nil {
		return CanonicalArtifact{}, err
	}

	normalized, err := NormalizeContext(ctx, reg, in)
	if err != nil {
		return CanonicalArtifact{}, err
	}
	if err := validateCanonicalLaneInventory(reg, normalized); err != nil {
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
	if err := encodeCanonicalBranchProofs(&writer, normalized.NormalReturnFacts.BranchProofs); err != nil {
		return CanonicalArtifact{}, err
	}
	if err := encodeCanonicalConditionParams(ctx, &writer, reg, productAuthority, normalized.ReturnConditionParamRefinements); err != nil {
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

func validateCanonicalLaneInventory(reg *axis.Registry, in Summary) error {
	for _, descriptor := range summaryFactDescriptors {
		if descriptor.Ops.empty(in) {
			continue
		}
		switch string(descriptor.Kind) {
		case "Returns", "NormalReturnParams", "NormalReturnFacts", "ReturnConditionParamRefinements":
		default:
			return &NonportableCanonicalError{Lane: string(descriptor.Kind), Reason: "outside evaluated-root subset"}
		}
	}
	for _, lane := range callboundary.NormalReturnFactLanes() {
		if lane.Len(in.NormalReturnFacts) == 0 || lane.ID() == callboundary.LaneBranchProofs {
			continue
		}
		return &NonportableCanonicalError{
			Lane: "NormalReturnFacts." + lane.FieldName(), Reason: "outside evaluated-root subset",
		}
	}
	if in.HeapKeySpace != nil {
		return &NonportableCanonicalError{Lane: "HeapKeySpace", Reason: "keyspace provenance is unsupported without the heap lane"}
	}
	if !canonicalProductsSafe(reg, in.Returns) {
		return &NonportableCanonicalError{Lane: "Returns", Reason: "registry mismatch or unsafe retained value"}
	}
	if !canonicalProductsSafe(reg, in.NormalReturnParams) {
		return &NonportableCanonicalError{Lane: "NormalReturnParams", Reason: "registry mismatch or unsafe retained value"}
	}
	for _, fact := range in.ReturnConditionParamRefinements {
		if !product.RetentionSafe(reg, fact.Value) {
			return &NonportableCanonicalError{
				Lane: "ReturnConditionParamRefinements", Reason: "registry mismatch or unsafe retained value",
			}
		}
	}
	return nil
}

func canonicalProductsSafe(reg *axis.Registry, values []product.Value) bool {
	for _, value := range values {
		if !product.RetentionSafe(reg, value) {
			return false
		}
	}
	return true
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
		"Returns", "NormalReturnParams", "NormalReturnFacts.BranchProofs", "ReturnConditionParamRefinements",
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
