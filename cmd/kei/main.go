package main

import (
	"fmt"
	"os"
)

var version = "0.2.0-dev"

func main() {
	if len(os.Args) < 2 {
		printGeneralHelp()
		return
	}
	switch os.Args[1] {
	case "run":
		must(run(os.Args[2:]))
	case "models":
		must(models(os.Args[2:]))
	case "extensions":
		must(extensions(os.Args[2:]))
	case "tools":
		must(tools(os.Args[2:]))
	case "commands":
		must(commands(os.Args[2:]))
	case "exec":
		must(execTool(os.Args[2:]))
	case "acp":
		must(runACP(os.Args[2:]))
	case "login":
		must(login(os.Args[2:]))
	case "help", "--help", "-h":
		must(helpCmd(os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Println(version)
	default:
		fmt.Fprintf(os.Stderr, "kei: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: kei <run|models|extensions|tools|commands|exec|acp|login|help|version>")
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "kei:", err)
		os.Exit(1)
	}
}
