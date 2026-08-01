package tool

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func TestCreateTodos(t *testing.T) {
	var todos []Todo
	var mu sync.Mutex
	tool := CreateTodosTool{Todos: &todos, Mu: &mu}

	// Create a list of todos
	args := json.RawMessage(`{"todos": ["Step 1", "Step 2", "Step 3"]}`)
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Created 3 todo(s)") {
		t.Fatalf("expected success message with count, got: %s", result)
	}
	if len(todos) != 3 || todos[0].Text != "Step 1" || todos[2].Text != "Step 3" {
		t.Fatalf("todos not stored correctly: %v", todos)
	}
	t.Logf("result: %s", result)
}

func TestCreateTodosEmpty(t *testing.T) {
	var todos []Todo
	tool := CreateTodosTool{Todos: &todos}

	args := json.RawMessage(`{"todos": []}`)
	result, _ := tool.Execute(context.Background(), args)
	if !strings.Contains(result, "cannot be empty") {
		t.Fatalf("expected empty error, got: %s", result)
	}
}

func TestListTodos(t *testing.T) {
	todos := []Todo{
		{Text: "Setup", Done: false},
		{Text: "Code", Done: false},
		{Text: "Test", Done: false},
	}
	tool := ListTodosTool{Todos: &todos}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "# Todo List") {
		t.Fatalf("expected header, got: %s", result)
	}
	if !strings.Contains(result, "1. [ ] Setup") {
		t.Fatalf("expected checkbox format, got: %s", result)
	}
	if !strings.Contains(result, "2. [ ] Code") {
		t.Fatalf("expected checkbox format, got: %s", result)
	}
	if !strings.Contains(result, "3. [ ] Test") {
		t.Fatalf("expected checkbox format, got: %s", result)
	}
	t.Logf("result: %s", result)
}

func TestListTodosEmpty(t *testing.T) {
	var todos []Todo
	tool := ListTodosTool{Todos: &todos}

	result, _ := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if !strings.Contains(result, "No todos") {
		t.Fatalf("expected empty message, got: %s", result)
	}
}

func TestFinishTodoSuccess(t *testing.T) {
	todos := []Todo{
		{Text: "Step 1", Done: false},
		{Text: "Step 2", Done: false},
		{Text: "Step 3", Done: false},
	}
	tool := FinishTodoTool{Todos: &todos}

	// Finish the second todo
	args := json.RawMessage(`{"todo_text": "Step 2"}`)
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Completed: Step 2") {
		t.Fatalf("expected completion message, got: %s", result)
	}
	if !todos[1].Done {
		t.Fatalf("todo not marked complete: %v", todos[1])
	}
	t.Logf("result: %s", result)
}

func TestFinishTodoAlreadyCompleted(t *testing.T) {
	todos := []Todo{
		{Text: "Step 1", Done: true},
	}
	tool := FinishTodoTool{Todos: &todos}

	args := json.RawMessage(`{"todo_text": "Step 1"}`)
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "already completed") {
		t.Fatalf("expected already completed message, got: %s", result)
	}
}

func TestFinishTodoNotFound(t *testing.T) {
	todos := []Todo{
		{Text: "Step 1", Done: false},
		{Text: "Step 2", Done: false},
	}
	tool := FinishTodoTool{Todos: &todos}

	// Try with wrong text
	args := json.RawMessage(`{"todo_text": "Step 4"}`)
	result, _ := tool.Execute(context.Background(), args)
	if !strings.Contains(result, "No todo found") {
		t.Fatalf("expected not found error, got: %s", result)
	}
	if !strings.Contains(result, "list_todos") {
		t.Fatalf("expected error to mention list_todos, got: %s", result)
	}
	// Todos should be unchanged
	if len(todos) != 2 {
		t.Fatalf("todos should be unchanged, got: %v", todos)
	}
}

