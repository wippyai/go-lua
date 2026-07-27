package summary

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

func TestDecodeCanonicalSummaryExactRoundTripIncludingPointerWitness(t *testing.T) {
	reg := standard.Registry()
	literal := typevalue.LiteralBool(reg, true)
	if product.RetentionSafe(reg, literal) {
		t.Fatal("literal fixture unexpectedly passed ordinary retention")
	}
	in := Summary{
		Returns:            []product.Value{literal, typevalue.String(reg)},
		NormalReturnParams: []product.Value{typevalue.String(reg)},
		NormalReturnFacts: callboundary.NormalReturnFacts{
			PathRefinements: []callboundary.PathValueFact{{Path: pathdom.NewPlaceholder(0).Field("name"), Value: typevalue.String(reg)}},
			BranchProofs: []callboundary.BranchProof{{
				Kind: pathevidence.BranchProofPathPresence, Path: pathdom.NewPlaceholder(0), Presence: presence.Present(),
			}},
		},
		ReturnConditionParamRefinements: []ReturnConditionParamRefinement{{
			ReturnIndex: 0, ReturnValue: true, Target: pathdom.NewPlaceholder(0), Value: typevalue.String(reg),
		}},
		ReturnFlows: []ReturnFlow{{
			ReturnIndex: 1, Kind: ReturnFlowParamMember, Param: 0, Path: []segment.Segment{{Kind: segment.SegmentField, Name: "name"}},
		}},
	}
	artifact, err := SealCanonical(context.Background(), reg, in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := DecodeCanonical(context.Background(), reg, artifact)
	if err != nil {
		t.Fatal(err)
	}
	if !Equal(reg, in, out) {
		t.Fatalf("decoded summary changed semantics\nin:  %#v\nout: %#v", in, out)
	}
	again, err := SealCanonical(context.Background(), reg, out)
	if err != nil {
		t.Fatal(err)
	}
	if again.Schema != artifact.Schema || again.Semantic != artifact.Semantic || !bytes.Equal(again.Bytes, artifact.Bytes) {
		t.Fatal("decoded summary changed canonical authority")
	}
}

func TestDecodeCanonicalSummaryRejectsMalformedSchemaAndCancellation(t *testing.T) {
	reg := standard.Registry()
	artifact, err := EncodeCanonical(context.Background(), reg, Summary{Returns: []product.Value{typevalue.String(reg)}})
	if err != nil {
		t.Fatal(err)
	}
	badSchema := artifact
	badSchema.Schema[0] ^= 0xff
	if got, err := DecodeCanonical(context.Background(), reg, badSchema); err == nil || !Equal(reg, got, Summary{}) {
		t.Fatalf("foreign schema decoded %#v, %v", got, err)
	}

	trailing := artifact
	trailing.Bytes = append(append([]byte(nil), artifact.Bytes...), 0)
	trailing.Semantic = CanonicalSemanticIdentity(sha256.Sum256(trailing.Bytes))
	if got, err := DecodeCanonical(context.Background(), reg, trailing); !errors.Is(err, canonical.ErrTrailing) || !Equal(reg, got, Summary{}) {
		t.Fatalf("trailing artifact decoded %#v, %v", got, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := DecodeCanonical(ctx, reg, artifact); !errors.Is(err, context.Canceled) || !Equal(reg, got, Summary{}) {
		t.Fatalf("canceled artifact decoded %#v, %v", got, err)
	}
}

func TestCanonicalSummaryDecoderRejectsImpossibleCountAndInvalidEnumsBeforePublication(t *testing.T) {
	var writer canonical.Writer
	if err := writer.ResetBuffer(context.Background(), canonicalSummaryDomain, canonicalSummaryVersion); err != nil {
		t.Fatal(err)
	}
	if err := writer.Count(^uint64(0)); err != nil {
		t.Fatal(err)
	}
	raw, err := writer.FinishBytes()
	if err != nil {
		t.Fatal(err)
	}
	var reader canonical.Reader
	if err := reader.Reset(context.Background(), raw, canonicalSummaryDomain, canonicalSummaryVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := canonicalDecodeCount(&reader); err == nil {
		t.Fatal("impossible structural count was admitted")
	}

	reg := standard.Registry()
	invalid := []Summary{
		{NormalReturnFacts: callboundary.NormalReturnFacts{BranchProofs: []callboundary.BranchProof{{
			Kind: pathevidence.BranchProofKind(255), Path: pathdom.NewPlaceholder(0),
		}}}},
		{NormalReturnFacts: callboundary.NormalReturnFacts{BranchProofs: []callboundary.BranchProof{{
			Kind: pathevidence.BranchProofPathPresence, Path: pathdom.NewPlaceholder(0), Presence: presence.Value(255),
		}}}},
		{ReturnFlows: []ReturnFlow{{ReturnIndex: 0, Kind: ReturnFlowKind(255), Param: 0}}},
		{ReturnFlows: []ReturnFlow{{ReturnIndex: 0, Kind: ReturnFlowParamMember, Param: 0, Path: []segment.Segment{{Kind: segment.SegmentKind(255)}}}}},
	}
	for index, item := range invalid {
		if err := validateCanonicalDecodedSummary(reg, item); err == nil {
			t.Fatalf("invalid decoded enum fixture %d was admitted", index)
		}
	}
}
