package effectlowering

import (
	"sync"
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func testPrepareCallOutcome(t *testing.T, program callpayload.CallOutcomeProgram, ctx transfer.NodeContext, site factflow.CallSiteView) callpayload.CallOutcomeSiteProgram {
	t.Helper()
	prepared, err := program.PrepareSite(ctx, site)
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}

func testEvaluateCallOutcome(t *testing.T, program callpayload.CallOutcomeProgram, ctx transfer.NodeContext, site factflow.CallSiteView, input callpayload.CallOutcomeInput) callpayload.CallOutcome {
	t.Helper()
	prepared := testPrepareCallOutcome(t, program, ctx, site)
	outcome, err := prepared.Evaluate(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	return outcome
}

type testCallOutcomeProducer struct {
	facts   factflow.Facts
	sources sourcevalue.SourceValues
	keys    *keyspace.KeySpace
}

var testCallOutcomeProducerRegistry sync.Map // map[*testing.T]testCallOutcomeProducer

func updateTestCallOutcomeProducer(t *testing.T, update func(*testCallOutcomeProducer)) {
	t.Helper()
	producer := testCallOutcomeProducer{}
	if raw, ok := testCallOutcomeProducerRegistry.Load(t); ok {
		producer = raw.(testCallOutcomeProducer)
	}
	update(&producer)
	testCallOutcomeProducerRegistry.Store(t, producer)
	t.Cleanup(func() { testCallOutcomeProducerRegistry.Delete(t) })
}

func registerTestSourceValues(t *testing.T, config sourcevalue.SourceValuesConfig) sourcevalue.SourceValues {
	t.Helper()
	sources := sourcevalue.NewSourceValues(config)
	registerTestSourceResolver(t, sources)
	return sources

}

func registerTestSourceResolver(t *testing.T, sources sourcevalue.SourceValues) {
	t.Helper()
	updateTestCallOutcomeProducer(t, func(producer *testCallOutcomeProducer) {
		producer.sources = sources
	})
}

func testSignatureOutcomeProvider(t *testing.T, config SignatureOutcomeProviderConfig) callpayload.CallOutcomeProgram {
	t.Helper()
	if config.KeySpace == nil {
		config.KeySpace = keyspace.New()
	}
	input, err := SealSignatureOutcomeOperands(state.RegisteredProductDomain(standard.Registry()), config.KeySpace)
	if err != nil {
		t.Fatal(err)
	}
	config.InputProgram = input
	updateTestCallOutcomeProducer(t, func(producer *testCallOutcomeProducer) {
		producer.facts = config.Facts
		producer.keys = config.KeySpace
		if producer.sources == nil {
			expressionValues := make(map[factflow.ExprRef]product.Value)
			config.Facts.ForEachExpressionValue(func(ref factflow.ExprRef, value product.Value) bool {
				expressionValues[ref] = value
				return true
			})
			expressionPaths := make(map[factflow.ExprRef]struct{})
			config.Facts.ForEachExpressionPath(func(ref factflow.ExprRef, _ pathdom.Path) bool {
				expressionPaths[ref] = struct{}{}
				return true
			})
			producer.sources = sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
				Registry:         standard.Registry(),
				KeySpace:         config.KeySpace,
				ExpressionValues: expressionValues,
				ExpressionPaths:  expressionPaths,
			})
		}
	})
	return SignatureOutcomeProvider(config)
}

func testSignatureArgumentTypeProgram(t *testing.T, keys *keyspace.KeySpace, evaluate func(SignatureArgumentTypeContext) (typ.Type, bool)) SignatureArgumentTypeProgram {
	t.Helper()
	input, err := SealSignatureOutcomeOperands(state.RegisteredProductDomain(standard.Registry()), keys)
	if err != nil {
		t.Fatal(err)
	}
	program, err := SealSignatureArgumentTypeProgram(input, evaluate)
	if err != nil {
		t.Fatal(err)
	}
	return program
}

