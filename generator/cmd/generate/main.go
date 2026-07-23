package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/closur3/cn-eyeball-prefixes/generator/internal/ipv4build"
	"github.com/closur3/cn-eyeball-prefixes/generator/internal/ipv6build"
	"github.com/closur3/cn-eyeball-prefixes/generator/internal/listmanifest"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	command := os.Args[1]
	os.Args = append([]string{os.Args[0]}, os.Args[2:]...)

	switch command {
	case "ipv4":
		ipv4build.Main()
	case "ipv6":
		ipv6build.Main()
	case "manifest":
		flags := flag.NewFlagSet("manifest", flag.ExitOnError)
		root := flags.String("root", "", "public lists root")
		if err := flags.Parse(os.Args[1:]); err != nil {
			panic(err)
		}
		if *root == "" {
			panic("--root is required")
		}
		changed, err := listmanifest.Generate(*root)
		if err != nil {
			panic(err)
		}
		if changed {
			fmt.Println("updated public list manifest")
		} else {
			fmt.Println("public list manifest is already current")
		}
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: generate <ipv4|ipv6|manifest> [flags]")
	os.Exit(2)
}
