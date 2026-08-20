package compiler

// This file is the construction-only ValueSource join.  Literal and
// TypeValue source rows are copied directly from Source/Flow/Static while the
// Program quartet is live; no Program ValueSourceOccurrence is retained by
// the artifact.

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
)

type valueSourceCompileRow struct {
	code              uint64
	term, target      keyspace.Term
	body, bodyContext identity.ContentID
	spanID            identity.ContentID
	finish            causal.Site
	id                identity.ContentID
	literalFamily     keyspace.Family
	literal           keyspace.LiteralValue
	literalOK         bool
}

// valueSourceAt issues one executable source row in the exact Source/Flow
// denominator for code 1..5, or Flow's authored TypeValue denominator for
// code 6. Dead TypeValue candidates deliberately return false.
func (compiler *compiler) valueSourceAt(code uint64, index int) (valueSourceCompileRow, bool) {
	if compiler == nil || !compiler.input.Available() || index < 0 || code < 1 || code > 6 {
		return valueSourceCompileRow{}, false
	}
	input, view := compiler.input, compiler.input.Flow()
	var term, owner, target keyspace.Term
	var ok bool
	switch code {
	case 1:
		term, owner, ok = input.Source().Literals().Nils().At(index)
	case 2:
		term, owner, _, ok = input.Source().Literals().Bools().At(index)
	case 3:
		term, owner, _, ok = input.Source().Literals().Integers().At(index)
	case 4:
		term, owner, _, ok = input.Source().Literals().Floats().At(index)
	case 5:
		term, owner, _, ok = input.Source().Literals().Strings().At(index)
	case 6:
		typeValues := view.Authored().TypeValues()
		term, ok = typeValues.At(index)
		if ok {
			owner, ok = typeValues.Get(term)
		}
		if ok {
			ok = view.Executable().Contains(term)
		}
		if ok {
			target, ok = input.Static().Operands().TypeValues().Target(term)
		}
		if ok {
			ref, refOK := input.Static().StaticTypes().Ref(target)
			ok = refOK && ref.Term() == target
		}
	}
	if !ok || term == 0 || owner == 0 {
		return valueSourceCompileRow{}, false
	}
	body, bodyOK := input.Body(owner)
	bodyPath, bodyPathOK := view.BodyPath(owner)
	programID := compiler.key.ProgramID()
	if !programID.Available() {
		programID = input.ContentID()
	}
	canonicalID, canonicalSpanID, canonicalTerm, canonicalOK := valueSourceIdentityAt(input, programID, keyspace.TermFamily(term), index)
	if !bodyOK || !bodyPathOK || !bodyPath.Available() || !canonicalOK || canonicalTerm != term || !canonicalID.Available() || !canonicalSpanID.Available() {
		return valueSourceCompileRow{}, false
	}
	finish, finishOK := input.Flow().FinishSite(term)
	if !finishOK {
		return valueSourceCompileRow{}, false
	}
	id := canonicalID
	row := valueSourceCompileRow{code: code, term: term, target: target, body: bodyPath, bodyContext: body.ContextID(), spanID: canonicalSpanID, finish: finish, id: id}
	if code < 6 {
		family, literal, literalOK := sourceLiteral(input, term)
		if !literalOK {
			return valueSourceCompileRow{}, false
		}
		row.literalFamily, row.literal, row.literalOK = family, literal, true
	}
	return row, id.Available() && row.body.Available() && row.bodyContext.Available()
}

func (compiler *compiler) typeValueCompileRow(index int) (valueSourceCompileRow, identity.ContentID, identity.ContentID, string, bool) {
	row, ok := compiler.valueSourceAt(6, index)
	if !ok {
		return valueSourceCompileRow{}, identity.ContentID{}, identity.ContentID{}, "", false
	}
	ref, refOK := compiler.input.Static().StaticTypes().Ref(row.target)
	referenceID, referenceOK := staticquery.TypeReferenceID(compiler.input.ContentID(), ref)
	name, nameOK := staticTypeValueName(compiler.input, row.target)
	rootID, rootOK := staticTypeValueRootID(compiler.input.ContentID(), row.body, name)
	if !refOK || !referenceOK || !nameOK || !rootOK {
		return valueSourceCompileRow{}, identity.ContentID{}, identity.ContentID{}, "", false
	}
	return row, referenceID, rootID, name, rootID.Available()
}

func staticTypeValueName(input *program.Program, target keyspace.Term) (string, bool) {
	view := input.Static()
	if primitive, ok := view.Types().Primitives().Get(target); ok {
		name := map[statictypes.PrimitiveKind]string{
			statictypes.PrimitiveNil: "nil", statictypes.PrimitiveBoolean: "boolean", statictypes.PrimitiveNumber: "number",
			statictypes.PrimitiveInteger: "integer", statictypes.PrimitiveString: "string", statictypes.PrimitiveAny: "any",
			statictypes.PrimitiveUnknown: "unknown", statictypes.PrimitiveNever: "never",
		}[primitive]
		return name, name != ""
	}
	_, declaration, _, ok := view.References().Get(target)
	if !ok || declaration == 0 {
		return "", false
	}
	if _, _, key, _, alias := view.Declarations().Aliases().Get(declaration); alias {
		value, valueOK := input.Source().Keys().Exact(key)
		return value.String, valueOK && value.Kind == keyspace.LiteralString && value.String != ""
	}
	if _, key, _, iface := view.Declarations().Interfaces().Get(declaration); iface {
		value, valueOK := input.Source().Keys().Exact(key)
		return value.String, valueOK && value.Kind == keyspace.LiteralString && value.String != ""
	}
	return "", false
}

func staticTypeValueRootID(owner, body identity.ContentID, name string) (identity.ContentID, bool) {
	if !owner.Available() || !body.Available() || name == "" {
		return identity.ContentID{}, false
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("program/static-typevalue-root/v1"))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(owner[:])
	_, _ = hash.Write(body[:])
	var word [8]byte
	binary.BigEndian.PutUint64(word[:], uint64(len(name)))
	_, _ = hash.Write(word[:])
	_, _ = hash.Write([]byte(name))
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}