// testCallOutcomeInput exercises the same sealed concrete edge used by the
// relation executor. Tests name the input construction explicitly; no legacy
// State/read provider signature is retained.
func testCallOutcomeInput(
	t *testing.T,
	program callpayload.CallOutcomeProgram,
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	in state.State,
	read func(cfg.Point) state.State,
) callpayload.CallOutcomeInput {
	t.Helper()
	if ctx.Registry == nil {
		ctx.Registry = standard.Registry()
	}
	producer := testCallOutcomeProducer{}
	if raw, ok := testCallOutcomeProducerRegistry.Load(t); ok {
		producer = raw.(testCallOutcomeProducer)
	}
	producerSite := site
	if factSite, ok := producer.facts.CallSiteView(ctx.Point); ok {
		producerSite = factSite
	}
	domain := state.RegisteredProductDomain(ctx.Registry)
	point := ctx.Point
	if point == 0 {
		point = 1
	}
	capability := testPrepareCallOutcome(t, program, ctx, site).Capability()
	access, err := factapply.SealExternalCallTransferAccess(
		domain, []state.TransferInputAccess{{}}, []cfg.Point{point}, 0, capability, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	inputProgram, err := callpayload.PrepareExternalCallInputProgram(
		domain, access, []cfg.Point{point}, 0,
		func(root statekey.Value) (statekey.Value, bool) { return root, true },
	)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := callpayload.BindConcreteExternalCallInputFrame(
		&inputProgram, []state.State{in}, []callpayload.DiagnosticOutput{{}},
	)
	if err != nil {
		t.Fatal(err)
	}
	resolve := func(source factflow.ValueSource) (product.Value, bool) {
		if source.Kind == factflow.ValueSourceCall && source.HasCallPoint && source.ResultIndex >= 0 && read != nil {
			value := read(source.CallPoint).ReadValue(ctx.Registry, statekey.ReturnSlot(source.ResultIndex))
			if !product.Equal(ctx.Registry, value, product.Bottom(ctx.Registry)) {
				return value, true
			}
		}
		if producer.sources != nil {
			if value, ok := producer.sources.ValueOfSource(ctx.Point, source, in, read); ok {
				return value, true
			}
		}
		input := in
		if source.HasSourcePoint && read != nil {
			input = read(source.SourcePoint)
		}
		switch source.Kind {
		case factflow.ValueSourceExpression:
			if !source.HasExpr {
				return product.Value{}, false
			}
			value := input.ReadValue(ctx.Registry, statekey.ExpressionValue(uint32(source.ExprRef)))
			if !product.Equal(ctx.Registry, value, product.Bottom(ctx.Registry)) {
				return value, true
			}
			return product.Value{}, false
		case factflow.ValueSourceCall:
			if !source.HasCallPoint || source.ResultIndex < 0 {
				return product.Value{}, false
			}
			callInput := input
			if read != nil {
				callInput = read(source.CallPoint)
			}
			value := callInput.ReadValue(ctx.Registry, statekey.ReturnSlot(source.ResultIndex))
			if product.Equal(ctx.Registry, value, product.Bottom(ctx.Registry)) {
				value = callInput.ReadValue(ctx.Registry, statekey.CallResult(uint32(source.CallPoint), uint32(source.ResultIndex)))
			}
			return value, true
		case factflow.ValueSourcePath:
			keys := producer.keys
			if keys == nil {
				keys = keyspace.New()
			}
			var pathKey keyspace.Key
			var ok bool
			if sym, segments, parsed := pathaddr.ParseSymbolPathKey(source.PathKey); parsed {
				pathKey, ok = keys.FromStableSymbol(sym, segments)
			} else {
				pathKey, ok = keys.FromStateKey(source.PathKey)
			}
			if !ok {
				return product.Value{}, false
			}
			if pathKey.Sym != 0 && len(keys.Segments(pathKey)) == 0 {
				return input.ReadValue(ctx.Registry, statekey.SymbolValue(pathKey.Sym)), true
			}
			return input.ReadPathKey(ctx.Registry, keys, source.PathKey), true
		case factflow.ValueSourceNil:
			return typevalue.Nil(ctx.Registry), true
		case factflow.ValueSourceLiteral:
			switch source.LiteralKind {
			case factflow.ValueSourceLiteralBool:
				return typevalue.LiteralBool(ctx.Registry, source.Bool), true
			case factflow.ValueSourceLiteralInteger:
				return typevalue.LiteralInt(ctx.Registry, source.Int), true
			case factflow.ValueSourceLiteralNumber:
				return typevalue.LiteralNumber(ctx.Registry, source.Float), true
			case factflow.ValueSourceLiteralString:
				return typevalue.LiteralString(ctx.Registry, source.String), true
			}
		}
		return product.Value{}, false
	}
	operands := callpayload.CallOutcomeValueOperands{}
	if source, ok := producerSite.CalleeSource(); ok {
		operands.Callee, operands.HasCallee = resolve(source)
	}
	if source, ok := producerSite.ReceiverSource(); ok {
		operands.Receiver, operands.HasReceiver = resolve(source)
	} else if receiver, ok := producerSite.ReceiverPath(); ok && receiver.Symbol != 0 && len(receiver.Segments) == 0 {
		operands.Receiver = in.ReadValue(ctx.Registry, statekey.SymbolValue(receiver.Symbol))
		operands.HasReceiver = true
	}
	operands.Arguments = make([]callpayload.CallOutcomeArgumentOperand, producerSite.ArgumentSourceCount())
	producerSite.ForEachArgumentSource(func(index int, source factflow.ValueSource) bool {
		value, ok := resolve(source)
		operands.Arguments[index] = callpayload.CallOutcomeArgumentOperand{Value: value, Present: ok}
		return true
	})
	bound, err := frame.BindCallOutcomeInput(operands)
	if err != nil {
		t.Fatal(err)
	}
	return bound
}
