package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/erreffect"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestErrorReturnCrossModuleSummaryRegression(t *testing.T) {
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
	rec := unwrap.Record(res.Manifest.EnrichedExport())
	if rec == nil {
		t.Fatalf("expected record export, got %v", res.Manifest.EnrichedExport())
	}
	field := rec.GetField("request")
	if field == nil {
		t.Fatalf("expected request field in %v", rec)
	}
	ft := unwrap.Function(field.Type)
	if ft == nil {
		t.Fatalf("expected request function, got %v", field.Type)
	}
	if !erreffect.HasErrorReturnLabel(ft) {
		t.Fatalf("expected request summary to carry error-return label, got returns %v", ft.Returns)
	}
}

func TestErrorReturnMultipleNarrowingRegression(t *testing.T) {
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
	requireCanonicalClean(t, src)
}

func TestTimeManifestResolutionRegression(t *testing.T) {
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
	res := testutil.CheckAndExport(src, "session_state", testutil.WithStdlib(), testutil.WithManifest("time", canonicalTimeManifest()), testutil.WithCheckOption(check.WithCanonicalFlow()))
	if msgs := testutil.ErrorMessages(res.Errors); len(msgs) != 0 {
		t.Fatalf("expected time manifest session state to be clean, got diagnostics: %v", msgs)
	}
}

func TestTimeNowManifestTypeRegression(t *testing.T) {
	src := `
local time = require("time")
local now = time.now()
local x: number = now
`
	res := testutil.CheckAndExport(src, "m", testutil.WithStdlib(), testutil.WithManifest("time", canonicalTimeManifest()), testutil.WithCheckOption(check.WithCanonicalFlow()))
	msgs := testutil.ErrorMessages(res.Errors)
	for _, msg := range msgs {
		if msg == "cannot assign time.Time to number" {
			return
		}
	}
	t.Fatalf("expected time.Time assignment diagnostic, got %v", msgs)
}

func canonicalTimeManifest() *io.Manifest {
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
