package main

import (
	"context"
	"flag"
	"os"

	"github.com/google/subcommands"
)

func main() {
	subcommands.Register(&githubCmd{}, "")
	subcommands.Register(&Server{}, "")
	subcommands.Register(&appCmd{}, "")

	flag.Parse()
	ctx := context.Background()
	os.Exit(int(subcommands.Execute(ctx)))
}
