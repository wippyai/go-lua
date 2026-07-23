package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/indexform"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
)

// IntegerConstantResolver recognizes an exact integer-valued source that is
// not already a raw factflow integer literal. It keeps factflow independent of
// value-axis/type layers while preserving extracted literal witnesses.
type IntegerConstantResolver func(ValueSource) (int64, bool)

// DynamicReadIndexForm is the sealed factflow result for one dynamic read.
// Form is syntax-free and comparable; modulo forms additionally retain the
// exact dividend source whose valuation must supply the integer certificate.
type DynamicReadIndexForm struct {
	form             indexform.IndexForm
	integerSource    ValueSource
	hasIntegerSource bool
	sealed           bool
}

// NormalizeDynamicReadIndexForm lowers the complete supported expression
// vocabulary to a syntax-free engine form. Unsupported and overflowing terms
// fail closed. Facts remains the sole owner of expression-graph traversal.
func (f Facts) NormalizeDynamicReadIndexForm(dynamic DynamicIndexExpression, resolveInteger IntegerConstantResolver) (DynamicReadIndexForm, bool) {
	array := dynamic.TablePathRef()
	if f.sourceIsLengthOfPath(dynamic.KeySource(), array) {
		form, ok := indexform.NewArrayLengthIndex(array)
		return DynamicReadIndexForm{form: form, sealed: ok}, ok
	}
	if dividend, ok := f.moduloLengthDividend(resolveInteger, dynamic.KeySource(), array); ok {
		form, formOK := indexform.NewModuloLengthIndex(array)
		if !formOK {
			return DynamicReadIndexForm{}, false
		}
		return DynamicReadIndexForm{
			form: form, integerSource: dividend, hasIntegerSource: true, sealed: true,
		}, true
	}
	if constant, ok := integerConstant(dynamic.KeySource(), resolveInteger); ok {
		return DynamicReadIndexForm{form: indexform.NewConstantIndex(constant), sealed: true}, true
	}
	term, ok := f.affineIndex(resolveInteger, dynamic.KeySource(), nil)
	if !ok {
		return DynamicReadIndexForm{}, false
	}
	form, ok := indexform.NewAffineIndex(term.path, term.coeff, term.offset)
	return DynamicReadIndexForm{form: form, sealed: ok}, ok
}

// Form returns the syntax-free, comparable engine descriptor.
func (f DynamicReadIndexForm) Form() (indexform.IndexForm, bool) {
	return f.form, f.sealed && f.form.Valid()
}

// IntegerCertificateSource returns the exact modulo dividend source. The
// evidence binder must prove its valuation integer; syntax never grants it.
func (f DynamicReadIndexForm) IntegerCertificateSource() (ValueSource, bool) {
	return f.integerSource, f.sealed && f.form.Kind() == indexform.IndexFormModuloLength && f.hasIntegerSource
}

type affineIndex struct {
	path   pathdom.Path
	coeff  int64
	offset int64
}

func (f Facts) affineIndex(resolveInteger IntegerConstantResolver, source ValueSource, active map[ExprRef]bool) (affineIndex, bool) {
	if path, ok := f.sourcePath(source); ok {
		return affineIndex{path: path, coeff: 1}, true
	}
	if source.Kind != ValueSourceExpression || !source.HasExpr {
		return affineIndex{}, false
	}
	if active[source.ExprRef] {
		return affineIndex{}, false
	}
	if path, ok := f.ExpressionPathRef(source.ExprRef); ok {
		return affineIndex{path: path, coeff: 1}, true
	}
	operation, ok := f.ExpressionOperation(source.ExprRef)
	if !ok || operation.Kind() != ExpressionOperationBinary {
		return affineIndex{}, false
	}
	if active == nil {
		active = make(map[ExprRef]bool, 1)
	}
	active[source.ExprRef] = true
	defer delete(active, source.ExprRef)
	switch operation.Op() {
	case "+":
		if term, ok := f.affinePlusConstant(resolveInteger, operation.Left(), operation.Right(), 1, active); ok {
			return term, true
		}
		return f.affinePlusConstant(resolveInteger, operation.Right(), operation.Left(), 1, active)
	case "-":
		return f.affinePlusConstant(resolveInteger, operation.Left(), operation.Right(), -1, active)
	case "*":
		if term, ok := f.affineScaled(resolveInteger, operation.Left(), operation.Right(), active); ok {
			return term, true
		}
		return f.affineScaled(resolveInteger, operation.Right(), operation.Left(), active)
	default:
		return affineIndex{}, false
	}
}

