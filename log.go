package main

import (
	"fmt"
	"os"
)

var verbose bool

func logv(format string, args ...any) {
	if verbose {
		fmt.Fprintf(os.Stderr, "[verbose] "+format+"\n", args...)
	}
}
