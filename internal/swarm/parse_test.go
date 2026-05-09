package swarm

import (
	"reflect"
	"testing"
)

func TestParseLabels(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   []Endpoint
	}{
		{
			name: "mixed valid and invalid labels",
			labels: map[string]string{
				"tsp.info.https.443": "80",
				"tsp.info.tcp.22":    "2222",
				"tsp.info.tcp.0":     "9999",
				"tsp.info.udp.53":    "53",
				"other.label":        "ignored",
			},
			want: []Endpoint{
				{Protocol: ProtocolHTTPS, ExposedPort: 443, ContainerPort: 80},
				{Protocol: ProtocolTCP, ExposedPort: 22, ContainerPort: 2222},
			},
		},
		{
			name: "ordering is deterministic",
			labels: map[string]string{
				"tsp.info.tcp.443":   "8443",
				"tsp.info.tcp.22":    "2222",
				"tsp.info.https.443": "80",
			},
			want: []Endpoint{
				{Protocol: ProtocolHTTPS, ExposedPort: 443, ContainerPort: 80},
				{Protocol: ProtocolTCP, ExposedPort: 22, ContainerPort: 2222},
				{Protocol: ProtocolTCP, ExposedPort: 443, ContainerPort: 8443},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseLabels(tt.labels)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParseServiceName(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   string
	}{
		{
			name: "single service name is returned",
			labels: map[string]string{
				"tsp.info.https.443": "80",
				"tsp.info.tcp.22":    "2222",
			},
			want: "info",
		},
		{
			name: "mixed services return empty name",
			labels: map[string]string{
				"tsp.info.https.443":    "80",
				"tsp.admin.tcp.2222":    "22",
				"tsp.invalid.tcp.bad":   "22",
				"unrelated.service.tag": "ignored",
			},
			want: "",
		},
		{
			name: "only invalid labels return empty name",
			labels: map[string]string{
				"tsp.info.udp.53": "53",
				"tsp.info.tcp.0":  "22",
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseServiceName(tt.labels)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
