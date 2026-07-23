package main

import (
	"fmt"
	"os"

	"github.com/closur3/cn-eyeball-prefixes/generator/internal/ipv4verify"
	"github.com/closur3/cn-eyeball-prefixes/generator/internal/ipv6build"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	command := os.Args[1]
	os.Args = append([]string{os.Args[0]}, os.Args[2:]...)

	switch command {
	case "ipv4":
		ipv4verify.Main()
	case "ipv6":
		ipv6build.VerifyMain()
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: verify <ipv4|ipv6> [flags]")
	os.Exit(2)
}