func TestFinishTodoExactMatch(t *testing.T) {
	todos := []Todo{{Text: "Do The Thing", Done: false}}
	tool := FinishTodoTool{Todos: &todos}

	// Case-sensitive: "do the thing" should NOT match "Do The Thing"
	args := json.RawMessage(`{"todo_text": "do the thing"}`)
	result, _ := tool.Execute(context.Background(), args)
	if !strings.Contains(result, "No todo found") {
		t.Fatalf("expected case-sensitive mismatch, got: %s", result)
	}

	// Exact match should work
	args = json.RawMessage(`{"todo_text": "Do The Thing"}`)
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Completed") {
		t.Fatalf("expected success, got: %s", result)
	}
}

func TestCreateFinishListFlow(t *testing.T) {
	var todos []Todo
	var mu sync.Mutex

	createTool := CreateTodosTool{Todos: &todos, Mu: &mu}
	listTool := ListTodosTool{Todos: &todos, Mu: &mu}
	finishTool := FinishTodoTool{Todos: &todos, Mu: &mu}

	// 1. Create todos
	_, err := createTool.Execute(context.Background(), json.RawMessage(`{"todos": ["Write code", "Write test"]}`))
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// 2. Finish first todo
	_, err = finishTool.Execute(context.Background(), json.RawMessage(`{"todo_text": "Write code"}`))
	if err != nil {
		t.Fatalf("finish failed: %v", err)
	}

	// 3. List todos and check checkbox output
	listResult, err := listTool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	if !strings.Contains(listResult, "1. [✓] Write code") {
		t.Errorf("expected completed item formatted as '1. [✓] Write code', got:\n%s", listResult)
	}
	if !strings.Contains(listResult, "2. [ ] Write test") {
		t.Errorf("expected uncompleted item formatted as '2. [ ] Write test', got:\n%s", listResult)
	}
}

func TestCreateTodosNilPointerSafe(t *testing.T) {
	var mu sync.Mutex
	tool := CreateTodosTool{Todos: nil, Mu: &mu}

	args := json.RawMessage(`{"todos": ["Step 1", "Step 2"]}`)
	// Should not panic when Todos is nil
	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFinishTodoDuplicates(t *testing.T) {
	todos := []Todo{
		{Text: "Run tests", Done: false},
		{Text: "Update docs", Done: false},
		{Text: "Run tests", Done: false},
	}
	tool := FinishTodoTool{Todos: &todos}

	// First finish should complete the first "Run tests"
	res1, err := tool.Execute(context.Background(), json.RawMessage(`{"todo_text": "Run tests"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res1, "Completed: Run tests") {
		t.Fatalf("expected completion of first item, got: %s", res1)
	}
	if !todos[0].Done || todos[2].Done {
		t.Fatalf("expected only first 'Run tests' to be done, got: %v", todos)
	}

	// Second finish should complete the second "Run tests"
	res2, err := tool.Execute(context.Background(), json.RawMessage(`{"todo_text": "Run tests"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res2, "Completed: Run tests") {
		t.Fatalf("expected completion of second item, got: %s", res2)
	}
	if !todos[0].Done || !todos[2].Done {
		t.Fatalf("expected both 'Run tests' to be done, got: %v", todos)
	}

	// Third finish should report already completed
	res3, _ := tool.Execute(context.Background(), json.RawMessage(`{"todo_text": "Run tests"}`))
	if !strings.Contains(res3, "already completed") {
		t.Fatalf("expected already completed message, got: %s", res3)
	}
}

func TestFinishTodoTrimSpace(t *testing.T) {
	todos := []Todo{
		{Text: "Do Something", Done: false},
	}
	tool := FinishTodoTool{Todos: &todos}

	// Leading/trailing whitespace should be trimmed and match
	args := json.RawMessage(`{"todo_text": "  Do Something  \n"}`)
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Completed: Do Something") {
		t.Fatalf("expected success with trimmed text, got: %s", result)
	}
	if !todos[0].Done {
		t.Fatalf("expected todo to be marked done")
	}
}

