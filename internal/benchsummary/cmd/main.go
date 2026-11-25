// Command cmd reads the output of the advice benchmark on stdin and prints its one summary line.
package main

import (
	"fmt"
	"os"

	"github.com/WoW-Companion/advisor/internal/benchsummary"
)

func main() {
	line, ok := benchsummary.Total(os.Stdin)
	if !ok {
		fmt.Fprintln(os.Stderr, "advice-benchmark: no count lines in the output (was a dataset given?)")
		os.Exit(1)
	}
	fmt.Println(line)
}
