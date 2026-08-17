package equation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/internal/canonical"
)

// PortMode is the closed direction of one formal/concrete Point connection.
// It is immutable ABI metadata, not an activation predicate or runtime
// capability.
type PortMode uint8

const (
	PortInvalid PortMode = iota
	PortImport
	PortExport
	PortImportExport
)

func (mode PortMode) imports() bool { return mode == PortImport || mode == PortImportExport }
func (mode PortMode) exports() bool { return mode == PortExport || mode == PortImportExport }

// PortRead is one named exact-read ABI slot. Role is semantic rather than
// positional; Surface.Local is the sole field a concrete binding may replace.
type PortRead struct {
	Role    composition.Key
	Surface Surface
}

func canonicalPortReads(values []PortRead) ([]PortRead, bool) {
	if len(values) == 0 {
		return nil, true
	}
	result := append([]PortRead(nil), values...)
	sort.Slice(result, func(left, right int) bool { return lessKey(result[left].Role, result[right].Role) })
	for index, read := range result {
		if !read.Role.Available() || !read.Surface.Available() || index > 0 && result[index-1].Role == read.Role {
			return nil, false
		}
	}
	return result, true
}

func samePortReadSlots(left, right []PortRead) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func writePortReads(writer *canonical.DigestWriter, reads []PortRead) bool {
	if writer.Count(uint64(len(reads))) != nil {
		return false
	}
	for _, read := range reads {
		if !writeKey(writer, read.Role) || !writeSurface(writer, read.Surface) {
			return false
		}
	}
	return true
}
