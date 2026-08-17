package lower

import "testing"

func TestLowerMachinePublishesAParsedChunk(t *testing.T) {
	program, err := Lower(Source{Name: "machine.lua", Text: []byte("return 1\n")})
	if err != nil || program == nil {
		t.Fatalf("Lower returned program=%v, err=%v", program, err)
	}
}

func TestLowerMachineRejectsEmptyLogicalName(t *testing.T) {
	if program, err := Lower(Source{Text: []byte("return 1\n")}); err == nil || program != nil {
		t.Fatalf("Lower accepted an unnamed source: program=%v err=%v", program, err)
	}
}
