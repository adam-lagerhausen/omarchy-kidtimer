package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"kidtimer/daemon/internal/desk"
	"kidtimer/daemon/internal/household"
	"kidtimer/daemon/internal/reverse"
)

func runParent(args []string) error {
	if len(args) > 0 && args[0] == "export" {
		return runParentExport(args[1:])
	}
	fs := flag.NewFlagSet("parent", flag.ContinueOnError)
	home := fs.String("home", "", "household dir (default ~/.local/share/kidtimer)")
	httpAddr := fs.String("http", "", "desk HTTP listen (default 127.0.0.1:8741)")
	sessionAddr := fs.String("session", "", "session listen (default 0.0.0.0:8743)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir, err := shareDir(*home)
	if err != nil {
		return err
	}
	if _, err := ensureHousehold(dir); err != nil {
		return err
	}
	if err := household.WriteRole(dir, reverse.RoleParent); err != nil {
		return err
	}
	lock, err := lockParent(dir)
	if err != nil {
		return err
	}
	defer lock.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return desk.Run(ctx, desk.Config{Home: dir, HTTPAddr: *httpAddr, SessionAddr: *sessionAddr})
}

func runParentExport(args []string) error {
	req, err := Parse("export", args)
	if err != nil {
		return err
	}
	return Do(req)
}

func shareDir(home string) (string, error) {
	if home != "" {
		return home, nil
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(userHome, ".local", "share", "kidtimer"), nil
}

func ensureHousehold(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := household.Path(dir)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := household.Save(path, nil); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	if err := household.ImportIfEmpty(dir); err != nil {
		return "", err
	}
	return path, nil
}

func lockParent(dir string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(dir, "parent.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}
