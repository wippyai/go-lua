package program

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/domain/type/normalize"
	"github.com/wippyai/go-lua/analysis/domain/type/subtype"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

type canonicalConcatExecution struct {
	registry     *axis.Registry
	request      *body.Result
	summary      summary.Summary
	fullURL      symbol.ID
	fullURLPoint cfg.Point
}

func solveCanonicalExportedFieldConcat(t *testing.T, source string) canonicalConcatExecution {
	t.Helper()
	reg := standard.Registry()
	stmts := parseRelationProgramInputChunk(t, source)
	bindings := bind.BindChunk(stmts, bind.Options{})
	rootSummaryKey := rootKey(summary.SummaryKey{})
	keys := collectKeys(bindings, rootSummaryKey, reg, nil, body.Config{}.ModuleExports, stmts)
	check := body.Config{Registry: reg, Context: context.Background()}
	prepared, err := prepareBoundChunkBodies(stmts, bindings, check, keys)
	if err != nil {
		t.Fatal(err)
	}
	published, err := runPreparedRelationProgram(check.Context, prepared, prepared.root, check, keys, []transformer.ObservationContract{SummaryProjectionObservationContract()}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	requestFn := findFunctionForPath(t, bindings, stmts, "client.request")
	requestStatic := prepared.function(requestFn)
	if requestStatic == nil {
		t.Fatal("client.request has no prepared lexical body")
	}
	requestBody := requestStatic.StableLexicalBodyID()
	request := published.results[requestBody]
	if request == nil {
		t.Fatal("client.request has no canonical lexical result")
	}

	var fullURL symbol.ID
	var fullURLPoint cfg.Point
	for _, point := range request.Graph().RPO() {
		assignment, ok := request.LocalAssignment(point)
		if !ok || assignment.Name != "full_url" || !assignment.HasSymbol || assignment.Symbol == 0 {
			continue
		}
		if fullURL != 0 {
			t.Fatal("client.request has multiple full_url assignments")
		}
		fullURL, fullURLPoint = assignment.Symbol, point
	}
	if fullURL == 0 || fullURLPoint == 0 {
		t.Fatal("client.request full_url boundary point is absent")
	}

	requestKey, ok := keys.summaryKeyForFunction(requestFn)
	if !ok {
		t.Fatal("client.request has no public summary identity")
	}
	requestSummary, ok := published.snapshot.Read(requestKey)
	if !ok {
		t.Fatal("client.request canonical summary is absent")
	}
	return canonicalConcatExecution{
		registry: reg, request: request, summary: requestSummary,
		fullURL: fullURL, fullURLPoint: fullURLPoint,
	}
}

func assertCanonicalConcatExecution(t *testing.T, solved canonicalConcatExecution) {
	t.Helper()
	registry := solved.registry
	if registry == nil {
		t.Fatal("canonical concat execution has no registry")
	}
	value, present := solved.request.SymbolValueAtBoundary(solved.fullURLPoint, solved.fullURL)
	valueType, typed := typevalue.TypeOf(registry, value)
	if !present || !typed || !subtype.IsSubtype(valueType, typ.String) {
		t.Fatalf("full_url canonical boundary type = %v/%t (present=%t), want string", valueType, typed, present)
	}

	wantType := normalize.UnionForEvidence(typ.String, typ.Number)
	wantValue := typevalue.WithWitness(registry, typevalue.FromType(registry, wantType), wantType)
	diagnostics := solved.request.DiagnosticOutput()
	var exact []callpayload.CallParamObligation
	for _, obligation := range diagnostics.ParamObligations {
		if obligation.ParamIndex == 0 && product.Equal(registry, obligation.Value, wantValue) {
			exact = append(exact, obligation)
		}
	}
	if len(exact) != 1 {
		t.Fatalf("client.request canonical diagnostics = %#v, want one endpoint string|number obligation", diagnostics.ParamObligations)
	}
	if exact[0].SignatureSurface || exact[0].Origin.HasOrigin {
		t.Fatalf("concat obligation provenance = %#v, want exact body-operation parameter provenance", exact[0])
	}
	if len(solved.summary.ParamObligations) == 0 || !product.Equal(registry, solved.summary.ParamObligations[0], wantValue) {
		t.Fatalf("client.request summary obligations = %#v, want endpoint string|number at slot 0", solved.summary.ParamObligations)
	}
	if !solved.request.HasBodyOwnedParamObligations() {
		t.Fatal("canonical application lost its exact body-owned parameter-obligation marker")
	}
}

func TestRelationProgramExportedFieldFunctionUsesBodyConcatObligation(t *testing.T) {
	solved := solveCanonicalExportedFieldConcat(t, `
type HTTP = {
    get: (url: string, options: table) -> ()
}

local http: HTTP = {
    get = function(url: string, options: table): () end
}

local client = {}

function client.request(endpoint_path): ()
    local base_url = "https://api.example.test"
    local full_url = base_url .. endpoint_path
    local function send_once()
        return http.get(full_url, {})
    end
    send_once()
end

return client
`)
	assertCanonicalConcatExecution(t, solved)
}

func TestRelationProgramExportedFieldFunctionUsesProjectedFieldConcatObligation(t *testing.T) {
	solved := solveCanonicalExportedFieldConcat(t, `
type HTTP = {
    get: (url: string, options: table) -> ()
}
type Config = {
    base_url: string,
    headers: any,
}

local http: HTTP = {
    get = function(url: string, options: table): () end
}

local client = {}
client._http_client = http

local function resolve_config(): Config
    return {
        base_url = "https://api.example.test",
        headers = client.headers,
    }
end

function client.request(endpoint_path)
    local config = resolve_config()
    local full_url = config.base_url .. endpoint_path
    return client._http_client.get(full_url, {})
end

return client
`)
	assertCanonicalConcatExecution(t, solved)
}
