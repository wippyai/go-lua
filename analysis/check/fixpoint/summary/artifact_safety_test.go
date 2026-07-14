package summary

import (
	"context"
	"reflect"
	"sort"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestArtifactRetentionCallbacksAreAttachedOnlyToReviewedLanes(t *testing.T) {
	var got []string
	for _, lane := range summaryLanes {
		if lane.retentionSafe != nil {
			got = append(got, lane.fieldName)
		}
	}
	sort.Strings(got)
	want := []string{"NormalReturnFacts", "NormalReturnParams", "ReturnConditionParamRefinements", "Returns"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("artifact-retention lane inventory = %v, want %v", got, want)
	}
}

func TestNormalizeArtifactContextAdmitsReviewedIsStrLanes(t *testing.T) {
	reg := standard.Registry()
	value := typevalue.String(reg)
	branchProofs := callboundary.NormalReturnFacts{BranchProofs: []callboundary.BranchProof{{
		Kind: pathevidence.BranchProofPathPresence, Path: pathdom.NewPlaceholder(0), Presence: presence.Present(),
	}}}
	condition := []ReturnConditionParamRefinement{{
		ReturnIndex: 0, ReturnValue: true, Target: pathdom.NewPlaceholder(0), Value: value,
	}}
	tests := map[string]Summary{
		"Returns":                         {Returns: []product.Value{value}},
		"NormalReturnParams":              {NormalReturnParams: []product.Value{value}},
		"NormalReturnFacts":               {NormalReturnFacts: branchProofs},
		"ReturnConditionParamRefinements": {ReturnConditionParamRefinements: condition},
		"combined-is-str": {
			Returns: valueSlice(value), NormalReturnParams: valueSlice(value), NormalReturnFacts: branchProofs,
			ReturnConditionParamRefinements: condition,
		},
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := NormalizeArtifactContext(context.Background(), reg, in); err != nil {
				t.Fatalf("reviewed artifact lane rejected: %v", err)
			}
		})
	}
}

func TestNormalizeArtifactContextFailsClosedForUnreviewedPopulatedLane(t *testing.T) {
	reg := standard.Registry()
	in := Summary{ParamObligations: []product.Value{typevalue.String(reg)}}
	if _, err := NormalizeArtifactContext(context.Background(), reg, in); err == nil {
		t.Fatal("unreviewed populated summary lane crossed artifact boundary")
	}
}

func TestHeapTableObjectsCannotBorrowNormalReturnFactsRetentionProof(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	in := Summary{
		HeapKeySpace: ks,
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			{Kind: "table", Site: "artifact-retention", Index: 1}: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Top()}),
		},
		NormalReturnFacts: callboundary.NormalReturnFacts{BranchProofs: []callboundary.BranchProof{{
			Kind: pathevidence.BranchProofPathPresence, Path: pathdom.NewPlaceholder(0), Presence: presence.Present(),
		}}},
	}
	if _, err := NormalizeArtifactContext(context.Background(), reg, in); err == nil {
		t.Fatal("HeapTableObjects borrowed the NormalReturnFacts retention callback")
	}
}

func valueSlice(value product.Value) []product.Value { return []product.Value{value} }
