package ts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/rtgnx/tsp/internal/swarm"
)

const tailscaleApi = `https://api.tailscale.com`

var (
	apiOAuthToken string
)

func init() {
	apiOAuthToken = must(url.JoinPath(tailscaleApi, `/api/v2/oauth/token`))
}

type Client struct {
	tailnet      string
	httpClient   *http.Client
	mx           sync.Mutex
	clientId     string
	clientSecret string
	accessToken  string
	tokenExpiry  time.Time
	serviceTags  []string
}

type OAuthTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

type APIError struct {
	Code int
	Body string
}

func (err APIError) Error() string {
	return fmt.Sprintf("http=%d, body=%s", err.Code, err.Body)
}

func NewClient(tailnet, clientId, clientSecret string, tags []string) *Client {
	return &Client{
		tailnet:      tailnet,
		httpClient:   &http.Client{},
		mx:           sync.Mutex{},
		clientId:     clientId,
		clientSecret: clientSecret,
		serviceTags:  tags,
	}
}

func (c *Client) token(ctx context.Context) (string, error) {
	c.mx.Lock()
	defer c.mx.Unlock()

	if c.accessToken != "" && time.Until(c.tokenExpiry) > 30*time.Second {
		return c.accessToken, nil
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiOAuthToken, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(c.clientId, c.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := c.httpClient.Do(req)

	if err != nil {
		return "", err
	}

	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return "", APIError{Code: res.StatusCode, Body: string(body)}
	}
	tok := new(OAuthTokenResponse)
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(tok); err != nil {
		return "", err
	}

	c.accessToken = tok.AccessToken
	c.tokenExpiry = time.Now().Add(time.Second * time.Duration(tok.ExpiresIn))
	return c.accessToken, nil
}

type VIPService struct {
	Name  string   `json:"name,omitempty"`
	Addrs []string `json:"addrs,omitempty"`
	Ports []string `json:"ports,omitempty"`
	Tags  []string `json:"tags,omitempty"`
}

type KeyCapabilities struct {
	Devices KeyDeviceCapabilities `json:"devices,omitempty"`
}

type KeyDeviceCapabilities struct {
	Create KeyDeviceCreateCapabilities `json:"create"`
}

type KeyDeviceCreateCapabilities struct {
	Reusable      bool     `json:"reusable"`
	Ephemeral     bool     `json:"ephemeral"`
	Preauthorized bool     `json:"preauthorized"`
	Tags          []string `json:"tags,omitempty"`
}

func (c *Client) Apply(ctx context.Context, se swarm.Event) error {
	switch se.EventType {
	case swarm.CreateService:
		log.Printf("tailscale: action=create, service=%s", se.ServiceName)
		return c.CreateService(ctx, se)
	case swarm.UpdateService:
		log.Printf("tailscale: action=update, service=%s", se.ServiceName)
		return c.UpdateService(ctx, se)
	case swarm.DestroyService:
		log.Printf("tailscale: action=delete, service=%s", se.ServiceName)
		return c.DeleteService(ctx, se)
	default:
		return nil
	}
}

func (c *Client) CreateService(ctx context.Context, se swarm.Event) error {
	serviceName := string(normalizeServiceName(se.ServiceName))
	return c.putVIPService(ctx, serviceName, se.Endpoints, false)
}

func (c *Client) UpdateService(ctx context.Context, se swarm.Event) error {
	serviceName := string(normalizeServiceName(se.ServiceName))
	return c.putVIPService(ctx, serviceName, se.Endpoints, true)
}

func (c *Client) getVIPService(ctx context.Context, serviceName string) (*VIPService, error) {
	apiServiceGet := must(url.JoinPath(
		tailscaleApi,
		`/api/v2/tailnet`,
		url.PathEscape(c.tailnet),
		`vip-services`,
		url.PathEscape(serviceName),
	))

	res, err := c.request(ctx, http.MethodGet, apiServiceGet, nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return nil, APIError{Code: res.StatusCode, Body: string(body)}
	}

	var svc VIPService
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&svc); err != nil {
		return nil, err
	}

	return &svc, nil
}

func (c *Client) putVIPService(ctx context.Context, serviceName string, endpoints []swarm.Endpoint, requireExisting bool) error {
	apiService := must(url.JoinPath(
		tailscaleApi,
		`/api/v2/tailnet`,
		url.PathEscape(c.tailnet),
		`vip-services`,
		url.PathEscape(serviceName),
	))

	vipService := VIPService{
		Name: serviceName,
		Tags: c.serviceTags,
	}

	existing, err := c.getVIPService(ctx, serviceName)
	switch {
	case err == nil:
		vipService.Addrs = existing.Addrs
	case requireExisting:
		return err
	default:
		var apiErr APIError
		if !errors.As(err, &apiErr) || apiErr.Code != http.StatusNotFound {
			return err
		}
	}

	for _, endpoint := range endpoints {
		vipService.Ports = append(vipService.Ports, fmt.Sprintf("tcp:%d", endpoint.ExposedPort))
	}

	b, err := json.Marshal(vipService)
	if err != nil {
		return err
	}

	res, err := c.request(ctx, http.MethodPut, apiService, b)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(res.Body)
		return APIError{Code: res.StatusCode, Body: string(body)}
	}

	return nil
}

func (c *Client) DeleteService(ctx context.Context, se swarm.Event) error {
	serviceName := string(normalizeServiceName(se.ServiceName))
	apiServiceDelete := must(url.JoinPath(
		tailscaleApi,
		`/api/v2/tailnet`,
		url.PathEscape(c.tailnet),
		`vip-services`,
		url.PathEscape(serviceName),
	))

	res, err := c.request(ctx, http.MethodDelete, apiServiceDelete, nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		return nil
	case http.StatusNotFound:
		return nil
	default:
		body, _ := io.ReadAll(res.Body)
		return APIError{Code: res.StatusCode, Body: string(body)}
	}
}

func (c *Client) TSAuthToken(ctx context.Context, tags []string) (string, error) {
	apiKeyCreate := must(url.JoinPath(
		tailscaleApi,
		`/api/v2/tailnet`,
		url.PathEscape("-"),
		`keys`,
	))

	reqBody := struct {
		Capabilities KeyCapabilities `json:"capabilities"`
	}{
		Capabilities: KeyCapabilities{
			Devices: KeyDeviceCapabilities{
				Create: KeyDeviceCreateCapabilities{
					Reusable:      false,
					Ephemeral:     false,
					Preauthorized: true,
					Tags:          tags,
				},
			},
		},
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	res, err := c.request(ctx, http.MethodPost, apiKeyCreate, b)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return "", APIError{Code: res.StatusCode, Body: string(body)}
	}

	var out struct {
		Secret string `json:"key"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); err != nil {
		return "", err
	}
	if out.Secret == "" {
		return "", fmt.Errorf("tailscale returned empty auth key")
	}
	return out.Secret, nil
}

func (c *Client) request(ctx context.Context, method, path string, data []byte) (*http.Response, error) {

	var body io.Reader
	if data != nil {
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set(`Content-Type`, `application/json`)
	}
	token, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	req.Header.Set(`Authorization`, `Bearer `+token)
	return c.httpClient.Do(req)
}

func must[T any](v T, err error) T {
	if err != nil {
		log.Fatal(err.Error())
	}

	return v
}
