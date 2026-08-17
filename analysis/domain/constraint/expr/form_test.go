package expr

import "testing"

// grammarSamples is one term per shape of the grammar, keyed by the shape it
// stands for. A shape added to the grammar and not sampled here leaves the
// density law below short of the declared count, so the sample set cannot fall
// behind the enumeration it exercises.
func grammarSamples() map[Form]Expr {
	return map[Form]Expr{
		FormVar:      V("i"),
		FormConst:    C(3),
		FormBinOp:    Add(V("i"), C(1)),
		FormLen:      L("arr"),
		FormParam:    P(0),
		FormRet:      R(0),
		FormParamLen: PL(0),
		FormRetLen:   RL(0),
		FormMin:      MinExpr(V("i"), C(1)),
		FormMax:      MaxExpr(V("i"), C(1)),
	}
}

// TestFormCatalogIsTheDenseEnumerationOfEveryValidForm states the density law a
// consumer's exhaustive iteration rests on: the catalog is every form the
// admission predicate accepts, each once, in ordinal order from the first. A
// form added to the type and not to the catalog is a rejected build here rather
// than a shape a consumer silently never visits.
func TestFormCatalogIsTheDenseEnumerationOfEveryValidForm(t *testing.T) {
	var admitted []Form
	for candidate := 0; candidate <= int(^uint8(0)); candidate++ {
		if form := Form(candidate); form.Valid() {
			admitted = append(admitted, form)
		}
	}
	catalog := Forms()
	if len(admitted) != FormCount || len(catalog) != FormCount {
		t.Fatalf("catalog holds %d forms and the type admits %d, declared count is %d", len(catalog), len(admitted), FormCount)
	}
	for position, form := range catalog {
		if form != admitted[position] {
			t.Fatalf("catalog position %d is form %d, but the type's ordinal %d is form %d", position, form, position, admitted[position])
		}
		if int(form) != position+1 {
			t.Fatalf("catalog position %d holds form %d, so the ordinals are not dense from one", position, form)
		}
	}
}

// TestEveryDeclaredFormIsInhabitedByATerm states the catalog is the grammar and
// not a list beside it: each declared form names a term the language can build,
// and that term answers as its own form.
func TestEveryDeclaredFormIsInhabitedByATerm(t *testing.T) {
	samples := grammarSamples()
	if len(samples) != FormCount {
		t.Fatalf("the grammar declares %d forms but %d are inhabited by a term", FormCount, len(samples))
	}
	for _, form := range Forms() {
		term, sampled := samples[form]
		if !sampled {
			t.Fatalf("declared form %d names no term of the grammar", form)
		}
		if got := FormOf(term); got != form {
			t.Fatalf("term %s answers form %d, not the form %d it inhabits", term, got, form)
		}
	}
}

// nilPointerSamples is the nil pointer spelling of every shape of the grammar,
// keyed by the shape whose pointer it is. A shape added to the grammar and not
// sampled here leaves the absent-term law below short of the declared count.
func nilPointerSamples() map[Form]Expr {
	return map[Form]Expr{
		FormVar:      (*Var)(nil),
		FormConst:    (*Const)(nil),
		FormBinOp:    (*BinOp)(nil),
		FormLen:      (*Len)(nil),
		FormParam:    (*Param)(nil),
		FormRet:      (*Ret)(nil),
		FormParamLen: (*ParamLen)(nil),
		FormRetLen:   (*RetLen)(nil),
		FormMin:      (*Min)(nil),
		FormMax:      (*Max)(nil),
	}
}

// TestAbsentTermAnswersInvalidThroughTheGrammarDispatch states the law FormOf
// declares for a term outside the grammar: an absent term is not a member,
// whether it arrives untyped or as the nil pointer spelling of a member's type.
// The dispatch answers Default for it and reads no fields off it, so a consumer
// that asks a term for its shape gets an answer rather than a panic.
func TestAbsentTermAnswersInvalidThroughTheGrammarDispatch(t *testing.T) {
	samples := nilPointerSamples()
	if len(samples) != FormCount {
		t.Fatalf("the grammar declares %d forms but %d have a sampled nil pointer spelling", FormCount, len(samples))
	}
	for _, form := range Forms() {
		term, sampled := samples[form]
		if !sampled {
			t.Fatalf("declared form %d names no nil pointer spelling", form)
		}
		if got := FormOf(term); got != FormInvalid {
			t.Fatalf("the absent %T term answered form %d, not FormInvalid", term, got)
		}
		reached := VisitExpr(term, ExprVisitor[string]{
			Var:      func(Var) string { return "Var" },
			Const:    func(Const) string { return "Const" },
			BinOp:    func(BinOp) string { return "BinOp" },
			Len:      func(Len) string { return "Len" },
			Param:    func(Param) string { return "Param" },
			Ret:      func(Ret) string { return "Ret" },
			ParamLen: func(ParamLen) string { return "ParamLen" },
			RetLen:   func(RetLen) string { return "RetLen" },
			Min:      func(Min) string { return "Min" },
			Max:      func(Max) string { return "Max" },
			Default:  func(Expr) string { return "Default" },
		})
		if reached != "Default" {
			t.Fatalf("the absent %T term reached the %s handler, not Default", term, reached)
		}
	}
}

// TestFormOfAnswersThroughTheGrammarDispatch states that a term reaching the
// grammar by pointer is the same shape as the term reaching it by value, and
// that nothing outside the grammar answers as a member of it.
func TestFormOfAnswersThroughTheGrammarDispatch(t *testing.T) {
	value := V("i")
	if FormOf(&value) != FormVar {
		t.Fatal("a term reached by pointer answered a different form than the same term by value")
	}
	if FormOf(nil).Valid() {
		t.Fatal("a term outside the grammar answered as a declared form")
	}
	if FormInvalid.Valid() || formLimit.Valid() {
		t.Fatal("a form outside the closed grammar was admitted")
	}
}
