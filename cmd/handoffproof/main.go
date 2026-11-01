package main

import (
	"fmt"
	"os"
)

func usage() {
	fmt.Fprintln(os.Stderr, "usage: handoffproof <scan|drill|report> [flags]")
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "scan", "drill", "report":
		fmt.Fprintf(os.Stderr, "%s: not implemented yet\n", os.Args[1])
		os.Exit(1)
	default:
		usage()
		os.Exit(2)
	}
}
