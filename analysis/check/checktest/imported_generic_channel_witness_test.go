package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
)

func TestRequireCheckAndExportedGenericMemberSignatureInstantiatesNestedObjectLiteralChannel(t *testing.T) {
	processMod := CheckAndExport(`
		type ListenOptions<T> = {
			channel: Channel<T>,
			schema: {
				witness: {
					decode: (any) -> T,
				},
			},
		}
		local M = {}
		function M.listen_nested<T>(topic: string, options: ListenOptions<T>): Channel<T>
			return options.channel
		end
		function M.receive_map<T, U>(channel: Channel<T>, fn: (T) -> U): U?
			local value, ok = channel:receive()
			if ok then
				return fn(value)
			end
			return nil
		end
		return M
	`, "process")
	if len(processMod.Errors) != 0 {
		t.Fatalf("process module errors = %#v, want none", processMod.Errors)
	}

	checked := Check(`
		local process = require("process")
		type Node = { id: string }
		type Source = { nodes: Channel<Node> }
		local function node_type(): { decode: (any) -> Node }
			return { decode = function(raw: any): Node return { id = tostring(raw) } end }
		end
		local function handle(source: Source)
			local node_ch = process.listen_nested("nodes", {
				channel = source.nodes,
				schema = { witness = node_type() },
			})
			local mapped = process.receive_map(node_ch, function(node)
				local node_id: string = node.id
				local bad_node_id: number = node.id
				return node_id
			end)
			if mapped then
				local id: string = mapped
				local wrong_id: number = mapped
			end
		end
	`, WithStdlib(), WithModule("process", processMod))
	if len(checked.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want bad_node_id and wrong_id diagnostics", checked.Diagnostics)
	}
	for _, diag := range checked.Diagnostics {
		if diag.Code != diagnostics.CodeAssignmentType {
			t.Fatalf("diagnostic code = %s, want %s", diag.Code, diagnostics.CodeAssignmentType)
		}
	}
}

func TestRequireCheckAndExportedGenericMemberSignatureFeedsChannelReceive(t *testing.T) {
	processMod := CheckAndExport(`
		type ListenOptions<T> = {
			channel: Channel<T>,
		}
		local M = {}
		function M.listen<T>(topic: string, options: ListenOptions<T>): Channel<T>
			return options.channel
		end
		return M
	`, "process")
	if len(processMod.Errors) != 0 {
		t.Fatalf("process module errors = %#v, want none", processMod.Errors)
	}

	checked := Check(`
		local process = require("process")
		type Node = { id: string }
		type Source = { nodes: Channel<Node> }
		local function handle(source: Source)
			local node_ch = process.listen("nodes", {
				channel = source.nodes,
			})
			local node, ok = node_ch:receive()
			if ok then
				local id: string = node.id
				local wrong_id: number = node.id
			end
		end
	`, WithStdlib(), WithModule("process", processMod))
	if len(checked.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one wrong_id diagnostic", checked.Diagnostics)
	}
	if checked.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", checked.Diagnostics[0].Code, diagnostics.CodeAssignmentType)
	}
}

