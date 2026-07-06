package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestChannelIdentityDiscriminantNarrowsPayloadAtAPILevel(t *testing.T) {
	src := strings.TrimLeft(`
type Event = {kind: "event", id: string}
type Tick = {kind: "tick", elapsed: number}

local function consume(events_ch: Channel<Event>, ticks_ch: Channel<Tick>): string
    local result = channel.select {
        events_ch:case_receive(),
        ticks_ch:case_receive(),
    }

    if result.channel == events_ch then
        local id: string = result.value.id
        local wrong_tick: Tick = result.value
        return id
    end

    local elapsed: number = result.value.elapsed
    return tostring(elapsed)
end
`, "\n")

	result := Check(src, WithStdlib(), WithManifest("channel", ChannelManifest()), WithGlobals("channel"))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            12,
		Column:          34,
		MessageContains: []string{
			"cannot assign result.value",
			"not Tick",
		},
		EvidenceMin: 2,
		EvidenceContains: []string{
			"result.value",
			"wrong_tick is declared as Tick",
		},
		LabelContains: []string{"assigned value", "declared type"},
		HelpContains:  []string{"Use a value compatible with the expected type"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderContains: []string{
			"error[type.assignment]",
			"local wrong_tick: Tick = result.value",
			"declared type",
		},
		RenderNotContains: []string{
			"number | string",
			"^~",
		},
	})
}
