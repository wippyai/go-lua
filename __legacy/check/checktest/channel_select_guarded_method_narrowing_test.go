package checktest

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/module/manifest"
)

// Mirrors wippy-golua-seam tests/app/src/test/process/link_explicit.lua: a
// channel.select over a message channel and a time.after timeout channel
// produces a process.Message | time.Time union on result.value. A structural
// guard on result.channel == timeout, followed by an early return, narrows
// away time.Time for every read of result.value in the fallthrough branch.
func processMessageManifestWithTopic() *manifest.Manifest {
	messageType := typ.NewInterface("process.Message", []typ.Method{
		{Name: "topic", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
	channelGeneric := runtimeModuleChannelGeneric()
	messageChannelType := typ.Instantiate(channelGeneric, messageType)

	m := manifest.New("process")
	m.DefineType("Message", messageType)
	m.SetExport(typetable.NewRecord().
		Field("inbox", typ.Func().Returns(messageChannelType).Build()).
		Build())
	return m
}

// processMessageManifestWithField is the field-access variant of the same
// union shape: process.Message carries a data field instead of a method, so
// a plain member read exercises the same guard-narrowing mechanism through a
// different access mode.
func processMessageManifestWithField() *manifest.Manifest {
	messageType := typetable.NewRecord().
		Field("topic_name", typ.String).
		Build()
	channelGeneric := runtimeModuleChannelGeneric()
	messageChannelType := typ.Instantiate(channelGeneric, messageType)

	m := manifest.New("process")
	m.SetExport(typetable.NewRecord().
		Field("inbox", typ.Func().Returns(messageChannelType).Build()).
		Build())
	return m
}

func channelSelectGuardedNarrowingSource(access string) string {
	return `
local process = require("process")
local time = require("time")
local channel = require("channel")

local function main()
    local inbox_ch = process.inbox()
    local timeout = time.after("2s")
    local result = channel.select {
        inbox_ch:case_receive(),
        timeout:case_receive(),
    }

    if result.channel == timeout then
        return false, "timeout waiting for link confirmation"
    end

    local msg = result.value
    ` + access + `
    return true
end
`
}

// TestCheckChannelSelectGuardedMethodCallNarrowsUnion is the red reproducer:
// the guard provably eliminates time.Time from msg, so the method call must
// not diagnose "has no member".
func TestCheckChannelSelectGuardedMethodCallNarrowsUnion(t *testing.T) {
	channelGeneric := runtimeModuleChannelGeneric()
	result := Check(channelSelectGuardedNarrowingSource("local topic = msg:topic()"),
		WithStdlib(),
		WithManifest("channel", ChannelManifest()),
		WithManifest("process", processMessageManifestWithTopic()),
		WithManifest("time", timeAfterManifest(channelGeneric)),
	)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none: guard on result.channel == timeout narrows msg to process.Message before msg:topic()", result.Diagnostics)
	}
}

// TestCheckChannelSelectGuardedFieldReadNarrowsUnion is the companion field
// read: the same guard, at the same point, on the same union already narrows
// correctly for a plain member read. It must keep passing.
func TestCheckChannelSelectGuardedFieldReadNarrowsUnion(t *testing.T) {
	channelGeneric := runtimeModuleChannelGeneric()
	result := Check(channelSelectGuardedNarrowingSource("local topic = msg.topic_name"),
		WithStdlib(),
		WithManifest("channel", ChannelManifest()),
		WithManifest("process", processMessageManifestWithField()),
		WithManifest("time", timeAfterManifest(channelGeneric)),
	)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for plain read of narrowed union", result.Diagnostics)
	}
}

// TestCheckChannelSelectUnguardedMethodCallOnUnionDiagnoses is the soundness
// companion: without the narrowing guard, time.Time really does not support
// topic(), so the method call must still diagnose.
func TestCheckChannelSelectUnguardedMethodCallOnUnionDiagnoses(t *testing.T) {
	channelGeneric := runtimeModuleChannelGeneric()
	src := `
local process = require("process")
local time = require("time")
local channel = require("channel")

local function main()
    local inbox_ch = process.inbox()
    local timeout = time.after("2s")
    local result = channel.select {
        inbox_ch:case_receive(),
        timeout:case_receive(),
    }

    local msg = result.value
    local topic = msg:topic()
    return true
end
`
	result := Check(src,
		WithStdlib(),
		WithManifest("channel", ChannelManifest()),
		WithManifest("process", processMessageManifestWithTopic()),
		WithManifest("time", timeAfterManifest(channelGeneric)),
	)
	if !hasDiagnosticContaining(result.Diagnostics, "has no member", "topic") {
		t.Fatalf("diagnostics = %#v, want a missing-member diagnostic for msg:topic() without the narrowing guard", result.Diagnostics)
	}
}
