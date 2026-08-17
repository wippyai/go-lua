package lualib

import (
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/type/normalize"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

// The authored typed application of every utf8-library export. Every member
// reads its subject and writes nothing, so each borrows what it is handed.
//
// Two of them are partial over their subject and say so in their result rather
// than by throwing: a length that cannot be measured and an offset that does not
// exist are answers, not failures, so the slot they arrive in is optional.

var utf8Signatures = map[string]signature.Function{
	"char": authored(typ.Func().
		Variadic(typ.Integer).
		Returns(typ.String).
		Build()),
	// utf8.codepoint reads the sequences between two byte positions and answers
	// with one integer per sequence. The contract states the first slot, which is
	// the one a caller reaches without unpacking the result row.
	"codepoint": authored(typ.Func().
		Param("s", typ.String).
		OptParam("i", typ.Integer).
		OptParam("j", typ.Integer).
		Returns(typ.Integer).
		Build(),
		ownership.BorrowAll{}),
	// utf8.codes answers the generic-for protocol triple over its subject: the
	// step function, the subject it steps over, and the byte offset it starts at.
	"codes": authored(typ.Func().
		Param("s", typ.String).
		Returns(typ.Any, typ.String, typ.Integer).
		Build(),
		ownership.BorrowAll{}),
	// utf8.len answers the number of sequences in the measured range, and answers
	// with nothing plus the offending byte position when the range is not valid
	// UTF-8. Both slots are optional because exactly one of them is present.
	"len": authored(typ.Func().
		Param("s", typ.String).
		OptParam("i", typ.Integer).
		OptParam("j", typ.Integer).
		Returns(normalize.Optional(typ.Integer), normalize.Optional(typ.Integer)).
		Build(),
		ownership.BorrowAll{}),
	// utf8.offset answers the byte position of the n-th sequence, and answers
	// with nothing when the subject holds no such sequence.
	"offset": authored(typ.Func().
		Param("s", typ.String).
		Param("n", typ.Integer).
		OptParam("i", typ.Integer).
		Returns(normalize.Optional(typ.Integer)).
		Build(),
		ownership.BorrowAll{}),
}
