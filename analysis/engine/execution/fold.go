package execution

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
)

// FoldExact is the one-read identity fold: the first exact cell is staged to
// the write axis, sparse absence and an empty cursor are NoCandidate, and any
// other cursor failure is Refuse. Ticket remains open for Submit.
func FoldExact[K scalar.Key, V any](ticket Ticket, read ExactRead[K, V], write ExactWrite[K, V], scratch *Scratch[K, V]) Outcome {
	if scratch == nil || !read.Valid() || !write.Valid() {
		return Refuse
	}
	switch read.Read(ticket, scratch) {
	case ReadAvailable:
		region, regionOK := scratch.Region()
		value, valueOK := scratch.Value()
		present := scratch.Present()
		if !read.Close(ticket, scratch) {
			_ = scratch.Discard(ticket)
			return Refuse
		}
		if !present {
			return NoCandidate
		}
		if regionOK && valueOK && write.Stage(ticket, scratch, region, value) && write.Close(ticket, scratch) {
			return Concrete
		}
		_ = scratch.Discard(ticket)
		return Refuse
	case ReadExhausted:
		if read.Close(ticket, scratch) {
			return NoCandidate
		}
		return Refuse
	default:
		_ = scratch.Discard(ticket)
		return Refuse
	}
}
