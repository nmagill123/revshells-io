package main

import (
	"errors"
	"flag"
	"log"
	"os"

	"github.com/noahmagill/webhook-rev-shell/internal/agent"
)

func main() {
	server := flag.String("server", os.Getenv("RSD_SERVER"), "broker base URL")
	session := flag.String("session", os.Getenv("RSD_SESSION"), "session id")
	secret := flag.String("secret", os.Getenv("RSD_SECRET"), "session secret")
	noPTY := flag.Bool("no-pty", false, "force HTTP command mode (no TTY)")
	flag.Parse()

	if *noPTY {
		os.Setenv("RSD_NO_PTY", "1")
	}

	cfg := agent.Config{Server: *server, Session: *session, Secret: *secret}
	if cfg.Server == "" || cfg.Session == "" || cfg.Secret == "" {
		if c, err := agent.ConfigFromEnv(); err == nil {
			cfg = c
		} else {
			log.Fatal("usage: rs-agent -server URL -session ID -secret SECRET")
		}
	}

	if err := agent.Run(cfg); err != nil {
		if errors.Is(err, agent.ErrBeaconRejected) {
			os.Exit(1)
		}
		log.Fatal(err)
	}
}
