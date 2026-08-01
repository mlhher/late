package common

import (
	"context"
	"encoding/json"
	"sort"
)

// Tool represents a primitive tool that the agent can execute.
type Tool interface {
	Name() string
	Description() string
	Parameters() json.RawMessage
	Execute(ctx context.Context, args json.RawMessage) (string, error)
	RequiresConfirmation(args json.RawMessage) bool
	CallString(args json.RawMessage) string
}

// ToolRegistry stores available tools.
type ToolRegistry struct {
	tools map[string]Tool
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]Tool),
	}
}

func (r *ToolRegistry) Register(t Tool) {
	r.tools[t.Name()] = t
}

func (r *ToolRegistry) Get(name string) Tool {
	return r.tools[name]
}

func (r *ToolRegistry) All() []Tool {
	var all []Tool
	for _, t := range r.tools {
		all = append(all, t)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].Name() < all[j].Name()
	})
	return all
}

// TodoItem represents a single item in a todo list.
type TodoItem struct {
	Text string
	Done bool
}

// TodoProvider provides a read-only snapshot of current todos.
type TodoProvider interface {
	GetTodos() []TodoItem
}

