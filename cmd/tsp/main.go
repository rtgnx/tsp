package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	cli "github.com/jawher/mow.cli"
	"github.com/rtgnx/tsp/internal/tsp"
	"github.com/rtgnx/tsp/internal/util"
)

func main() {
	app := cli.App("tsp", "Tailscale Swarm Proxy")

	cfg := tsp.Config{}

	app.StringOptPtr(&cfg.OAuthClientID, `client-id`, util.EnvValue(`TS_OAUTH_CLIENT_ID`, ``), `tailscale oauth client id`)
	app.StringOptPtr(&cfg.OAuthClientSecret, `client-secret`, util.EnvValue(`TS_OAUTH_CLIENT_SECRET`, ``), `tailscale oauth client secret`)
	app.StringOptPtr(&cfg.Tailnet, `tailnet`, util.EnvValue(`TS_TAILNET`, ``), `tailscale tailnet`)
	app.StringOptPtr(&cfg.SwarmNetwork, `swarm-network`, util.EnvValue(`SWARM_NETWORK`, ``), `ingress swarm network`)
	app.StringsOptPtr(&cfg.Tags, "tags", strings.Split(util.EnvValue("TS_TAGS", ""), ","), "tailscale tags")

	app.Before = func() {
		cfg.Normalize()
		if err := cfg.Validate(); err != nil {
			log.Fatal(err)
		}
	}

	app.Action = func() {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()

		if err := tsp.Start(ctx, cfg); err != nil {
			log.Fatal(err)
		}
	}

	app.Run(os.Args)
}
