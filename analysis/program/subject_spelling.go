package program

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// subjectSpellingDepthLimit bounds one rendered path. An authored access chain
// is finite, but the renderer walks a relation rather than a tree, so the bound
// keeps a malformed chain from spinning instead of answering.
const subjectSpellingDepthLimit = 32

// subjectSpellingByteLimit bounds one rendered path's length. A diagnostic
// names its subject to locate it, not to reproduce the program, and a template
// token that grows without bound is not a name.
const subjectSpellingByteLimit = 120

// SubjectSpelling is the authored spelling of one expression term as the
// source wrote it: the binder name a read names, the dotted or bracketed
// access path a lens names, and the call form a call names with its actuals
// elided. It is the one place the analyzer projects the authored access
// relations back to the text a finding refers to its subject by.
//
// The projection is exact or absent. Every component comes from an authored
// relation - the Cell spelling row, the Source key literal, the Call spelling
// row - and a component none of them supplies ends the answer rather than
// being replaced by a placeholder. A diagnostic that cannot name its subject
// says so; it never invents a name the source did not write.
//
// The elision in a call form is deliberate and is not a missing component: the
// actuals are separate subjects with separate spellings, and a finding about
// the call's own value names the call, not its arguments.
func (program *Program) SubjectSpelling(term keyspace.Term) (string, bool) {
	if !program.Available() || term == 0 {
		return "", false
	}
	var builder strings.Builder
	if !program.writeSubjectSpelling(&builder, term, 0) || builder.Len() == 0 || builder.Len() > subjectSpellingByteLimit {
		return "", false
	}
	return builder.String(), true
}

// writeSubjectSpelling appends one term's authored spelling. It answers false
// for every term family whose authored text this projection does not hold, so
// a partially rendered path never escapes as a whole one.
func (program *Program) writeSubjectSpelling(builder *strings.Builder, term keyspace.Term, depth int) bool {
	if depth >= subjectSpellingDepthLimit || term == 0 || builder.Len() > subjectSpellingByteLimit {
		return false
	}
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyRead:
		return program.writeReadSpelling(builder, term)
	case keyspace.FamilyLensExact:
		return program.writeLensSpelling(builder, term, depth)
	case keyspace.FamilyCall:
		return program.writeCallSpelling(builder, term, depth)
	default:
		return false
	}
}

// writeReadSpelling names the cell a read reaches. A global cell is named by
// its authored key literal, which is the same row the unresolved-value
// population publishes; every other cell is named by its authored debug
// spelling row.
func (program *Program) writeReadSpelling(builder *strings.Builder, term keyspace.Term) bool {
	reads := program.Flow().Authored().Storage().Reads()
	_, source, _, relationOK := reads.Get(term)
	if !relationOK || source == 0 {
		return false
	}
	cellKind, _, key, cellOK := program.Flow().Authored().Storage().Cells().Get(source)
	if !cellOK {
		return false
	}
	if cellKind == authored.CellGlobal && key != 0 {
		literal, literalOK := program.Source().Keys().Exact(key)
		if !literalOK || literal.Kind != keyspace.LiteralString || literal.String == "" {
			return false
		}
		builder.WriteString(literal.String)
		return true
	}
	name, named := program.Source().Spellings().CellName(source)
	if !named || name == "" {
		return false
	}
	builder.WriteString(name)
	return true
}

// writeLensSpelling names one member access as the source wrote it: a dotted
// name for a name field, a bracketed literal for a key or list field. A
// dynamic lens has no authored key text and is therefore not spelled.
func (program *Program) writeLensSpelling(builder *strings.Builder, term keyspace.Term, depth int) bool {
	_, base, source, fieldKind, relationOK := program.Flow().Authored().Access().Exact().Get(term)
	if !relationOK || base == 0 || source == 0 {
		return false
	}
	if !program.writeSubjectSpelling(builder, base, depth+1) {
		return false
	}
	keys := program.Source().Keys()
	switch fieldKind {
	case kind.FieldName:
		_, name, _, nameOK := keys.Name(source)
		if !nameOK || name == "" {
			return false
		}
		builder.WriteString(".")
		builder.WriteString(name)
		return true
	case kind.FieldKey:
		_, name, _, nameOK := keys.Name(source)
		if !nameOK || name == "" {
			return false
		}
		builder.WriteString("[")
		builder.WriteString(strconv.Quote(name))
		builder.WriteString("]")
		return true
	case kind.FieldList, kind.FieldExact:
		_, ordinal, _, listOK := keys.List(source)
		if !listOK {
			return false
		}
		builder.WriteString("[")
		builder.WriteString(strconv.FormatInt(ordinal, 10))
		builder.WriteString("]")
		return true
	default:
		return false
	}
}

// writeCallSpelling names one call by its callee and an elided actual list. A
// method call keeps the authored colon, because the receiver it names is part
// of the subject and the plain form would name a different expression.
func (program *Program) writeCallSpelling(builder *strings.Builder, term keyspace.Term, depth int) bool {
	_, callee, receiver, _, relationOK := program.Flow().Authored().Calls().Get(term)
	if !relationOK {
		return false
	}
	if receiver != 0 {
		name, named := program.Source().Spellings().CallName(term)
		if !named || name == "" || !program.writeSubjectSpelling(builder, receiver, depth+1) {
			return false
		}
		builder.WriteString(":")
		builder.WriteString(name)
		builder.WriteString("(...)")
		return true
	}
	if callee == 0 || !program.writeSubjectSpelling(builder, callee, depth+1) {
		return false
	}
	builder.WriteString("(...)")
	return true
}
