package ts

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"slices"
	"sync"

	"github.com/rtgnx/tsp/internal/swarm"
	"tailscale.com/client/local"
	"tailscale.com/ipn"
	"tailscale.com/tailcfg"
)

var (
	mx     sync.Mutex
	tsdCmd *exec.Cmd
	Local  *LocalClient
)

const (
	stateDir   = "/data"
	socketFile = "/data/tailscaled.sock"
)

type LocalClient struct {
	*local.Client
}

func (c *LocalClient) Up(ctx context.Context, authKey string) error {
	cmd := exec.CommandContext(ctx, "/tailscale", "--socket="+socketFile, "up", "--auth-key="+authKey)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *LocalClient) Apply(ctx context.Context, se swarm.Event) error {
	sc, err := c.GetServeConfig(ctx)
	if err != nil {
		return err
	}
	if sc == nil {
		sc = new(ipn.ServeConfig)
	}
	if sc.Services == nil {
		sc.Services = map[tailcfg.ServiceName]*ipn.ServiceConfig{}
	}

	sn := normalizeServiceName(se.ServiceName)
	sc.Services[sn] = &ipn.ServiceConfig{}
	mds, err := c.magicDNSSuffix(ctx)
	if err != nil {
		return err
	}

	for _, endpoint := range se.Endpoints {
		switch endpoint.Protocol {
		case swarm.ProtocolTCP:
			sc.SetTCPForwardingForService(uint16(endpoint.ExposedPort), endpoint.Target(), false, sn, 0, "")
		case swarm.ProtocolHTTPS:
			sc.SetWebHandler(&ipn.HTTPHandler{
				Proxy: endpoint.Target(),
			}, string(sn), uint16(endpoint.ExposedPort), "/", true, mds)
		}
	}

	if err := c.SetServeConfig(ctx, sc); err != nil {
		return fmt.Errorf("apply local serve config for %s: %w", se.ServiceName, err)
	}
	return c.advertiseService(ctx, sn)
}

func (c *LocalClient) Delete(ctx context.Context, serviceName string) error {
	sc, err := c.GetServeConfig(ctx)
	if err != nil {
		return err
	}
	if sc == nil || len(sc.Services) == 0 {
		return nil
	}

	sn := normalizeServiceName(serviceName)
	if _, ok := sc.Services[sn]; !ok {
		return nil
	}

	delete(sc.Services, sn)
	if len(sc.Services) == 0 {
		sc.Services = nil
	}

	if err := c.SetServeConfig(ctx, sc); err != nil {
		return fmt.Errorf("delete local serve config for %s: %w", serviceName, err)
	}
	return c.unadvertiseService(ctx, sn)
}

func Start(ctx context.Context) error {
	mx.Lock()
	defer mx.Unlock()

	if Local == nil {
		Local = &LocalClient{
			Client: &local.Client{
				Socket:        socketFile,
				UseSocketOnly: true,
			},
		}
	}

	if tsdCmd != nil && tsdCmd.Process != nil {
		return nil
	}

	if err := os.MkdirAll(stateDir, 0750); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "/tailscaled",
		"--statedir="+stateDir,
		"--socket="+socketFile,
		`--tun=userspace-networking`,
	)
	cmd.Dir = stateDir
	//cmd.Stdout = os.Stdout
	//cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return err
	}

	tsdCmd = cmd

	go func(cmd *exec.Cmd) {
		err := cmd.Wait()

		mx.Lock()
		if tsdCmd == cmd {
			tsdCmd = nil
		}
		mx.Unlock()

		if ctx.Err() != nil {
			return
		}

		if err == nil {
			log.Printf("tailscaled exited")
		} else {
			log.Printf("tailscaled exited: %v", err)
		}
		os.Exit(1)
	}(cmd)

	return nil
}

func normalizeServiceName(serviceName string) tailcfg.ServiceName {
	if len(serviceName) >= 4 && serviceName[:4] == "svc:" {
		return tailcfg.ServiceName(serviceName)
	}
	return "svc:" + tailcfg.ServiceName(serviceName)
}

func (c *LocalClient) magicDNSSuffix(ctx context.Context) (string, error) {
	st, err := c.Status(ctx)
	if err != nil {
		return "", err
	}
	if st.CurrentTailnet != nil && st.CurrentTailnet.MagicDNSSuffix != "" {
		return st.CurrentTailnet.MagicDNSSuffix, nil
	}
	if st.MagicDNSSuffix != "" {
		return st.MagicDNSSuffix, nil
	}
	return "", fmt.Errorf("tailscale status missing MagicDNS suffix")
}

func (c *LocalClient) advertiseService(ctx context.Context, serviceName tailcfg.ServiceName) error {
	prefs, err := c.GetPrefs(ctx)
	if err != nil {
		return err
	}
	if slices.Contains(prefs.AdvertiseServices, serviceName.String()) {
		return nil
	}

	services := append(append([]string(nil), prefs.AdvertiseServices...), serviceName.String())
	_, err = c.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs: ipn.Prefs{
			AdvertiseServices: services,
		},
		AdvertiseServicesSet: true,
	})
	if err != nil {
		return fmt.Errorf("advertise service %s: %w", serviceName, err)
	}
	return nil
}

func (c *LocalClient) unadvertiseService(ctx context.Context, serviceName tailcfg.ServiceName) error {
	prefs, err := c.GetPrefs(ctx)
	if err != nil {
		return err
	}
	if !slices.Contains(prefs.AdvertiseServices, serviceName.String()) {
		return nil
	}

	services := make([]string, 0, len(prefs.AdvertiseServices))
	for _, service := range prefs.AdvertiseServices {
		if service == serviceName.String() {
			continue
		}
		services = append(services, service)
	}

	_, err = c.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs: ipn.Prefs{
			AdvertiseServices: services,
		},
		AdvertiseServicesSet: true,
	})
	if err != nil {
		return fmt.Errorf("unadvertise service %s: %w", serviceName, err)
	}
	return nil
}
