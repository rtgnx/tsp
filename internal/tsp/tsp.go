package tsp

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/rtgnx/tsp/internal/swarm"
	"github.com/rtgnx/tsp/internal/ts"
)

type Config struct {
	Tailnet           string   `validate:"required"`
	OAuthClientID     string   `validate:"required"`
	OAuthClientSecret string   `validate:"required"`
	SwarmNetwork      string   `validate:"required"`
	Tags              []string `validate:"omitempty,dive,startswith=tag:"`
}

func (c *Config) Normalize() {
	tags := make([]string, 0, len(c.Tags))
	for _, tag := range c.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		tags = append(tags, tag)
	}
	c.Tags = tags
}

func (c Config) Validate() error {
	return validator.New().Struct(c)
}

func Start(ctx context.Context, cfg Config) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := ts.Start(ctx); err != nil {
		return err
	}

	sw, err := swarm.NewWatcher(cfg.SwarmNetwork)
	if err != nil {
		return err
	}

	tsClient := ts.NewClient(cfg.Tailnet, cfg.OAuthClientID, cfg.OAuthClientSecret, cfg.Tags)

	status, err := ts.Local.Status(ctx)
	if err != nil {
		return err
	}

	if !status.HaveNodeKey {
		token, err := tsClient.TSAuthToken(ctx, cfg.Tags)
		if err != nil {
			return err
		}

		if err := ts.Local.Up(ctx, token); err != nil {
			return err
		}
	}

	seCh := make(chan swarm.Event, 32)
	watchErrCh := make(chan error, 1)

	go func() {
		if err := sw.Watch(ctx, seCh); err != nil {
			watchErrCh <- err
			cancel()
		}
	}()

	events, err := sw.Discover(ctx)
	if err != nil {
		return err
	}
	for _, event := range events {
		seCh <- event
	}

	runErr := Run(ctx, tsClient, seCh)
	select {
	case err := <-watchErrCh:
		if err != nil && err != context.Canceled {
			return err
		}
	default:
	}
	return runErr
}

func Run(ctx context.Context, tsc *ts.Client, ch <-chan swarm.Event) error {
	for {
		select {
		case <-ctx.Done():
			log.Print(ctx.Err())
			return ctx.Err()
		case e := <-ch:
			if err := tsc.Apply(ctx, e); err != nil {
				log.Print(err.Error())
				continue
			}
			switch e.EventType {
			case swarm.DestroyService:
				if err := ts.Local.Delete(ctx, e.ServiceName); err != nil {
					return fmt.Errorf("delete local service %s: %w", e.ServiceName, err)
				}
			default:
				if err := ts.Local.Apply(ctx, e); err != nil {
					return fmt.Errorf("apply local service %s: %w", e.ServiceName, err)
				}
			}
		}
	}
}
