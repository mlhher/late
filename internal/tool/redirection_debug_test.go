package tool

import (
	"fmt"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestParseRedirection(t *testing.T) {
	commands := []string{
		"ls > file.txt",
		"ls 2>&1",
		"ls &> file.txt",
		"ls 2> file.txt",
		"ls >> file.txt",
		"ls >&1",
		"ls >& file.txt",
		"ls 2>&-",
	}

	parser := syntax.NewParser()
	for _, cmd := range commands {
		f, err := parser.Parse(strings.NewReader(cmd), "")
		if err != nil {
			t.Errorf("Failed to parse %q: %v", cmd, err)
			continue
		}

		fmt.Printf("Command: %s\n", cmd)
		syntax.Walk(f, func(node syntax.Node) bool {
			if r, ok := node.(*syntax.Redirect); ok {
				wordStr := ""
				if r.Word != nil {
					for _, part := range r.Word.Parts {
						if lit, ok := part.(*syntax.Lit); ok {
							wordStr += lit.Value
						} else {
							wordStr += "[non-lit]"
						}
					}
				}
				fmt.Printf("  Op: %s, Word: %q, N: %v\n", r.Op, wordStr, r.N)
			}
			return true
		})
	}
}
