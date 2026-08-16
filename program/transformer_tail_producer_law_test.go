package program_test

import (
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/lower"
)

// TailProducer is only a catalog-backed receipt. Equivalent publication may
// replay the semantic ID, but it cannot cross the exact Program owner fence;
// repeated hot queries must remain allocation-free and must not reopen Flow.
func TestTransformerTailProducerIsSealedOwnerFencedAndHot(t *testing.T) {
	open, err := lower.Lower(lower.Source{Name: "transformer-tail-owner.lua", Text: []byte(`
local function forward(...)
  return sink(...)
end
return forward
`)})
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	input := open.TransformerInput()
	var producerKind uint8
	var producerContext string
	var producerSpanOK bool
	var producerAvailable bool
	var original program.TailProducer
	for index := 0; index < input.CallCount(); index++ {
		call, callOK := input.CallAt(index)
		if !callOK {
			continue
		}
		values, valuesOK := call.Values()
		if !valuesOK {
			continue
		}
		producer, openTail := values.Tail()
		if !openTail {
			continue
		}
		producerKind = uint8(producer.Kind())
		original = producer
		producerContext = producer.ContextID().String()
		_, producerSpanOK = producer.Span()
		producerAvailable = producer.Available() && input.OwnsTailProducer(producer)
		if !producerAvailable || producerKind == 0 || producerContext == "" || !producerSpanOK {
			t.Fatal("catalog-backed tail producer was unavailable or incomplete")
		}
		allocs := testing.AllocsPerRun(10000, func() {
			if !producer.Available() || producer.Kind() == 0 || producer.ContextID().String() == "" {
				t.Fatal("hot tail producer receipt failed")
			}
		})
		if allocs != 0 {
			t.Fatalf("hot tail producer queries allocated %g times per run", allocs)
		}
		break
	}
	if !producerAvailable || producerKind == 0 || producerContext == "" || !producerSpanOK {
		t.Fatal("fixture did not expose an open catalog-backed tail producer")
	}

	replay, err := lower.Lower(lower.Source{Name: "transformer-tail-owner.lua", Text: []byte(`
local function forward(...)
  return sink(...)
end
return forward
`)})
	if err != nil {
		t.Fatalf("Replay lower: %v", err)
	}
	replayInput := replay.TransformerInput()
	for index := 0; index < replayInput.CallCount(); index++ {
		call, callOK := replayInput.CallAt(index)
		if !callOK {
			continue
		}
		values, valuesOK := call.Values()
		if !valuesOK {
			continue
		}
		producer, openTail := values.Tail()
		if openTail {
			if producer.ContextID().String() != producerContext {
				t.Fatal("equivalent replay changed semantic tail ID")
			}
			if input.OwnsTailProducer(producer) || replayInput.OwnsTailProducer(original) {
				t.Fatal("tail producer crossed the exact Program owner fence")
			}
		}
	}
}
