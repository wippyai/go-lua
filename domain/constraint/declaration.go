// Package constraint is the root of the analyzer's constraint domain and the
// domain's declaration statement against the analyzer declaration table.
//
// # What the domain owns
//
// One thing: the symbolic integer expression language in
// analysis/domain/constraint/expr. It is a closed grammar of ten forms - a
// variable, a constant, a binary arithmetic application over five operators, a
// length, a parameter reference, a return reference, the two length shorthands
// over those references, and the two extrema - together with the six-member
// relation vocabulary those forms are compared under, the string boundary the
// symbolic references are keyed by, structural equality, constant folding, and
// evaluation against a concrete environment.
//
// The language is a value grammar, not a fact domain. It holds no coordinate
// space, publishes no fact, and runs nothing during a solve. Its terms are
// carried as data by effect labels - a mutation's length delta and a reserved
// return length - and are serialized by the module manifest codec. Both
// consumers already import it for their own semantics, so the domain sits
// below them and imports no peer domain of its own.
//
// # Declaration
//
// The domain declares one row set, on the structural vocabulary surface, and
// nothing on the other seven. The reason it declares nothing on those seven is
// the same one in each case: a surface row there is a statement about the
// solver, and this domain is not in the solver.
//
//   - Axis. An axis is a coordinate space the solver holds facts over, carrying
//     an algebra, a storage discipline, a writer principal, and a key
//     cardinality. The expression language holds no facts; a term is a value a
//     consumer carries, so there is no space to declare.
//   - Rule. A rule declares an engine slot at an artifact rule role and attaches
//     at mount points. The domain has no engine hot factor: nothing in it is
//     bound at a mount, evaluated at a point, or admitted into a receipt.
//   - Diagnostic. A diagnostic row publishes a code from facts the analyzer
//     already produces. The domain produces none. Evaluation that cannot
//     conclude - an unbound variable, a division by zero, an integer overflow -
//     is answered to its caller as an unevaluated term, which is the caller's
//     input to its own reasoning rather than a published finding.
//   - Composite. A composite is a relation over declared coordinate spaces, and
//     every axis it names must resolve. The domain names none.
//   - Denominator. A denominator names the surface entry whose universe it
//     quantifies over, and that owner must be an entry on a surface sealed
//     below the denominator surface. The domain owns no such entry, so it has
//     nothing to close a world around.
//   - Query. A registration names a result codec, a fold contract, and the
//     coordinate spaces it reads. The domain answers no query family and reads
//     no coordinate space.
//   - Structure. The structural vocabulary surface holds the closed catalogs the
//     analyzer would otherwise spell once per consumer. The expression form
//     catalog is one of those: ten members a consumer switches on exhaustively.
//     StructureSpecs below is the domain's declaration of it.
//   - Library. A library contract kind publishes exported values under a member
//     form algebra. Expression terms ride inside the effect-label form, which
//     the effect domain owns; the domain publishes no contract kind of its own.
//
// # The second spelling
//
// The form catalog is spelled twice. Once in the grammar, as the enumeration
// expr.Forms and the dispatch expr.VisitExpr, and once in the module manifest
// codec, as the wire kind strings encodeExpr and decodeExpr switch on. The
// declaration here is what the two are held against: the codec's spelling is
// pinned to these sealed rows, so a form added to the grammar and not to the
// codec is a rejected build rather than a term that serializes as an error at
// write time.
package constraint

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/constraint/expr"
)

// StructureSpecs is this domain's declaration of the symbolic expression form
// vocabulary: one row per shape the grammar can build.
//
// Neither the membership nor the ordinals are invented here. Both are read from
// expr.Forms, the grammar's own closed enumeration, so a form added, removed,
// or reordered in the grammar moves this declaration with it rather than
// leaving a second list behind to drift.
//
// A row's key is the form's name on this surface, and its spelling is the name
// the form renders as. Neither is the codec's wire spelling: the wire strings
// are the manifest's own boundary vocabulary, pinned to these rows by law,
// because a wire spelling is a serialization commitment the codec owns and this
// declaration adopts nothing of.
//
// Every form is projected. The grammar builds terms of any of them, so no
// member is held back from the projection this vocabulary feeds.
func StructureSpecs() []structure.Spec {
	forms := expr.Forms()
	specs := make([]structure.Spec, 0, len(forms))
	for _, form := range forms {
		key, spelling := formNaming(form)
		specs = append(specs, structure.Spec{
			Key:      key,
			Category: structure.CategoryConstraintForm,
			Ordinal:  uint16(form),
			Spelling: spelling,
			Accepted: true,
		})
	}
	return specs
}

// FormKey is one grammar form's name on the structural vocabulary surface. A
// form the grammar admits and this naming does not answer for yields no key, so
// the declaration it would produce is rejected at construction rather than
// sealed under an empty identity.
func FormKey(form expr.Form) schema.Key {
	key, _ := formNaming(form)
	return key
}

// FormSpelling is one grammar form's rendered name on the structural
// vocabulary surface.
func FormSpelling(form expr.Form) string {
	_, spelling := formNaming(form)
	return spelling
}

// formNaming answers a form's surface key and its rendered spelling from one
// statement, so the two cannot name different sets of forms.
func formNaming(form expr.Form) (schema.Key, string) {
	switch form {
	case expr.FormVar:
		return "constraint-form/var", "var"
	case expr.FormConst:
		return "constraint-form/const", "const"
	case expr.FormBinOp:
		return "constraint-form/binop", "binop"
	case expr.FormLen:
		return "constraint-form/len", "len"
	case expr.FormParam:
		return "constraint-form/param", "param"
	case expr.FormRet:
		return "constraint-form/ret", "ret"
	case expr.FormParamLen:
		return "constraint-form/param-len", "param-len"
	case expr.FormRetLen:
		return "constraint-form/ret-len", "ret-len"
	case expr.FormMin:
		return "constraint-form/min", "min"
	case expr.FormMax:
		return "constraint-form/max", "max"
	default:
		return "", ""
	}
}

// FormFor projects one declared row back to the grammar form it declares. A
// consumer reading the sealed vocabulary recovers the form through this rather
// than converting an ordinal of its own.
func FormFor(entry *structure.Entry) (expr.Form, bool) {
	if entry == nil || entry.Category() != structure.CategoryConstraintForm {
		return expr.FormInvalid, false
	}
	form := expr.Form(entry.Ordinal())
	return form, form.Valid()
}
