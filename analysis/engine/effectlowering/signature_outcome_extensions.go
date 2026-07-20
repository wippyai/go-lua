package effectlowering

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// SignatureOutcomeSite is the immutable lexical/signature input used to
// select the exact non-scalar queries required at one call site.
type SignatureOutcomeSite struct {
	Site      factflow.CallSiteView
	Name      string
	Signature signature.Function
}

// SignatureArgumentTypeContext is the State-free input to one typed argument
// extension. Source is structural provenance only; Value is the canonical
// evaluated operand and Input grants exactly the extension's registered
// non-scalar queries.
type SignatureArgumentTypeContext struct {
	Node   transfer.NodeContext
	Site   factflow.CallSiteView
	Source factflow.ValueSource
	Index  int
	Formal typ.Type
	Value  product.Value
	Input  SignatureOutcomeInput
}

type signatureArgumentTypeExtension struct {
	input    SignatureOutcomeInputProgram
	evaluate func(SignatureArgumentTypeContext) (typ.Type, bool)
}

// SignatureArgumentTypeProgram is an ordered fail-closed extension chain.
// Each extension is bound against its own query program, so composing a
// stronger sibling cannot accidentally widen another extension's authority.
type SignatureArgumentTypeProgram struct {
	extensions []signatureArgumentTypeExtension
}

func SealSignatureArgumentTypeProgram(
	input SignatureOutcomeInputProgram,
	evaluate func(SignatureArgumentTypeContext) (typ.Type, bool),
) (SignatureArgumentTypeProgram, error) {
	if !input.Valid() || evaluate == nil {
		return SignatureArgumentTypeProgram{}, fmt.Errorf("effectlowering: invalid signature argument-type program")
	}
	return SignatureArgumentTypeProgram{extensions: []signatureArgumentTypeExtension{{input: input, evaluate: evaluate}}}, nil
}

func ComposeSignatureArgumentTypePrograms(programs ...SignatureArgumentTypeProgram) SignatureArgumentTypeProgram {
	var out SignatureArgumentTypeProgram
	for _, program := range programs {
		out.extensions = append(out.extensions, program.extensions...)
	}
	return out
}

func (p SignatureArgumentTypeProgram) Empty() bool { return len(p.extensions) == 0 }

func (p SignatureArgumentTypeProgram) maximumInputProgram() (SignatureOutcomeInputProgram, error) {
	inputs := make([]SignatureOutcomeInputProgram, 0, len(p.extensions))
	for _, extension := range p.extensions {
		inputs = append(inputs, extension.input)
	}
	return UnionSignatureOutcomeInputPrograms(inputs...)
}

// PreparedSignatureArgumentTypeProgram is the exact argument-type extension
// program required by one resolved signature. Ordinary signatures need no
// contextual argument query at all.
type PreparedSignatureArgumentTypeProgram struct {
	program SignatureArgumentTypeProgram
}

func (p SignatureArgumentTypeProgram) PrepareSite(site SignatureOutcomeSite) (PreparedSignatureArgumentTypeProgram, error) {
	if p.Empty() || !signatureNeedsContextualArgumentTypes(site.Signature) {
		return PreparedSignatureArgumentTypeProgram{}, nil
	}
	if _, err := p.maximumInputProgram(); err != nil {
		return PreparedSignatureArgumentTypeProgram{}, err
	}
	return PreparedSignatureArgumentTypeProgram{program: p}, nil
}

func (p PreparedSignatureArgumentTypeProgram) Empty() bool { return p.program.Empty() }

func (p PreparedSignatureArgumentTypeProgram) InputProgram() (SignatureOutcomeInputProgram, error) {
	return p.program.maximumInputProgram()
}

func (p SignatureArgumentTypeProgram) evaluate(
	base callpayload.CallOutcomeInput,
	ctx SignatureArgumentTypeContext,
) (typ.Type, bool, error) {
	for _, extension := range p.extensions {
		input, err := extension.input.Bind(base)
		if err != nil {
			return nil, false, err
		}
		ctx.Input = input
		if result, ok := extension.evaluate(ctx); ok {
			return result, true, nil
		}
	}
	return nil, false, nil
}

func (p PreparedSignatureArgumentTypeProgram) evaluate(
	base callpayload.CallOutcomeInput,
	ctx SignatureArgumentTypeContext,
) (typ.Type, bool, error) {
	return p.program.evaluate(base, ctx)
}

