package formal

import "github.com/wippyai/go-lua/analysis/lexicalidentity"

// LexicalClassID is the body-qualified identity of one immutable lexical
// binding class. Root identity deliberately remains unchanged.
type LexicalClassID struct {
	owner   lexicalidentity.StableLexicalBodyID
	ordinal uint64
}

func NewLexicalClassID(owner lexicalidentity.StableLexicalBodyID, ordinal uint64) LexicalClassID {
	if owner == (lexicalidentity.StableLexicalBodyID{}) || ordinal == 0 {
		return LexicalClassID{}
	}
	return LexicalClassID{owner: owner, ordinal: ordinal}
}

func (c LexicalClassID) Valid() bool {
	return c.owner != (lexicalidentity.StableLexicalBodyID{}) && c.ordinal != 0
}
func (c LexicalClassID) Owner() lexicalidentity.StableLexicalBodyID { return c.owner }
func (c LexicalClassID) Ordinal() uint64                            { return c.ordinal }

// OccurrenceID identifies one frozen producer occurrence. It is deliberately
// distinct from a lexical class and cannot authorize another occurrence.
type OccurrenceID struct {
	owner   lexicalidentity.StableLexicalBodyID
	ordinal uint64
}

func NewOccurrenceID(owner lexicalidentity.StableLexicalBodyID, ordinal uint64) OccurrenceID {
	if owner == (lexicalidentity.StableLexicalBodyID{}) || ordinal == 0 {
		return OccurrenceID{}
	}
	return OccurrenceID{owner: owner, ordinal: ordinal}
}

func (o OccurrenceID) Valid() bool {
	return o.owner != (lexicalidentity.StableLexicalBodyID{}) && o.ordinal != 0
}
func (o OccurrenceID) Owner() lexicalidentity.StableLexicalBodyID { return o.owner }
func (o OccurrenceID) Ordinal() uint64                            { return o.ordinal }
