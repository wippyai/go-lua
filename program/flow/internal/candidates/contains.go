package candidates

import "github.com/wippyai/go-lua/program/keyspace"

const (
	unaryNoCandidate uint8 = iota
	unaryNumericCandidate
	unaryLengthCandidate
)

const (
	binaryNoCandidate uint8 = iota
	binaryArithmeticCandidate
	binaryBitwiseCandidate
	binaryConcatCandidate
	binaryEqualityCandidate
	binaryOrderCandidate
)

const (
	accessNoCandidate uint8 = iota
	accessIndexCandidate
)

const genericLoopCandidate uint8 = 1

func (r *Result) unaryContains(term keyspace.Term, class uint8) bool {
	if !r.available() || keyspace.TermFamily(term) != keyspace.FamilyUnary {
		return false
	}
	ordinal := keyspace.TermOrdinal(term)
	return ordinal != 0 && uint64(ordinal) <= uint64(len(r.classes.unaryClass)) && r.classes.unaryClass[ordinal-1] == class
}

func (r *Result) binaryContains(term keyspace.Term, class uint8) bool {
	if !r.available() || keyspace.TermFamily(term) != keyspace.FamilyBinary {
		return false
	}
	ordinal := keyspace.TermOrdinal(term)
	return ordinal != 0 && uint64(ordinal) <= uint64(len(r.classes.binaryClass)) && r.classes.binaryClass[ordinal-1] == class
}

func (r *Result) readContains(term keyspace.Term) bool {
	if !r.available() || keyspace.TermFamily(term) != keyspace.FamilyRead {
		return false
	}
	ordinal := keyspace.TermOrdinal(term)
	return ordinal != 0 && uint64(ordinal) <= uint64(len(r.classes.readClass)) && r.classes.readClass[ordinal-1] == accessIndexCandidate
}

func (r *Result) writeContains(term keyspace.Term) bool {
	if !r.available() || keyspace.TermFamily(term) != keyspace.FamilyWrite {
		return false
	}
	ordinal := keyspace.TermOrdinal(term)
	return ordinal != 0 && uint64(ordinal) <= uint64(len(r.classes.writeClass)) && r.classes.writeClass[ordinal-1] == accessIndexCandidate
}

func (r *Result) loopContains(term keyspace.Term) bool {
	if !r.available() || keyspace.TermFamily(term) != keyspace.FamilyLoop {
		return false
	}
	ordinal := keyspace.TermOrdinal(term)
	return ordinal != 0 && uint64(ordinal) <= uint64(len(r.classes.loopClass)) && r.classes.loopClass[ordinal-1] == genericLoopCandidate
}
