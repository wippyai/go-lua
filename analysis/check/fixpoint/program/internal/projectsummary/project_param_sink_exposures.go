package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

type symbolKindReader interface {
	SymbolKind(symbol.ID) (symbol.Kind, bool)
}

type functionCaptureReader interface {
	Function() *ast.FunctionExpr
	DirectCaptures(*ast.FunctionExpr) []bind.Capture
}

type covariantExposureReader interface {
	CovariantExposures(cfg.Point) []factflow.CovariantExposure
}

// projectParamSinkExposures lowers each normal-return-reachable param-to-sink
// member store (sink.member = src, where sink is a persistent captured upvalue or
// global and src resolves to a parameter) into a ParamSinkExposure. The callee
// retains the source parameter object in a sink slot the caller cannot track
// writes back through, so a later write through that sink launders a wider value
// back into the source argument. The carried contract is the sink slot's type,
// reused from the in-body covariant-exposure fact the store already produced
// (addStoreExposure computes the slot type from the sink container's declared
// type): the slot type, not the parameter's declared type, is the real exposure
// type, since a covariant store of a narrow parameter into a wider sink slot is
// well-typed.
func projectParamSinkExposures(reg *axis.Registry, result ResultReader, exit state.State) []summary.ParamSinkExposure {
	if reg == nil {
		return nil
	}
	params := parameterValuePaths(result)
	if len(params) == 0 {
		return nil
	}
	reader, ok := result.(ordinaryAssignmentReader)
	if !ok {
		return nil
	}
	pathReader, ok := result.(expressionPathReader)
	if !ok {
		return nil
	}
	exposureReader, _ := result.(covariantExposureReader)
	kindReader, ok := result.(symbolKindReader)
	if !ok {
		return nil
	}
	captureReader, ok := result.(functionCaptureReader)
	if !ok {
		return nil
	}
	var entry state.State
	hasEntry := false
	if entryReader, ok := result.(entryStateReader); ok {
		entry, hasEntry = entryReader.EntryState()
	}
	captured := capturedSinkSymbols(captureReader)
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	noNormal, _ := result.(noNormalReturnReader)
	var out []summary.ParamSinkExposure
	for _, point := range graph.RPO() {
		if noNormal != nil && noNormal.NoNormalReturn(point) {
			continue
		}
		fact, ok := reader.OrdinaryAssignment(point)
		if !ok || !fact.HasPath || fact.Path.Symbol == 0 || len(fact.Path.Segments) == 0 {
			continue
		}
		if !persistentSinkSymbol(kindReader, captured, fact.Path.Symbol) {
			continue
		}
		source, ok := assignmentSourceParameterPlaceholder(fact, pathReader, params)
		if !ok {
			continue
		}
		contract, ok := sinkExposureContract(reg, exposureReader, params, point, source, entry, hasEntry)
		if !ok {
			continue
		}
		sourceKey, ok := pathaddr.RootPlaceholderKeyFromPath(source)
		if !ok {
			continue
		}
		out = append(out, summary.ParamSinkExposure{
			Source:   sourceKey,
			Contract: contract,
		})
	}
	return out
}

// sinkExposureContract resolves the sink slot's contract value for a param-to-sink
// store. When the store strictly widens the source parameter, addStoreExposure
// already recorded an in-body covariant exposure carrying the wider slot type;
// that exposure is the precise slot contract. When the store does not strictly
// widen the parameter, the slot type equals the parameter's own declared type, so
// the parameter's entry value is the contract. Either way the carried contract is
// the sink slot type, never an under-widening fallback.
func sinkExposureContract(
	reg *axis.Registry,
	exposureReader covariantExposureReader,
	params []pathdom.Path,
	point cfg.Point,
	source pathdom.Path,
	entry state.State,
	hasEntry bool,
) (product.Value, bool) {
	if exposureReader != nil {
		for _, exposure := range exposureReader.CovariantExposures(point) {
			exposurePath := exposure.SourcePath()
			if exposurePath.Symbol == 0 || len(exposurePath.Segments) != 0 {
				continue
			}
			placeholder, ok := parameterPlaceholderPath(exposurePath, params)
			if !ok || !placeholder.Equal(source) {
				continue
			}
			return exposure.WideValue(), true
		}
	}
	if !hasEntry {
		return product.Value{}, false
	}
	index := source.PlaceholderIndex()
	if index < 0 || index >= len(params) || params[index].Symbol == 0 {
		return product.Value{}, false
	}
	value := entry.ReadValue(reg, key.SymbolValue(params[index].Symbol))
	if product.Equal(reg, value, product.Bottom(reg)) || product.Equal(reg, value, product.Top()) {
		return product.Value{}, false
	}
	return value, true
}

// capturedSinkSymbols collects the symbols the current function captures from an
// enclosing scope. A captured symbol names a persistent object the caller still
// holds after the call returns, so a store into one of its slots outlives the
// call. The binder records a captured local under its declaration kind (Local),
// so the capture set, not the symbol kind, identifies upvalue sinks.
func capturedSinkSymbols(captureReader functionCaptureReader) map[symbol.ID]struct{} {
	fn := captureReader.Function()
	if fn == nil {
		return nil
	}
	captures := captureReader.DirectCaptures(fn)
	if len(captures) == 0 {
		return nil
	}
	out := make(map[symbol.ID]struct{}, len(captures))
	for _, capture := range captures {
		if capture.Captured != 0 {
			out[capture.Captured] = struct{}{}
		}
	}
	return out
}

// persistentSinkSymbol reports whether id names a sink whose lifetime outlives the
// call: a captured upvalue (held by the caller across the call) or a global. A
// callee-local sink does not escape, so a store into it cannot launder a value
// back into the argument after the call.
func persistentSinkSymbol(kindReader symbolKindReader, captured map[symbol.ID]struct{}, id symbol.ID) bool {
	if _, ok := captured[id]; ok {
		return true
	}
	kind, ok := kindReader.SymbolKind(id)
	if !ok {
		return false
	}
	return kind == symbol.Upvalue || kind == symbol.Global
}