func TestRequireCheckAndExportedReceiveMapKeepsCallbackContextAfterPriorReceive(t *testing.T) {
	processMod := CheckAndExport(`
		type ListenOptions<T> = {
			channel: Channel<T>,
			schema: {
				witness: {
					decode: (any) -> T,
				},
			},
		}
		local M = {}
		function M.listen_nested<T>(topic: string, options: ListenOptions<T>): Channel<T>
			return options.channel
		end
		function M.receive_map<T, U>(channel: Channel<T>, fn: (T) -> U): U?
			local value, ok = channel:receive()
			if ok then
				return fn(value)
			end
			return nil
		end
		return M
	`, "process")
	if len(processMod.Errors) != 0 {
		t.Fatalf("process module errors = %#v, want none", processMod.Errors)
	}

	checked := Check(`
		local process = require("process")
		type Node = { id: string, children: {Node} }
		type Source = { nodes: Channel<Node> }
		local function node_type(): { decode: (any) -> Node }
			return { decode = function(raw: any): Node return { id = tostring(raw), children = {} } end }
		end
		local function handle(source: Source)
			local node_ch = process.listen_nested("nodes", {
				channel = source.nodes,
				schema = { witness = node_type() },
			})
			local node, node_ok = node_ch:receive()
			if node_ok then
				local node_id: string = node.id
			end
			local mapped = process.receive_map(node_ch, function(decoded)
				local decoded_id: string = decoded.id
				local bad_decoded_id: number = decoded.id
				return decoded_id
			end)
			if mapped then
				local accepted: string = mapped
				local bad_mapped: number = mapped
			end
			local summary = process.receive_map(node_ch, function(decoded)
				return {
					id = decoded.id,
					label = decoded.id .. ":node",
				}
			end)
			if summary then
				local id: string = summary.id
				local label: string = summary.label
				local bad_id: number = summary.id
				local bad_label: number = summary.label
			end
		end
	`, WithStdlib(), WithModule("process", processMod))
	if len(checked.Diagnostics) != 4 {
		t.Fatalf("diagnostics = %#v, want callback/member mismatch diagnostics", checked.Diagnostics)
	}
	for _, diag := range checked.Diagnostics {
		if diag.Code != diagnostics.CodeAssignmentType {
			t.Fatalf("diagnostic code = %s, want %s", diag.Code, diagnostics.CodeAssignmentType)
		}
	}
}

func TestRequireCheckAndExportedReceiveMapSeedsCallbackFromImportedSourceChannel(t *testing.T) {
	protocolMod := CheckAndExport(`
		type Type<T> = {
			decode: (any) -> T,
		}
		type RawRecord = {
			id: string,
			amount: number,
		}
		type Node = {
			id: string,
			children: {Node},
		}
		type Source = {
			records: Channel<RawRecord>,
			nodes: Channel<Node>,
		}
		local M = {}
		function M.raw_record_array_type(): Type<{RawRecord}>
			return {
				decode = function(raw: any): {RawRecord}
					return {{ id = tostring(raw), amount = 1 }}
				end,
			}
		end
		function M.node_type(): Type<Node>
			return {
				decode = function(raw: any): Node
					return { id = tostring(raw), children = {} }
				end,
			}
		end
		return M
	`, "protocol", WithStdlib())
	if len(protocolMod.Errors) != 0 {
		t.Fatalf("protocol module errors = %#v, want none", protocolMod.Errors)
	}
	jsonMod := CheckAndExport(`
		type Type<T> = {
			decode: (any) -> T,
		}
		local M = {}
		function M.decode_map<T, U>(data: string, witness: Type<T>, fn: (T) -> U): U
			return fn(witness.decode(data))
		end
		function M.decode_many_map<T, U>(data: string, witness: Type<{T}>, fn: (T) -> U): {U}
			local out: {U} = {}
			for _, item in ipairs(witness.decode(data)) do
				table.insert(out, fn(item))
			end
			return out
		end
		return M
	`, "json",
		WithStdlib(),
		WithManifest("channel", ChannelManifest()),
		WithGlobals("channel"),
		WithModule("protocol", protocolMod))
	if len(jsonMod.Errors) != 0 {
		t.Fatalf("json module errors = %#v, want none", jsonMod.Errors)
	}
	processMod := CheckAndExport(`
		type ListenOptions<T> = {
			channel: Channel<T>,
			schema: {
				witness: {
					decode: (any) -> T,
				},
			},
		}
		local M = {}
		function M.listen_nested<T>(topic: string, options: ListenOptions<T>): Channel<T>
			return options.channel
		end
		function M.receive_map<T, U>(channel: Channel<T>, fn: (T) -> U): U?
			local value, ok = channel:receive()
			if ok then
				return fn(value)
			end
			return nil
		end
		return M
	`, "process",
		WithStdlib(),
		WithManifest("channel", ChannelManifest()),
		WithGlobals("channel"),
		WithModule("protocol", protocolMod),
		WithModule("json", jsonMod))
	if len(processMod.Errors) != 0 {
		t.Fatalf("process module errors = %#v, want none", processMod.Errors)
	}

	checked := Check(`
		local protocol = require("protocol")
		local json = require("json")
		local process = require("process")
		local function handle(source: protocol.Source)
			local root_label = json.decode_map("{}", protocol.node_type(), function(decoded)
				local decoded_id: string = decoded.id
				local bad_decoded_id: number = decoded.id
				return decoded_id
			end)
			local accepted_label: string = root_label
			local bad_label: number = root_label

			local row_labels = json.decode_many_map("[]", protocol.raw_record_array_type(), function(row)
				local row_amount: number = row.amount
				local bad_row_amount: string = row.amount
				return row.id .. tostring(row_amount)
			end)
			local accepted_labels: {string} = row_labels
			local bad_labels: {number} = row_labels

			local node_ch = process.listen_nested("nodes", {
				channel = source.nodes,
				schema = { witness = protocol.node_type() },
			})
			local node, node_ok = node_ch:receive()
			if node_ok then
				local node_id: string = node.id
				for _, child in ipairs(node.children) do
					local child_id: string = child.id
				end
			end
			local mapped = process.receive_map(node_ch, function(decoded)
				local decoded_id: string = decoded.id
				local bad_decoded_id: number = decoded.id
				return decoded_id
			end)
			if mapped then
				local accepted: string = mapped
				local bad_mapped: number = mapped
			end
		end
	`, WithStdlib(),
		WithManifest("channel", ChannelManifest()),
		WithGlobals("channel"),
		WithModule("protocol", protocolMod),
		WithModule("json", jsonMod),
		WithModule("process", processMod))
	if len(checked.Diagnostics) != 6 {
		t.Fatalf("diagnostics = %#v, want json and receive_map mismatch diagnostics", checked.Diagnostics)
	}
	for _, diag := range checked.Diagnostics {
		if diag.Code != diagnostics.CodeAssignmentType {
			t.Fatalf("diagnostic code = %s, want %s", diag.Code, diagnostics.CodeAssignmentType)
		}
	}
}

