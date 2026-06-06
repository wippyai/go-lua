package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func TestRegression_ProcessListenTypedPayloadSelectCorrelation(t *testing.T) {
	chManifest := testutil.ChannelManifest()
	channelGen, _ := chManifest.LookupType("Channel")
	channelGeneric := channelGen.(*typ.Generic)

	messageType := typ.NewInterface("process.Message", []typ.Method{
		{Name: "topic", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
	timerType := typ.NewInterface("time.TimerTick", []typ.Method{
		{Name: "elapsed", Type: typ.Func().Param("self", typ.Self).Returns(typ.Number).Build()},
	})

	listenSpec := contract.NewSpec().
		WithReturnCase(
			constraint.FromConjunction(constraint.NewConjunction(constraint.FieldEquals{
				Target: constraint.Path{Root: "$1"},
				Field:  "message",
				Value:  typ.LiteralBool(true),
			})),
			typ.Instantiate(channelGeneric, messageType),
		).
		WithEffectRow(effect.Returns(0, effect.TypeProjection{
			Source: effect.ParamRef{Index: 1},
			Steps: []effect.TypeProjectionStep{
				effect.ProjectField("type"),
				effect.ProjectInstantiateGeneric(channelGeneric),
			},
		}))

	processModule := typ.NewInterface("process", []typ.Method{
		{Name: "listen", Type: typ.Func().
			Param("topic", typ.String).
			OptParam("opts", typ.Any).
			Returns(typ.Instantiate(channelGeneric, typ.Any)).
			Spec(listenSpec).
			Build()},
		{Name: "timer", Type: typ.Func().Returns(typ.Instantiate(channelGeneric, timerType)).Build()},
	})
	processManifest := io.NewManifest("process")
	processManifest.SetExport(processModule)
	processManifest.DefineType("Message", messageType)

	source := `
		type CleanRecord = {
			source: string,
			value: number,
		}

		local function accumulate(rec: CleanRecord): number
			return rec.value
		end

		local records = process.listen("record", { type = CleanRecord })
		local flushes = process.listen("flush", { message = true })
		local flush_at = process.timer()
		local direct_flush, direct_ok = flushes:receive()
		local direct_topic: string = direct_flush:topic()

		local selected = channel.select({
			records:case_receive(),
			flushes:case_receive(),
			flush_at:case_receive(),
		})

		if selected.channel == records then
			accumulate(selected.value)
			local source: string = selected.value.source
		elseif selected.channel == flushes then
			local topic: string = selected.value:topic()
		elseif selected.channel == flush_at then
			local elapsed: number = selected.value:elapsed()
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(),
		testutil.WithManifest("channel", chManifest),
		testutil.WithManifest("process", processManifest),
	)
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected only annotated errors for typed listen/select correlation")
	}
}
