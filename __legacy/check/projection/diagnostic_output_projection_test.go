package projection

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestProjectDiagnosticOutputConvertsCanonicalTupleWithoutSyntax(t *testing.T) {
	reg := standard.Registry()
	wantParam := typevalue.String(reg)
	wantPath := typevalue.LiteralNumber(reg, 42)
	wantContract := typevalue.LiteralString(reg, "contract")
	capturedPath := pathdom.Path{Symbol: symbol.ID(77)}.Field("value")
	output := callpayload.DiagnosticOutput{
		SuspensionKnown:  true,
		ParamObligations: []callpayload.CallParamObligation{{ParamIndex: 1, Value: wantParam}},
		PathObligations:  []callpayload.CallPathObligation{{Path: capturedPath, Value: wantPath}},
		ParamExposures: []callpayload.CallParamExposure{{
			Source: pathdom.NewPlaceholder(0), Contract: wantContract, Kind: factflow.CovariantExposureArray,
		}},
	}

	params, captured, exposures, err := projectDiagnosticOutput(reg, output, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(params) != 2 || !product.Equal(reg, params[0], product.Top()) || !product.Equal(reg, params[1], wantParam) {
		t.Fatalf("parameter obligations = %#v, want Top/string", params)
	}
	stable, _ := pathaddr.StableOfPath(capturedPath)
	if len(captured) != 1 || captured[0].Path != stable.StableKey() || !product.Equal(reg, captured[0].Value, wantPath) {
		t.Fatalf("captured path obligations = %#v, want exact stable path/value", captured)
	}
	source, _ := pathaddr.RootPlaceholderKeyFromPath(pathdom.NewPlaceholder(0))
	if len(exposures) != 1 || exposures[0].Source != source || !product.Equal(reg, exposures[0].Contract, wantContract) {
		t.Fatalf("parameter exposures = %#v, want exact source/contract", exposures)
	}
}

func TestProjectDiagnosticOutputRejectsUnrepresentableExposureSource(t *testing.T) {
	reg := standard.Registry()
	_, _, _, err := projectDiagnosticOutput(reg, callpayload.DiagnosticOutput{
		ParamExposures: []callpayload.CallParamExposure{{
			Source:   pathdom.NewPlaceholder(0).Field("child"),
			Contract: typevalue.LiteralString(reg, "contract"),
			Kind:     factflow.CovariantExposureRecord,
		}},
	}, 1)
	if err == nil {
		t.Fatal("member exposure silently lost through root-only summary carrier")
	}
}