func TestRequireCheckAndExportedGenericSignatureInstantiatesRecursiveWitness(t *testing.T) {
	protocolMod := CheckAndExport(`
		type Type<T> = {
			decode: (any) -> T,
		}
		type Node = {
			id: string,
			children: {Node},
		}
		local M = {}
		function M.node_type(): Type<Node>
			return {
				decode = function(raw: any): Node
					return { id = tostring(raw), children = {} }
				end,
			}
		end
		return M
	`, "protocol", WithStdlib())
	if len(protocolMod.Errors) != 0 {
		t.Fatalf("protocol module errors = %#v, want none", protocolMod.Errors)
	}
	jsonMod := CheckAndExport(`
		type Type<T> = {
			decode: (any) -> T,
		}
		local M = {}
		function M.decode<T>(data: string, witness: Type<T>): T
			return witness.decode(data)
		end
		return M
	`, "json")
	if len(jsonMod.Errors) != 0 {
		t.Fatalf("json module errors = %#v, want none", jsonMod.Errors)
	}

	checked := Check(`
		local protocol = require("protocol")
		local json = require("json")
		local root = json.decode("{}", protocol.node_type())
		local id: string = root.id
		local wrong_id: number = root.id
		for _, child in ipairs(root.children) do
			local child_id: string = child.id
			local wrong_child_id: number = child.id
		end
	`, WithStdlib(), WithModule("protocol", protocolMod), WithModule("json", jsonMod))
	if len(checked.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want wrong root and child id diagnostics", checked.Diagnostics)
	}
	for _, diag := range checked.Diagnostics {
		if diag.Code != diagnostics.CodeAssignmentType {
			t.Fatalf("diagnostic code = %s, want %s", diag.Code, diagnostics.CodeAssignmentType)
		}
	}
}

