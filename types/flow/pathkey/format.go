package pathkey

import (
	"strconv"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// SymbolRoot returns the canonical symbol root "sym<id>".
func SymbolRoot(sym cfg.SymbolID) string {
	return "sym" + strconv.FormatUint(uint64(sym), 10)
}

// SymbolVersionRoot returns the canonical versioned symbol root "sym<id>@<version>".
func SymbolVersionRoot(sym cfg.SymbolID, versionID int) string {
	return SymbolRoot(sym) + "@" + strconv.Itoa(versionID)
}

// SymbolVersionKey returns the canonical versioned key for a symbol path.
func SymbolVersionKey(sym cfg.SymbolID, versionID int, segments []constraint.Segment) constraint.PathKey {
	if sym == 0 || versionID == 0 {
		return ""
	}
	return constraint.PathKey(SymbolVersionRoot(sym, versionID) + SegmentsSuffix(segments))
}
