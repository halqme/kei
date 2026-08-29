package main

import (
	"context"
	"flag"
	"os"

	"github.com/halqme/kei/internal/acp"
	"github.com/halqme/kei/internal/agent"
	keisession "github.com/halqme/kei/internal/session"
)

func runACP(args []string) error {
	fs := flag.NewFlagSet("acp", flag.ContinueOnError)
	conf := fs.String("config", "", "config path")
	model := fs.String("m", "", "model override or alias")
	prov := fs.String("provider", "", "connection target override")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := loadSessionConfig(*conf)
	if err != nil {
		return err
	}
	srv := acp.NewServer(os.Stdin, os.Stdout, func(id, cwd string) (*agent.Runtime, error) {
		r, err := discoverExtensions(cfg, cwd)
		if err != nil {
			return nil, err
		}
		state := &keisession.State{ID: id, Workspace: cwd}
		return makeRuntime(cfg, r, state, nil, *prov, *model)
	})
	return srv.Serve(context.Background())
}