func TestRequireCheckAndExportedGenericSignatureInstantiatesRecursiveUnionWitness(t *testing.T) {
	protocolMod := CheckAndExport(`
		type Type<T> = {
			decode: (any) -> T,
		}
		type TextNode = {
			kind: "text",
			value: string,
		}
		type GroupNode = {
			kind: "group",
			children: {TreeNode},
		}
		type TreeNode = TextNode | GroupNode
		type RawRecord = {
			id: string,
			amount: number,
		}
		type Node = {
			id: string,
			children: {Node},
		}
		local M = {}
		function M.raw_record_type(): Type<RawRecord>
			return {
				decode = function(raw: any): RawRecord
					return { id = tostring(raw), amount = 1 }
				end,
			}
		end
		function M.node_type(): Type<Node>
			return {
				decode = function(raw: any): Node
					return { id = tostring(raw), children = {} }
				end,
			}
		end
		function M.tree_type(): Type<TreeNode>
			return {
				decode = function(raw: any): TreeNode
					return {
						kind = "group",
						children = {
							{
								kind = "text",
								value = tostring(raw),
							},
						},
					}
				end,
			}
		end
		return M
	`, "protocol", WithStdlib())
	if len(protocolMod.Errors) != 0 {
		t.Fatalf("protocol module errors = %#v, want none", protocolMod.Errors)
	}
	jsonMod := CheckAndExport(`
		type Type<T> = {
			decode: (any) -> T,
		}
		local M = {}
		function M.decode<T>(data: string, witness: Type<T>): T
			return witness.decode(data)
		end
		return M
	`, "json")
	if len(jsonMod.Errors) != 0 {
		t.Fatalf("json module errors = %#v, want none", jsonMod.Errors)
	}

	checked := Check(`
		local protocol = require("protocol")
		local json = require("json")
		local record = json.decode("{}", protocol.raw_record_type())
		local id: string = record.id
		local root = json.decode("{}", protocol.node_type())
		local root_id: string = root.id
		local tree = json.decode("{}", protocol.tree_type())
		if tree.kind == "group" then
			local first = tree.children[1]
			if first and first.kind == "text" then
				local value: string = first.value
				local bad_value: number = first.value
			end
		end
		if tree.kind == "text" then
			local children = tree.children
		end
	`, WithStdlib(), WithModule("protocol", protocolMod), WithModule("json", jsonMod))
	if len(checked.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want recursive union witness mismatch diagnostics", checked.Diagnostics)
	}
	messages := make([]string, 0, len(checked.Diagnostics))
	for _, diag := range checked.Diagnostics {
		messages = append(messages, diag.Message)
		if diag.Code != diagnostics.CodeAssignmentType && diag.Code != diagnostics.CodeMissingMember {
			t.Fatalf("diagnostic code = %s, want assignment or member-read diagnostic", diag.Code)
		}
	}
	if !hasDiagnosticMessage(messages, "cannot assign first.value because it is string, not number") ||
		!hasDiagnosticMessage(messages, `has no member "children"`) {
		t.Fatalf("diagnostics = %#v, want first.value mismatch and text.children missing-member", messages)
	}
}

func TestRequireCheckAndExportedGenericSignatureSeedsCallbackParamFromRecursiveWitness(t *testing.T) {
	protocolMod := CheckAndExport(`
		type Type<T> = {
			decode: (any) -> T,
		}
		type Node = {
			id: string,
			children: {Node},
		}
		local M = {}
		function M.node_type(): Type<Node>
			return {
				decode = function(raw: any): Node
					return { id = tostring(raw), children = {} }
				end,
			}
		end
		return M
	`, "protocol", WithStdlib())
	if len(protocolMod.Errors) != 0 {
		t.Fatalf("protocol module errors = %#v, want none", protocolMod.Errors)
	}
	jsonMod := CheckAndExport(`
		type Type<T> = {
			decode: (any) -> T,
		}
		local M = {}
		function M.decode_map<T, U>(data: string, witness: Type<T>, fn: (T) -> U): U
			return fn(witness.decode(data))
		end
		return M
	`, "json")
	if len(jsonMod.Errors) != 0 {
		t.Fatalf("json module errors = %#v, want none", jsonMod.Errors)
	}

	checked := Check(`
		local protocol = require("protocol")
		local json = require("json")
		local label = json.decode_map("{}", protocol.node_type(), function(node)
			local node_id: string = node.id
			local bad_node_id: number = node.id
			return node_id
		end)
		local accepted: string = label
		local bad_label: number = label
	`, WithStdlib(), WithModule("protocol", protocolMod), WithModule("json", jsonMod))
	if len(checked.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want bad_node_id and bad_label diagnostics", checked.Diagnostics)
	}
	for _, diag := range checked.Diagnostics {
		if diag.Code != diagnostics.CodeAssignmentType {
			t.Fatalf("diagnostic code = %s, want %s", diag.Code, diagnostics.CodeAssignmentType)
		}
	}
}
