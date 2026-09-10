package cli

import (
	"fmt"
	"io"
)

// renderTree prints a group heading followed by its items as an ASCII tree:
//
//	DNS
//	 ├── A
//	 ├── AAAA
//	 └── PTR
func renderTree(w io.Writer, group string, items []string) {
	fmt.Fprintln(w, group)
	for i, item := range items {
		branch := "├──"
		if i == len(items)-1 {
			branch = "└──"
		}
		fmt.Fprintf(w, " %s %s\n", branch, item)
	}
}
