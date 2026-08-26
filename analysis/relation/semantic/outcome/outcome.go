package outcome

import "github.com/wippyai/go-lua/analysis/relation/schema/model"

// Code is the closed terminal disposition of one semantic invocation.
type Code uint8

const (
	Invalid Code = iota
	Produced
	NoCandidate
	NoSelection
	Opaque
	Refused
)

func (code Code) Available() bool { return code >= Produced && code <= Refused }

// Publishes reports whether the disposition carries a fact. Produced carries
// the operation's answer; Opaque carries the authenticated-opaque fact the
// population keeps. Every other disposition seals an empty batch.
func (code Code) Publishes() bool { return code == Produced || code == Opaque }

// Result is one closed terminal outcome. Only Refused carries a refusal
// identity; every other code must leave RefusalID unavailable.
type Result struct {
	Code      Code
	RefusalID model.RefusalID
}

func NewResult(code Code, refusalID model.RefusalID) (Result, bool) {
	result := Result{Code: code, RefusalID: refusalID}
	return result, result.Available()
}

func (result Result) Available() bool {
	if !result.Code.Available() {
		return false
	}
	if result.Code == Refused {
		return result.RefusalID.Available()
	}
	return !result.RefusalID.Available()
}

// Set is an immutable ordered outcome vocabulary used by a sealed signature.
type Set struct{ codes []Code }

func NewSet(codes ...Code) (Set, bool) {
	if len(codes) == 0 {
		return Set{}, false
	}
	copyOf := make([]Code, len(codes))
	for index, code := range codes {
		if !code.Available() {
			return Set{}, false
		}
		for _, prior := range copyOf[:index] {
			if prior == code {
				return Set{}, false
			}
		}
		copyOf[index] = code
	}
	return Set{codes: copyOf}, true
}

func Singleton(code Code) (Set, bool) { return NewSet(code) }

func (set Set) Available() bool { return len(set.codes) > 0 }

func (set Set) Len() int { return len(set.codes) }

func (set Set) At(index int) (Code, bool) {
	if index < 0 || index >= len(set.codes) {
		return Invalid, false
	}
	return set.codes[index], true
}

func (set Set) Contains(code Code) bool {
	for _, candidate := range set.codes {
		if candidate == code {
			return true
		}
	}
	return false
}

func (set Set) Codes() []Code { return append([]Code(nil), set.codes...) }

func (set Set) Equal(other Set) bool {
	if len(set.codes) != len(other.codes) {
		return false
	}
	for index, code := range set.codes {
		if code != other.codes[index] {
			return false
		}
	}
	return true
}
