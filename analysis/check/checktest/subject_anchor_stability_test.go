package checktest

import (
	"reflect"
	"sort"
	"testing"

	internalreadmodel "github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	obligationpass "github.com/wippyai/go-lua/analysis/check/obligation/pass"
)

func TestSubjectAnchorsStableAcrossUnrelatedInsertion(t *testing.T) {
	base := `local function need_string(value: string): () end
need_string(1)`
	inserted := `local unrelated = 0
local function need_string(value: string): () end
need_string(1)`

	baseAnchors := subjectAnchorSetForSource(t, base)
	insertedAnchors := subjectAnchorSetForSource(t, inserted)
	if !reflect.DeepEqual(baseAnchors, insertedAnchors) {
		t.Fatalf("anchor sets differ after unrelated insertion\nbase:     %#v\ninserted: %#v", baseAnchors, insertedAnchors)
	}
}

func subjectAnchorSetForSource(t *testing.T, src string) []string {
	t.Helper()
	checked := CheckFile(src, "test.lua")
	root := checked.RootResult()
	if root == nil {
		t.Fatal("RootResult nil")
	}
	items := obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:anchor-stability",
		SourceFile:  "test.lua",
		Reader:      internalreadmodel.New(root),
	})
	var anchors []string
	for _, item := range items {
		if item.Code != judgment.CodeCallArgType {
			continue
		}
		if item.Subject.Anchor.IsZero() {
			t.Fatalf("judgment has no subject anchor: %#v", item.Subject)
		}
		anchors = append(anchors, string(item.Code)+"|"+item.Subject.Anchor.StableKey())
	}
	if len(anchors) == 0 {
		t.Fatal("no anchored call argument judgments")
	}
	sort.Strings(anchors)
	return anchors
}
