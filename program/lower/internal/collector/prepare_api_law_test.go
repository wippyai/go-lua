package collector_test

import (
	"reflect"
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/lower/internal/collector"
	"github.com/wippyai/go-lua/program/source"
)

// This external-package law proves that lowering publishes one opaque,
// copyable assembly capability—not an owner-capability splice surface. The
// public receiver is the only way to consume it, and every copy shares one
// terminal winner.
func TestPrepareIsTheOnlyPublicCollectorFreezeBoundary(t *testing.T) {
	typeOfCollector := reflect.TypeOf((*collector.Collector)(nil))
	for _, forbidden := range []string{"SourceInput", "DependentInputs"} {
		if _, ok := typeOfCollector.MethodByName(forbidden); ok {
			t.Fatalf("Collector still exports repeatable %s", forbidden)
		}
	}
	if _, ok := typeOfCollector.MethodByName("Prepare"); !ok {
		t.Fatal("Collector does not export terminal Prepare")
	}

	preparedType := reflect.TypeOf(collector.Prepared{})
	if preparedType.NumField() != 1 {
		t.Fatalf("Prepared fields = %d, want one private shared-state field", preparedType.NumField())
	}
	stateField := preparedType.Field(0)
	if stateField.IsExported() {
		t.Fatalf("Prepared field %q is exported", stateField.Name)
	}
	for name, methodType := range map[string]reflect.Type{
		"value":   preparedType,
		"pointer": reflect.TypeOf((*collector.Prepared)(nil)),
	} {
		if methodType.NumMethod() != 1 || methodType.Method(0).Name != "Assemble" {
			t.Fatalf("Prepared %s methods = %d, want Assemble only", name, methodType.NumMethod())
		}
	}
	for _, forbidden := range []string{"Source", "Flow", "Static", "Module", "Entry"} {
		if _, ok := preparedType.FieldByName(forbidden); ok {
			t.Fatalf("Prepared exposes raw %s capability", forbidden)
		}
	}
	if assembly, err := (collector.Prepared{}).Assemble(); err == nil || assembly != nil {
		t.Fatalf("zero Prepared Assemble = %v/%v, want invalid rejection", assembly, err)
	}

	c := collector.New("prepare-api.lua", 0, bind.GlobalCensus{})
	order := c.Source().Order()
	body := order.Body(source.Span{File: "prepare-api.lua"})
	if body == 0 || !order.SetBody(body) || !order.SetEntry(body) {
		t.Fatal("public Source setup failed")
	}
	prepared, err := c.Prepare()
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	repeated, repeatedErr := c.Prepare()
	if repeatedErr == nil {
		t.Fatalf("second Prepare = %#v/%v, want terminal rejection", repeated, repeatedErr)
	}
	if assembly, err := repeated.Assemble(); err == nil || assembly != nil {
		t.Fatalf("failed Prepare returned a live capability: %v/%v", assembly, err)
	}

	const copies = 16
	start := make(chan struct{})
	type outcome struct {
		assembly *flow.Assembly
		err      error
	}
	results := make(chan outcome, copies)
	var group sync.WaitGroup
	for range copies {
		copy := prepared
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			assembly, err := copy.Assemble()
			results <- outcome{assembly: assembly, err: err}
		}()
	}
	close(start)
	group.Wait()
	close(results)

	winners := 0
	for result := range results {
		if result.err == nil && result.assembly != nil {
			winners++
			continue
		}
		if result.err == nil || result.assembly != nil {
			t.Fatalf("Prepared copy result = %v/%v, want failed loser", result.assembly, result.err)
		}
	}
	if winners != 1 {
		t.Fatalf("Prepared copy winners = %d, want exactly one", winners)
	}
	if assembly, err := prepared.Assemble(); err == nil || assembly != nil {
		t.Fatalf("repeated Prepared Assemble = %v/%v, want terminal rejection", assembly, err)
	}
}
