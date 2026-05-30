package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/erreffect"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// TestZZErrRetProbe resolves the callee fn type via the standard checker export
// and reports whether its contract spec carries an ErrorReturn label. Debug probe.
func TestZZErrRetProbe(t *testing.T) {
	cases := map[string]string{
		"declared-getData": `
local function getData(): (string?, string?)
    return "data", nil
end
return getData
`,
		"declared-process": `
local function process(x: number): (number?, string?)
    if x < 0 then
        return nil, "negative"
    end
    return x * 2, nil
end
return process
`,
		"inferred-load_page": `
type Page = { data_func: string? }
local function load_page(): (Page?, string?)
    return { data_func = "demo" }, nil
end
return load_page
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			res := testutil.CheckAndExport(src, "m", testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
			ft := unwrap.Function(res.Manifest.EnrichedExport())
			if ft == nil {
				t.Logf("%s: export not a function: %v", name, res.Manifest.EnrichedExport())
				return
			}
			t.Logf("%s: returns=%v hasErrRet=%v", name, ft.Returns, erreffect.HasErrorReturnLabel(ft))
			if sp := contract.ExtractSpec(ft); sp != nil {
				t.Logf("  spec.effects.labels=%v", sp.Effects.Labels)
			}
			_ = typ.Nil
		})
	}
}

// TestZZErrRetCrossModule checks whether the exported manifest type of a module
// function carries the inferred ErrorReturn label. Debug probe.
func TestZZErrRetCrossModule(t *testing.T) {
	client := `
local M = {}
type Response = { metadata: { response_id: string } }
function M.request(ok: boolean): (Response?, string?)
    if ok then
        return { metadata = { response_id = "resp-123" } }, nil
    end
    return nil, "failed"
end
return M
`
	res := testutil.CheckAndExport(client, "client", testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	exp := res.Manifest.EnrichedExport()
	t.Logf("client export: %v", exp)
	rec := unwrap.Record(exp)
	if rec == nil {
		t.Logf("not a record")
		return
	}
	for _, f := range rec.Fields {
		if f.Name != "request" {
			continue
		}
		ft := unwrap.Function(f.Type)
		if ft == nil {
			t.Logf("request not a function: %v", f.Type)
			return
		}
		t.Logf("request: returns=%v hasErrRet=%v", ft.Returns, erreffect.HasErrorReturnLabel(ft))
		if sp := contract.ExtractSpec(ft); sp != nil {
			t.Logf("  spec.effects.labels=%v", sp.Effects.Labels)
		}
	}
}

// TestZZErrReturnMultiple drives the error-return-multiple fixture. Debug probe.
func TestZZErrReturnMultiple(t *testing.T) {
	src := `
local function process(x: number): (number?, string?)
    if x < 0 then
        return nil, "negative"
    end
    return x * 2, nil
end
local result, err = process(5)
if err ~= nil then
    return
end
local n: number = result
`
	res := testutil.CheckAndExport(src, "m", testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	for _, m := range testutil.ErrorMessages(res.Errors) {
		t.Logf("DIAG: %s", m)
	}
}

// TestZZActiveSession drives the record-field-unknown fixture. Debug probe.
func TestZZActiveSession(t *testing.T) {
	src := `
local time = require("time")
type ActiveSession = {
    created_at: time.Time,
    last_activity: time.Time?,
}
local M = {}
function M.new(): ActiveSession
    local now = time.now()
    return {
        created_at = now,
        last_activity = now,
    }
end
return M
`
	res := testutil.CheckAndExport(src, "session_state", testutil.WithStdlib(), testutil.WithManifest("time", zzTimeManifest()), testutil.WithCheckOption(check.WithCanonicalFlow()))
	for _, m := range testutil.ErrorMessages(res.Errors) {
		t.Logf("DIAG: %s", m)
	}
}

// zzTimeManifest mirrors the fixture time package manifest. Debug probe helper.
func zzTimeManifest() *io.Manifest {
	m := io.NewManifest("time")
	durationType := typ.NewInterface("time.Duration", []typ.Method{
		{Name: "seconds", Type: typ.Func().Param("self", typ.Self).Returns(typ.Number).Build()},
	})
	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "sub", Type: typ.Func().Param("self", typ.Self).Param("t", typ.Self).Returns(durationType).Build()},
	})
	m.DefineType("Time", timeType)
	m.DefineType("Duration", durationType)
	moduleType := typ.NewInterface("time", []typ.Method{
		{Name: "now", Type: typ.Func().Returns(timeType).Build()},
	})
	m.SetExport(moduleType)
	return m
}

// TestZZTimeNow probes the return type of time.now() in canonical. Debug probe.
func TestZZTimeNow(t *testing.T) {
	src := `
local time = require("time")
local now = time.now()
local x: number = now
`
	res := testutil.CheckAndExport(src, "m", testutil.WithStdlib(), testutil.WithManifest("time", zzTimeManifest()), testutil.WithCheckOption(check.WithCanonicalFlow()))
	for _, m := range testutil.ErrorMessages(res.Errors) {
		t.Logf("DIAG: %s", m)
	}
}

// TestZZRequireTime probes whether the time module export resolves. Debug probe.
func TestZZRequireTime(t *testing.T) {
	src := `
local time = require("time")
local n = time.now
local x: number = n
`
	res := testutil.CheckAndExport(src, "m", testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	for _, m := range testutil.ErrorMessages(res.Errors) {
		t.Logf("DIAG: %s", m)
	}
}