// SignatureReturnValueInputContext is the State-free input to a custom signature return
// extension. Arguments/receiver are read through Input; no source resolver or
// read(point) authority exists here.
type SignatureReturnValueInputContext struct {
	Node  transfer.NodeContext
	Site  factflow.CallSiteView
	Name  string
	Index int
	Input SignatureOutcomeInput
}

type signatureReturnExtension struct {
	input      SignatureOutcomeInputProgram
	selectSite func(SignatureOutcomeSite) bool
	evaluate   func(SignatureReturnValueInputContext) (product.Value, bool)
}

// SignatureReturnValueProgram is the typed custom-return extension. The
// current semantics have one extension, but the carrier is explicitly ordered
// so future composition cannot invent an unordered callback map.
type SignatureReturnValueProgram struct {
	extensions []signatureReturnExtension
}

func SealSignatureReturnValueProgram(
	input SignatureOutcomeInputProgram,
	selectSite func(SignatureOutcomeSite) bool,
	evaluate func(SignatureReturnValueInputContext) (product.Value, bool),
) (SignatureReturnValueProgram, error) {
	if !input.Valid() || selectSite == nil || evaluate == nil {
		return SignatureReturnValueProgram{}, fmt.Errorf("effectlowering: invalid signature return-value program")
	}
	return SignatureReturnValueProgram{extensions: []signatureReturnExtension{{input: input, selectSite: selectSite, evaluate: evaluate}}}, nil
}

func ComposeSignatureReturnValuePrograms(programs ...SignatureReturnValueProgram) SignatureReturnValueProgram {
	var out SignatureReturnValueProgram
	for _, program := range programs {
		out.extensions = append(out.extensions, program.extensions...)
	}
	return out
}

func (p SignatureReturnValueProgram) Empty() bool { return len(p.extensions) == 0 }

func (p SignatureReturnValueProgram) maximumInputProgram() (SignatureOutcomeInputProgram, error) {
	inputs := make([]SignatureOutcomeInputProgram, 0, len(p.extensions))
	for _, extension := range p.extensions {
		inputs = append(inputs, extension.input)
	}
	return UnionSignatureOutcomeInputPrograms(inputs...)
}

// PreparedSignatureReturnValueProgram contains only extensions applicable to
// one resolved call site. Its input program is therefore the exact query
// footprint of that site, rather than the union of all stdlib behavior.
type PreparedSignatureReturnValueProgram struct {
	extensions []signatureReturnExtension
}

func (p SignatureReturnValueProgram) PrepareSite(site SignatureOutcomeSite) (PreparedSignatureReturnValueProgram, error) {
	out := PreparedSignatureReturnValueProgram{extensions: make([]signatureReturnExtension, 0, len(p.extensions))}
	for _, extension := range p.extensions {
		if extension.selectSite(site) {
			out.extensions = append(out.extensions, extension)
		}
	}
	if _, err := out.InputProgram(); err != nil {
		return PreparedSignatureReturnValueProgram{}, err
	}
	return out, nil
}

func (p PreparedSignatureReturnValueProgram) Empty() bool { return len(p.extensions) == 0 }

func (p PreparedSignatureReturnValueProgram) InputProgram() (SignatureOutcomeInputProgram, error) {
	inputs := make([]SignatureOutcomeInputProgram, 0, len(p.extensions))
	for _, extension := range p.extensions {
		inputs = append(inputs, extension.input)
	}
	return UnionSignatureOutcomeInputPrograms(inputs...)
}

func (p SignatureReturnValueProgram) evaluate(
	base callpayload.CallOutcomeInput,
	ctx SignatureReturnValueInputContext,
) (product.Value, bool, error) {
	for _, extension := range p.extensions {
		input, err := extension.input.Bind(base)
		if err != nil {
			return product.Value{}, false, err
		}
		ctx.Input = input
		if result, ok := extension.evaluate(ctx); ok {
			return result, true, nil
		}
	}
	return product.Value{}, false, nil
}

func (p PreparedSignatureReturnValueProgram) evaluate(
	base callpayload.CallOutcomeInput,
	ctx SignatureReturnValueInputContext,
) (product.Value, bool, error) {
	return SignatureReturnValueProgram{extensions: p.extensions}.evaluate(base, ctx)
}
