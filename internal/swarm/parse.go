package swarm

import (
	"slices"
	"strconv"
	"strings"
)

// ParseLabels parses labels in the form:
//
//	tsp.<service>.tcp.<exposedPort>=<containerPort>
//	tsp.<service>.https.<exposedPort>=<containerPort>
//
// Invalid labels are ignored. The returned slice is sorted for deterministic
// reconciliation.
func ParseLabels(labels map[string]string) []Endpoint {
	endpoints := make([]Endpoint, 0, len(labels))

	for key, value := range labels {
		endpoint, ok := parseEndpointLabel(key, value)
		if !ok {
			continue
		}
		endpoints = append(endpoints, endpoint)
	}

	slices.SortFunc(endpoints, func(a, b Endpoint) int {
		if a.Protocol != b.Protocol {
			if a.Protocol < b.Protocol {
				return -1
			}
			return 1
		}
		if a.ExposedPort != b.ExposedPort {
			if a.ExposedPort < b.ExposedPort {
				return -1
			}
			return 1
		}
		if a.ContainerPort != b.ContainerPort {
			if a.ContainerPort < b.ContainerPort {
				return -1
			}
			return 1
		}
		return 0
	})

	return endpoints
}

func ParseServiceName(labels map[string]string) string {
	serviceName := ""

	for key, value := range labels {
		name, ok := parseServiceName(key, value)
		if !ok {
			continue
		}
		if serviceName == "" {
			serviceName = name
			continue
		}
		if serviceName != name {
			return ""
		}
	}

	return serviceName
}

func parseEndpointLabel(key, value string) (Endpoint, bool) {
	rest, ok := strings.CutPrefix(key, "tsp.")
	if !ok {
		return Endpoint{}, false
	}

	parts := strings.Split(rest, ".")
	if len(parts) != 3 {
		return Endpoint{}, false
	}

	serviceName := strings.TrimSpace(parts[0])
	if serviceName == "" {
		return Endpoint{}, false
	}

	protocol, ok := parseProtocol(parts[1])
	if !ok {
		return Endpoint{}, false
	}

	exposedPort, ok := parsePort(parts[2])
	if !ok {
		return Endpoint{}, false
	}

	containerPort, ok := parsePort(value)
	if !ok {
		return Endpoint{}, false
	}

	return Endpoint{
		Protocol:      protocol,
		ExposedPort:   exposedPort,
		ContainerPort: containerPort,
	}, true
}

func parseServiceName(key, value string) (string, bool) {
	rest, ok := strings.CutPrefix(key, "tsp.")
	if !ok {
		return "", false
	}

	parts := strings.Split(rest, ".")
	if len(parts) != 3 {
		return "", false
	}

	serviceName := strings.TrimSpace(parts[0])
	if serviceName == "" {
		return "", false
	}

	if _, ok := parseProtocol(parts[1]); !ok {
		return "", false
	}

	if _, ok := parsePort(parts[2]); !ok {
		return "", false
	}

	if _, ok := parsePort(value); !ok {
		return "", false
	}

	return serviceName, true
}

func parseProtocol(raw string) (Protocol, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "tcp":
		return ProtocolTCP, true
	case "https", "http":
		return ProtocolHTTPS, true
	default:
		return "", false
	}
}

func parsePort(raw string) (int, bool) {
	port, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || port < 1 || port > 65535 {
		return 0, false
	}
	return port, true
}
