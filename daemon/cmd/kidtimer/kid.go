package main

import (
	"context"
	"flag"
	"os/signal"
	"syscall"

	"kidtimer/daemon/internal/dial"
)

func runKid(args []string) error {
	fs := flag.NewFlagSet("kid", flag.ContinueOnError)
	home := fs.String("home", "", "household dir (default ~/.local/share/kidtimer)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir, err := shareDir(*home)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return dial.Run(ctx, dial.Config{Home: dir})
}
