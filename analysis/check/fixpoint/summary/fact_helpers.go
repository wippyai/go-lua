package summary

import "github.com/wippyai/go-lua/analysis/domain/value/axis/presence"

func presenceLessOrEq(a, b presence.Value) bool {
	return presence.Join(a, b) == b
}
