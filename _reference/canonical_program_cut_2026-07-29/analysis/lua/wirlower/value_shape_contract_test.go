package wirlower_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/wir"
)

func TestParenthesizedFinalCallsStayAdjustedAcrossValueLists(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		consumerOp wir.Op
	}{
		{name: "return", source: `return (f())`, consumerOp: wir.OpReturn},
		{name: "call argument", source: `g((f()))`, consumerOp: wir.OpCall},
		{name: "table tail", source: `local values = {(f())}`, consumerOp: wir.OpMakeTable},
		{name: "generic for", source: `for a, b in (f()) do end`, consumerOp: wir.OpIterate},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := lowerBody(t, test.source, "f", "g")
			calls := instructionsWithOp(body, wir.OpCall)
			if len(calls) == 0 {
				t.Fatal("missing call instruction")
			}
			producer := calls[0]
			if !producer.CallFinal || !producer.CallAdjusted || producer.CallExpanded || producer.CallOpenTail || producer.ResultSpread {
				t.Fatalf(
					"parenthesized producer shape = final:%v adjusted:%v expanded:%v open:%v spread:%v, want adjusted scalar",
					producer.CallFinal, producer.CallAdjusted, producer.CallExpanded, producer.CallOpenTail, producer.ResultSpread,
				)
			}

			consumers := instructionsWithOp(body, test.consumerOp)
			if test.consumerOp == wir.OpCall {
				if len(consumers) != 2 {
					t.Fatalf("call instructions = %d, want inner producer and outer consumer", len(consumers))
				}
				consumers = consumers[1:]
			}
			if len(consumers) != 1 {
				t.Fatalf("%v consumers = %d, want one", test.consumerOp, len(consumers))
			}
			if consumers[0].ListSpread {
				t.Fatalf("parenthesized %s consumer has open list spread", test.name)
			}
		})
	}
}

func TestParenthesizedAssignmentProducesOneResultAndNilFillsRemainder(t *testing.T) {
	body := lowerBody(t, `local first, second = (f())`, "f")
	calls := instructionsWithOp(body, wir.OpCall)
	if len(calls) != 1 {
		t.Fatalf("call instructions = %d, want one", len(calls))
	}
	call := calls[0]
	if got := len(body.Operands(call.Results)); got != 1 {
		t.Fatalf("parenthesized call result destinations = %d, want one", got)
	}
	if !call.CallAdjusted || call.CallExpanded || call.ResultSpread {
		t.Fatalf("parenthesized assignment call = adjusted:%v expanded:%v spread:%v, want adjusted scalar", call.CallAdjusted, call.CallExpanded, call.ResultSpread)
	}

	assigns := instructionsWithOp(body, wir.OpAssign)
	if len(assigns) != 2 {
		t.Fatalf("assignment instructions = %d, want two", len(assigns))
	}
	second := assigns[1].A
	if second.Kind != wir.OperandConst || body.Const(wir.ConstRef(second.Ref)).Kind != wir.ConstNil {
		t.Fatalf("second assignment source = %#v, want nil fill", second)
	}
}

func TestParenthesizedVarargAssignmentNilFillsRemainder(t *testing.T) {
	body := lowerBody(t, `local first, second = (...)`)
	assigns := instructionsWithOp(body, wir.OpAssign)
	if len(assigns) != 2 {
		t.Fatalf("assignment instructions = %d, want two", len(assigns))
	}
	if assigns[0].A.Kind != wir.OperandVararg {
		t.Fatalf("first assignment source = %#v, want adjusted vararg", assigns[0].A)
	}
	second := assigns[1].A
	if second.Kind != wir.OperandConst || body.Const(wir.ConstRef(second.Ref)).Kind != wir.ConstNil {
		t.Fatalf("second assignment source = %#v, want nil fill", second)
	}
}

func TestParenthesizedFinalVarargsDoNotOpenValueLists(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		consumerOp wir.Op
	}{
		{name: "return", source: `return (...)`, consumerOp: wir.OpReturn},
		{name: "call argument", source: `g((...))`, consumerOp: wir.OpCall},
		{name: "table tail", source: `local values = {(...)}`, consumerOp: wir.OpMakeTable},
		{name: "generic for", source: `for a, b in (...) do end`, consumerOp: wir.OpIterate},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := lowerBody(t, test.source, "g")
			consumers := instructionsWithOp(body, test.consumerOp)
			if len(consumers) != 1 {
				t.Fatalf("%v consumers = %d, want one", test.consumerOp, len(consumers))
			}
			if consumers[0].ListSpread {
				t.Fatalf("parenthesized vararg %s consumer has open list spread", test.name)
			}
		})
	}
}

func TestTailArgumentSpreadSynchronizesProducerCallShape(t *testing.T) {
	body := lowerBody(t, `g(f())`, "f", "g")
	calls := instructionsWithOp(body, wir.OpCall)
	if len(calls) != 2 {
		t.Fatalf("call instructions = %d, want inner producer and outer consumer", len(calls))
	}
	producer, consumer := calls[0], calls[1]
	if !producer.ResultSpread || !producer.CallFinal || !producer.CallExpanded || producer.CallAdjusted || producer.CallOpenTail {
		t.Fatalf(
			"tail producer shape = final:%v adjusted:%v expanded:%v open:%v spread:%v, want expanded non-open tail",
			producer.CallFinal, producer.CallAdjusted, producer.CallExpanded, producer.CallOpenTail, producer.ResultSpread,
		)
	}
	if !consumer.ListSpread {
		t.Fatal("outer call does not consume the tail producer as an open argument list")
	}
}

func instructionsWithOp(body *wir.Body, op wir.Op) []wir.Instruction {
	if body == nil {
		return nil
	}
	out := make([]wir.Instruction, 0, 2)
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		if inst.Op == op {
			out = append(out, inst)
		}
	}
	return out
}
