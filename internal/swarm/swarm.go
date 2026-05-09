package swarm

import (
	"context"
	"fmt"
	"net/netip"
	"sync"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
)

const (
	CreateService = iota
	UpdateService
	DestroyService
)

type Protocol string

const (
	ProtocolHTTPS Protocol = "http"
	ProtocolTCP   Protocol = "tcp"
)

type Endpoint struct {
	ExposedPort   int
	ContainerPort int
	Protocol      Protocol
	VIP           netip.Prefix
}

func (e Endpoint) Target() string {
	return fmt.Sprintf("%s://%s:%d", e.Protocol, e.VIP.Addr().String(), e.ContainerPort)
}

type Event struct {
	EventType   int
	ServiceId   string
	ServiceName string
	Endpoints   []Endpoint
}

type SwarmWatcher struct {
	client    *client.Client
	networkId string
	mx        sync.Mutex
	services  map[string]string
}

func NewWatcher(networkName string) (*SwarmWatcher, error) {
	c, err := client.New(client.WithHost("unix:///var/run/docker.sock"))
	if err != nil {
		return nil, err
	}
	sw := &SwarmWatcher{
		client:   c,
		services: map[string]string{},
	}
	sw.networkId, err = resolveNetworkID(context.Background(), c, networkName)
	return sw, err
}
func (sw *SwarmWatcher) Discover(ctx context.Context) ([]Event, error) {
	events := make([]Event, 0)
	result, err := sw.client.ServiceList(ctx, client.ServiceListOptions{})
	if err != nil {
		return nil, err
	}

	for _, service := range result.Items {
		for _, vip := range service.Endpoint.VirtualIPs {
			if vip.NetworkID == sw.networkId {
				endpoints := ParseLabels(service.Spec.Labels)
				serviceName := ParseServiceName(service.Spec.Labels)
				if serviceName == "" || len(endpoints) < 1 {
					break
				}
				for i := range endpoints {
					endpoints[i].VIP = vip.Addr
				}
				sw.setServiceName(service.ID, serviceName)
				events = append(events, Event{
					ServiceId:   service.ID,
					ServiceName: serviceName,
					Endpoints:   endpoints,
					EventType:   CreateService,
				})
				break
			}
		}
	}

	return events, nil
}

func (sw *SwarmWatcher) Watch(ctx context.Context, out chan<- Event) error {

	filters := make(client.Filters)
	//filters.Add("type", "container")
	filters.Add("type", "service")
	result := sw.client.Events(ctx, client.EventsListOptions{
		Filters: filters,
	})

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-result.Err:
			if err != nil {
				return err
			}
		case msg := <-result.Messages:
			if msg.Type != events.ServiceEventType {
				continue
			}

			switch msg.Action {
			case events.ActionCreate:
				endpoints, sn, err := sw.ResolveEndpoints(ctx, msg.Actor.ID)
				if err != nil {
					break
				}
				if sn == "" || len(endpoints) < 1 {
					break
				}
				sw.setServiceName(msg.Actor.ID, sn)
				out <- Event{
					EventType:   CreateService,
					ServiceId:   msg.Actor.ID,
					Endpoints:   endpoints,
					ServiceName: sn,
				}
			case events.ActionUpdate:
				endpoints, sn, err := sw.ResolveEndpoints(ctx, msg.Actor.ID)
				if err != nil {
					break
				}
				if sn == "" || len(endpoints) < 1 {
					if sn, ok := sw.deleteServiceName(msg.Actor.ID); ok {
						out <- Event{EventType: DestroyService, ServiceId: msg.Actor.ID, ServiceName: sn}
					}
					break
				}
				sw.setServiceName(msg.Actor.ID, sn)
				out <- Event{
					EventType:   UpdateService,
					ServiceId:   msg.Actor.ID,
					Endpoints:   endpoints,
					ServiceName: sn,
				}
			case events.ActionRemove:
				if sn, ok := sw.deleteServiceName(msg.Actor.ID); ok {
					out <- Event{EventType: DestroyService, ServiceId: msg.Actor.ID, ServiceName: sn}
				}
			}
		}
	}
}

func (sw *SwarmWatcher) ResolveEndpoints(ctx context.Context, serviceId string) ([]Endpoint, string, error) {
	result, err := sw.client.ServiceInspect(ctx, serviceId, client.ServiceInspectOptions{})
	if err != nil {
		return nil, "", err
	}
	serviceName := ParseServiceName(result.Service.Spec.Labels)
	for _, vip := range result.Service.Endpoint.VirtualIPs {
		if vip.NetworkID == sw.networkId {
			endpoints := ParseLabels(result.Service.Spec.Labels)
			for i := range endpoints {
				endpoints[i].VIP = vip.Addr
			}
			return endpoints, serviceName, nil
		}
	}

	return nil, serviceName, fmt.Errorf("service doesn't expose vip on %s", sw.networkId)
}

func resolveNetworkID(ctx context.Context, cli *client.Client, networkName string) (string, error) {
	result, err := cli.NetworkList(ctx, client.NetworkListOptions{})
	if err != nil {
		return "", err
	}

	for _, nw := range result.Items {
		if nw.Name == networkName {
			return nw.ID, nil
		}
	}

	return "", fmt.Errorf("network %q not found", networkName)
}

func (sw *SwarmWatcher) setServiceName(serviceID, serviceName string) {
	sw.mx.Lock()
	defer sw.mx.Unlock()

	sw.services[serviceID] = serviceName
}

func (sw *SwarmWatcher) deleteServiceName(serviceID string) (string, bool) {
	sw.mx.Lock()
	defer sw.mx.Unlock()

	serviceName, ok := sw.services[serviceID]
	if !ok {
		return "", false
	}
	delete(sw.services, serviceID)
	return serviceName, true
}
