package main

import (
	"fmt"

	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/internal/testfixture"
)

func main() {
	comp, compOK := composite.Build()
	fmt.Printf("composition=%v available=%v schema=%v\n", compOK, comp.Available(), comp.ExecutionSchemaID().Available())
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(comp)
	fmt.Printf("issuance=%v count=%d axes=%v\n", issuanceOK, issuance.Count(), issuance.Axes())
	for _, name := range []string{"types/cast-multiple-in-statement", "native/arith-metamethod-operand-withheld"} {
		corpus, err := testfixture.LoadCorpus(".")
		if err != nil { panic(err) }
		project, err := corpus.Project(name)
		if err != nil { panic(err) }
		target, err := testfixture.StandardLibraryTarget()
		if err != nil { panic(err) }
		linked, err := testfixture.SealCorpusProject(target, project)
		if err != nil { panic(err) }
		mounts := linked.Project().Mounts()
		fmt.Printf("fixture=%s mounts=%d\n", name, mounts.Count())
		for index := 0; index < mounts.Count(); index++ {
			shard, shardOK := mounts.At(index)
			program, programOK := mounts.Program(shard)
			artifact, failure := artifactcompiler.CompileDetailed(program, comp.ExecutionSchemaID(), issuance)
			fmt.Printf("  mount=%d shard=%v program=%v id=%v artifact=%v failure=%v available=%v stage=%v reason=%v row=%v subrow=%v construction=%v\n", index, shardOK, programOK, program.ContentID(), artifact != nil, failure, failure.Available(), failure.Stage(), failure.Reason(), func() int { row, ok := failure.Row(); if !ok { return -1 }; return row }(), func() int { row, ok := failure.Subrow(); if !ok { return -1 }; return row }(), failure.Construction())
		}
	}
}
