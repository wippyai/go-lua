package summary

import "github.com/wippyai/go-lua/analysis/check/fixpoint/ref"

// Digest is an explicit caller-provided digest for future entry dimensions.
type Digest uint64

// EntryKey identifies the abstract call-entry dimensions for a summary key.
type EntryKey struct {
	Values     Digest
	Facts      Digest
	References Digest
}

// SummaryKey identifies one exact summary entry.
type SummaryKey struct {
	Ref   ref.FuncRef
	Entry EntryKey
}

// DefaultSummaryKey returns the default summary key for r.
func DefaultSummaryKey(r ref.FuncRef) SummaryKey {
	return SummaryKey{Ref: r}
}

// Less reports whether k sorts before other.
func (k SummaryKey) Less(other SummaryKey) bool {
	if k.Ref != other.Ref {
		return k.Ref.Less(other.Ref)
	}
	if k.Entry.Values != other.Entry.Values {
		return k.Entry.Values < other.Entry.Values
	}
	if k.Entry.Facts != other.Entry.Facts {
		return k.Entry.Facts < other.Entry.Facts
	}
	return k.Entry.References < other.Entry.References
}
