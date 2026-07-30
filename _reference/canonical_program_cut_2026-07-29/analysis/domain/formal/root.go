package formal

import "github.com/wippyai/go-lua/analysis/lexicalidentity"

// Root is one finite, vocabulary-qualified coordinate in a sealed relational
// schema. Its complete lexical owner is retained; no digest truncation,
// process-local index, or caller state participates in its identity.
type Root struct {
	owner      lexicalidentity.StableLexicalBodyID
	ordinal    uint64
	vocabulary Vocabulary
}

// NewRoot constructs a typed relational root. The zero owner, ordinal zero,
// and invalid vocabularies are outside the sealed coordinate space.
func NewRoot(owner lexicalidentity.StableLexicalBodyID, ordinal uint64, vocabulary Vocabulary) Root {
	if owner == (lexicalidentity.StableLexicalBodyID{}) || ordinal == 0 || !vocabulary.Valid() {
		return Root{}
	}
	return Root{owner: owner, ordinal: ordinal, vocabulary: vocabulary}
}

func (r Root) Valid() bool {
	return r.owner != (lexicalidentity.StableLexicalBodyID{}) && r.ordinal != 0 && r.vocabulary.Valid()
}

func (r Root) Owner() lexicalidentity.StableLexicalBodyID { return r.owner }
func (r Root) Ordinal() uint64                            { return r.ordinal }
func (r Root) Vocabulary() Vocabulary                     { return r.vocabulary }

// Compare orders complete structural roots by owner bytes, then ordinal, then
// vocabulary. It never observes a dense interning index or presentation text.
func Compare(left, right Root) int {
	leftOwner, rightOwner := left.owner, right.owner
	for index := range leftOwner {
		if leftOwner[index] < rightOwner[index] {
			return -1
		}
		if leftOwner[index] > rightOwner[index] {
			return 1
		}
	}
	if left.ordinal < right.ordinal {
		return -1
	}
	if left.ordinal > right.ordinal {
		return 1
	}
	if left.vocabulary < right.vocabulary {
		return -1
	}
	if left.vocabulary > right.vocabulary {
		return 1
	}
	return 0
}

func (r Root) Less(other Root) bool { return Compare(r, other) < 0 }