func (f Facts) affineScaled(resolveInteger IntegerConstantResolver, constantSource, termSource ValueSource, active map[ExprRef]bool) (affineIndex, bool) {
	constant, ok := integerConstant(constantSource, resolveInteger)
	if !ok || constant <= 0 {
		return affineIndex{}, false
	}
	term, ok := f.affineIndex(resolveInteger, termSource, active)
	if !ok {
		return affineIndex{}, false
	}
	coeff, coeffOK := indexform.CheckedMulInt64(term.coeff, constant)
	offset, offsetOK := indexform.CheckedMulInt64(term.offset, constant)
	if !coeffOK || !offsetOK || coeff <= 0 {
		return affineIndex{}, false
	}
	term.coeff, term.offset = coeff, offset
	return term, true
}

func (f Facts) affinePlusConstant(resolveInteger IntegerConstantResolver, termSource, constantSource ValueSource, sign int64, active map[ExprRef]bool) (affineIndex, bool) {
	term, ok := f.affineIndex(resolveInteger, termSource, active)
	if !ok {
		return affineIndex{}, false
	}
	constant, ok := integerConstant(constantSource, resolveInteger)
	if !ok {
		return affineIndex{}, false
	}
	delta, deltaOK := indexform.CheckedMulInt64(sign, constant)
	if !deltaOK {
		return affineIndex{}, false
	}
	offset, offsetOK := indexform.CheckedAddInt64(term.offset, delta)
	if !offsetOK {
		return affineIndex{}, false
	}
	term.offset = offset
	return term, true
}

func (f Facts) moduloLengthDividend(resolveInteger IntegerConstantResolver, source ValueSource, array pathdom.Path) (ValueSource, bool) {
	plus, ok := f.binaryOperation(source, "+")
	if !ok {
		return ValueSource{}, false
	}
	var moduloSource ValueSource
	if constant, ok := integerConstant(plus.Right(), resolveInteger); ok && constant == 1 {
		moduloSource = plus.Left()
	} else if constant, ok := integerConstant(plus.Left(), resolveInteger); ok && constant == 1 {
		moduloSource = plus.Right()
	} else {
		return ValueSource{}, false
	}
	modulo, ok := f.binaryOperation(moduloSource, "%")
	if !ok || !f.sourceIsLengthOfPath(modulo.Right(), array) {
		return ValueSource{}, false
	}
	return modulo.Left(), true
}

func (f Facts) binaryOperation(source ValueSource, operator string) (ExpressionOperation, bool) {
	if source.Kind != ValueSourceExpression || !source.HasExpr {
		return ExpressionOperation{}, false
	}
	operation, ok := f.ExpressionOperation(source.ExprRef)
	return operation, ok && operation.Kind() == ExpressionOperationBinary && operation.Op() == operator
}

func (f Facts) sourceIsLengthOfPath(source ValueSource, want pathdom.Path) bool {
	if source.Kind != ValueSourceExpression || !source.HasExpr {
		return false
	}
	operation, ok := f.ExpressionOperation(source.ExprRef)
	if !ok || operation.Kind() != ExpressionOperationUnary || operation.Op() != "#" {
		return false
	}
	got, ok := f.sourcePath(operation.Left())
	return ok && got.Equal(want)
}

func (f Facts) sourcePath(source ValueSource) (pathdom.Path, bool) {
	if source.Kind == ValueSourceExpression && source.HasExpr {
		return f.ExpressionPathRef(source.ExprRef)
	}
	if source.Kind != ValueSourcePath || source.PathKey == "" {
		return pathdom.Path{}, false
	}
	if path, ok := pathaddr.LocalPathFromKey(source.PathKey); ok {
		return path, true
	}
	if sym, version, suffix, ok := pathaddr.ParseResolverPath(source.PathKey); ok {
		segments, segmentsOK := segment.ParseFormattedSegments(suffix)
		if !segmentsOK {
			return pathdom.Path{}, false
		}
		return pathdom.Path{Symbol: sym, Version: version, Segments: segments}, true
	}
	if stable, ok := pathaddr.StableFromKey(source.PathKey); ok {
		return stable.Path()
	}
	if sym, segments, ok := pathaddr.ParseSymbolPathKey(source.PathKey); ok {
		return pathdom.Path{Symbol: sym, Segments: segments}, true
	}
	return pathdom.Path{}, false
}

func integerConstant(source ValueSource, resolve IntegerConstantResolver) (int64, bool) {
	if source.Kind == ValueSourceLiteral && source.LiteralKind == ValueSourceLiteralInteger {
		return source.Int, true
	}
	if resolve == nil {
		return 0, false
	}
	return resolve(source)
}
