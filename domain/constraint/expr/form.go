package expr

// Form is one member of the closed grammar this package owns: the ten shapes a
// term of the symbolic integer language can have. It is the grammar stated as a
// value, so a consumer that has to visit, serialize, or declare every shape
// reaches for the enumeration instead of restating the member list at its own
// site.
//
// The ordinals are dense from FormVar and are this package's own numbering.
// They are not a wire format: a serializer that needs a stable external
// spelling declares one against this catalog rather than writing the ordinal.
type Form uint8

const (
	FormInvalid Form = iota
	FormVar
	FormConst
	FormBinOp
	FormLen
	FormParam
	FormRet
	FormParamLen
	FormRetLen
	FormMin
	FormMax
	formLimit
)

// FormCount is the size of the closed grammar. The ordinals are dense from
// FormVar, so a consumer indexes by form without a lookup.
const FormCount = int(formLimit) - 1

// Valid reports membership in the closed grammar.
func (form Form) Valid() bool { return form > FormInvalid && form < formLimit }

// Forms is the grammar catalog in ordinal order. It is the one enumeration of
// the vocabulary this package owns, so a consumer that visits every shape
// projects it instead of restating the member list. The catalog is returned by
// value and costs no allocation to range over.
func Forms() [FormCount]Form {
	return [FormCount]Form{
		FormVar, FormConst, FormBinOp, FormLen, FormParam,
		FormRet, FormParamLen, FormRetLen, FormMin, FormMax,
	}
}

// FormOf answers which shape a term has. The answer comes from the grammar's
// one dispatch, VisitExpr, so a shape added to the grammar is answered here
// without a second type switch to keep in step. A term outside the grammar,
// including a nil term, answers FormInvalid.
func FormOf(e Expr) Form {
	return VisitExpr(e, ExprVisitor[Form]{
		Var:      func(Var) Form { return FormVar },
		Const:    func(Const) Form { return FormConst },
		BinOp:    func(BinOp) Form { return FormBinOp },
		Len:      func(Len) Form { return FormLen },
		Param:    func(Param) Form { return FormParam },
		Ret:      func(Ret) Form { return FormRet },
		ParamLen: func(ParamLen) Form { return FormParamLen },
		RetLen:   func(RetLen) Form { return FormRetLen },
		Min:      func(Min) Form { return FormMin },
		Max:      func(Max) Form { return FormMax },
		Default:  func(Expr) Form { return FormInvalid },
	})
}
