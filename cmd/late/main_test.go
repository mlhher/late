package main

import (
	"context"
	"encoding/json"
	"testing"
)

type mcpToolStub struct {
	name     string
	bareName string
}

func (t mcpToolStub) Name() string                                           { return t.name }
func (t mcpToolStub) BareName() string                                       { return t.bareName }
func (mcpToolStub) Description() string                                      { return "" }
func (mcpToolStub) Parameters() json.RawMessage                              { return nil }
func (mcpToolStub) Execute(context.Context, json.RawMessage) (string, error) { return "", nil }
func (mcpToolStub) RequiresConfirmation(json.RawMessage) bool                { return false }
func (mcpToolStub) CallString(json.RawMessage) string                        { return "" }

func TestMCPToolEnabledSupportsBareNames(t *testing.T) {
	testTool := mcpToolStub{name: "graph-rag__list_files", bareName: "list_files"}

	if mcpToolEnabled(testTool, map[string]bool{"list_files": false}) {
		t.Fatal("legacy bare-name setting did not disable namespaced MCP tool")
	}
	if !mcpToolEnabled(testTool, map[string]bool{"list_files": false, testTool.name: true}) {
		t.Fatal("namespaced setting did not override bare-name setting")
	}
}
