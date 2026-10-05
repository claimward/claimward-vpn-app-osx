// Command claimward-helper is the privileged macOS helper for Claimward.
//
// It runs as a root LaunchDaemon and listens on a Unix socket for the app.
// Everything it does is github.com/claimward/claimward-vpn-client/pkg/helper,
// shared with the Linux and Windows apps: it enrolls only with a server its
// own configuration names, takes no tunnel configuration from a request, and
// listens on a 0660 socket for root and the admin group.
//
// Its configuration is /Library/Application Support/Claimward/helper.json,
// owned by root and writable by root alone (scripts/install-helper.sh):
//
//	{"servers": ["https://vpn.example.org"]}
package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/claimward/claimward-vpn-client/pkg/helper"
)

const defaultConfig = "/Library/Application Support/Claimward/helper.json"

func main() {
	path := flag.String("config", defaultConfig, "the helper's configuration")
	flag.Parse()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(*path, log); err != nil {
		log.Error("claimward-helper", "err", err)
		os.Exit(1)
	}
}

func run(path string, log *slog.Logger) error {
	cfg, err := helper.LoadConfig(path)
	if err != nil {
		return err
	}
	ln, err := helper.Listen(cfg.Socket, cfg.Group)
	if err != nil {
		return err
	}
	h := helper.New(*cfg, "darwin", "app-osx", log)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		h.Shutdown()
		_ = ln.Close()
		_ = os.Remove(cfg.Socket)
	}()

	log.Info("claimward-helper listening", "socket", cfg.Socket, "group", cfg.Group, "servers", cfg.Servers)
	return h.Serve(ln)
}
