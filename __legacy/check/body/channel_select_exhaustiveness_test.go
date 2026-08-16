package body

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/domain/type/channelselect"
)

func TestChannelSelectCaseIndexPreservesDuplicateAndReversedMatches(t *testing.T) {
	selected := pathdom.Path{Root: "selected"}
	result := selected.Field("result")
	resultChannel := result.Field(channelselect.ResultChannelField)
	primary := pathdom.Path{Root: "primary"}
	timers := pathdom.Path{Root: "timers"}
	otherResult := pathdom.Path{Root: "other"}.Field("result")

	index := newChannelSelectCaseIndex([]channelSelectInfo{
		{
			result: result,
			cases: []channelSelectCase{
				{path: primary, name: "primary receive"},
				{path: primary, name: "primary send"},
				{path: timers, name: "timers"},
			},
		},
		{
			result: otherResult,
			cases:  []channelSelectCase{{path: primary, name: "later primary"}},
		},
	})

	matches := index.matchesForCheck(branchcond.Check{
		Kind:      branchcond.CheckPathEqual,
		Path:      primary,
		OtherPath: resultChannel,
	})
	if len(matches) != 2 ||
		matches[0].selectIndex != 0 || matches[0].caseIndex != 0 ||
		matches[1].selectIndex != 0 || matches[1].caseIndex != 1 {
		t.Fatalf("reversed primary matches = %#v, want first select duplicate cases [0 1]", matches)
	}

	matches = index.matchesForCheck(branchcond.Check{
		Kind:      branchcond.CheckPathEqual,
		Path:      resultChannel,
		OtherPath: timers,
	})
	if len(matches) != 1 || matches[0].selectIndex != 0 || matches[0].caseIndex != 2 {
		t.Fatalf("direct timers matches = %#v, want select 0 case [2]", matches)
	}
}
