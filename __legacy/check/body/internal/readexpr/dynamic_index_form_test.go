package readexpr

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/indexform"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestNormalizeDynamicReadIndexFormCoversCompleteStructuralVocabulary(t *testing.T) {
	array := pathdom.NewPath(symbol.ID(801), "actor").Field("mailbox").Field("queue")
	index := pathdom.NewPath(symbol.ID(802), "cursor").Field("position")
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("invalid source shape")
	}
	expression := func(ref factflow.ExprRef) factflow.ValueSource {
		source, valid := factflow.NewExpressionValueSource(ref, -1, -1, 0, shape)
		if !valid {
			t.Fatalf("invalid expression source %d", ref)
		}
		return source
	}
	literal := func(value int64) factflow.ValueSource {
		source, valid := factflow.NewIntegerLiteralValueSource(value, -1, -1, 0, shape)
		if !valid {
			t.Fatalf("invalid integer source %d", value)
		}
		return source
	}
	pathSource := func(path pathdom.Path) factflow.ValueSource {
		source, valid := factflow.NewPathValueSource(path.Key(), -1, -1, 0, shape)
		if !valid {
			t.Fatalf("invalid path source %s", path.String())
		}
		return source
	}
	op := func(operation string, left, right factflow.ValueSource) factflow.ExpressionOperation {
		out, valid := factflow.NewBinaryExpressionOperation(operation, left, right)
		if !valid {
			t.Fatalf("invalid %s operation", operation)
		}
		return out
	}

	lenOp, _ := factflow.NewUnaryExpressionOperation("#", expression(1))
	facts := factflow.NewFacts(factflow.FactsInput{
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{1: array},
		ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{
			2: lenOp,
			3: op("%", pathSource(index), expression(2)),
			4: op("+", expression(3), literal(1)),
			5: op("*", literal(2), pathSource(index)),
			6: op("+", expression(5), literal(3)),
		},
	})
	config := Config{Facts: facts}

	for _, test := range []struct {
		name   string
		source factflow.ValueSource
		kind   indexform.IndexFormKind
	}{
		{name: "array length", source: expression(2), kind: indexform.IndexFormArrayLength},
		{name: "modulo length", source: expression(4), kind: indexform.IndexFormModuloLength},
		{name: "constant", source: literal(7), kind: indexform.IndexFormConstant},
		{name: "affine", source: expression(6), kind: indexform.IndexFormAffine},
	} {
		t.Run(test.name, func(t *testing.T) {
			dynamic, valid := factflow.NewDynamicIndexExpression(array, test.source)
			if !valid {
				t.Fatal("invalid dynamic expression")
			}
			normalized, valid := normalizeDynamicReadIndexForm(config, dynamic)
			if !valid {
				t.Fatal("normalization failed")
			}
			form, valid := normalized.Form()
			if !valid || form.Kind() != test.kind {
				t.Fatalf("form = %#v/%v, want kind %d", form, valid, test.kind)
			}
			if test.kind == indexform.IndexFormArrayLength || test.kind == indexform.IndexFormModuloLength {
				got, pathOK := form.ArrayPath()
				if !pathOK || !got.Equal(array) {
					t.Fatalf("array path = %v/%v, want %s", got, pathOK, array.String())
				}
			}
			if test.kind == indexform.IndexFormModuloLength {
				integerSource, sourceOK := normalized.IntegerCertificateSource()
				if !sourceOK || integerSource != pathSource(index) {
					t.Fatalf("integer source = %#v/%v, want dividend", integerSource, sourceOK)
				}
			}
			if test.kind == indexform.IndexFormAffine {
				term, termOK := form.Affine()
				gotPath, pathOK := term.Path()
				if !termOK || !pathOK || !gotPath.Equal(index) || term.Coeff() != 2 || term.Offset() != 3 {
					t.Fatalf("affine = %#v path=%v/%v", term, gotPath, pathOK)
				}
			}
		})
	}
}

func TestNormalizeDynamicReadIndexFormFailsClosedForMismatchAndOverflow(t *testing.T) {
	array := pathdom.NewPath(symbol.ID(811), "actor").Field("queue")
	other := pathdom.NewPath(symbol.ID(812), "other").Field("queue")
	index := pathdom.NewPath(symbol.ID(813), "i")
	shape, _ := factflow.NewValueSourceShape(true, false, false, false)
	expression := func(ref factflow.ExprRef) factflow.ValueSource {
		source, _ := factflow.NewExpressionValueSource(ref, -1, -1, 0, shape)
		return source
	}
	literal := func(value int64) factflow.ValueSource {
		source, _ := factflow.NewIntegerLiteralValueSource(value, -1, -1, 0, shape)
		return source
	}
	pathSource, _ := factflow.NewPathValueSource(index.Key(), -1, -1, 0, shape)
	lenOther, _ := factflow.NewUnaryExpressionOperation("#", expression(1))
	wrongMod, _ := factflow.NewBinaryExpressionOperation("%", pathSource, expression(2))
	wrongPlus, _ := factflow.NewBinaryExpressionOperation("+", expression(3), literal(1))
	overflowScale, _ := factflow.NewBinaryExpressionOperation("*", literal(math.MaxInt64), pathSource)
	overflowScaleAgain, _ := factflow.NewBinaryExpressionOperation("*", literal(2), expression(5))
	overflowOffset, _ := factflow.NewBinaryExpressionOperation("+", pathSource, literal(math.MaxInt64))
	overflowOffsetAgain, _ := factflow.NewBinaryExpressionOperation("+", expression(7), literal(1))
	cyclicAffine, _ := factflow.NewBinaryExpressionOperation("+", expression(9), literal(1))
	facts := factflow.NewFacts(factflow.FactsInput{
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{1: other},
		ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{
			2: lenOther, 3: wrongMod, 4: wrongPlus,
			5: overflowScale, 6: overflowScaleAgain,
			7: overflowOffset, 8: overflowOffsetAgain, 9: cyclicAffine,
		},
	})
	for _, ref := range []factflow.ExprRef{4, 6, 8, 9} {
		dynamic, _ := factflow.NewDynamicIndexExpression(array, expression(ref))
		if normalized, ok := normalizeDynamicReadIndexForm(Config{Facts: facts}, dynamic); ok {
			t.Fatalf("expression %d normalized as %#v", ref, normalized)
		}
	}
}
