
# tsp

`tsp` connects Docker Swarm services to Tailscale Services.


## What it does

`tsp` runs on a Swarm manager and watches Docker service events.

For each labelled service, it:

- reads `tsp.*` labels from the Swarm service spec
- finds the service VIP on the configured Swarm overlay network
- creates or updates the matching Tailscale Service
- configures the local node as a service host using `tailscale serve`

## Example

Below example stack will start tsp proxy and expose whoami service at https://whoami.TAILNET.ts.net

1. Creat tag:docker in acls
2. Add auto approvers:

```json
"autoApprovers": {
		"services": {
			"tag:docker": ["tag:docker"],
		},
	},
```

**Required OAUTH Scopes**:

- General > Services > Write (tag:docker)
- Devices > Core > Write (tag:docker)
- Keys > Access Tokens > Write
- Keys > Auth Keys > Write (tag:docker)
- Keys > OAuth Keys > Write 

### Example Docker Stack

```yaml
services:
  tsp:
    image: ghcr.io/rtgnx/tsp:v0.0.1
    environment:
      TS_TAILNET: rtgnx.github
      TS_OAUTH_CLIENT_ID: file:/run/secrets/tsp_oauth_client_id
      TS_OAUTH_CLIENT_SECRET: file:/run/secrets/tsp_oauth_client_secret
      TS_TAGS: tag:docker # Associated tag with oauth credentials and autoapprovers
      SWARM_NETWORK: tsp-ingress
    secrets:
      - tsp_oauth_client_id
      - tsp_oauth_client_secret
    networks:
      - ts-ingress
    volumes:
      - tsp-state:/data
      - /var/run/docker.sock:/var/run/docker.sock
    deploy:
      replicas: 1
      placement:
        constraints:
          - node.role == manager
      restart_policy:
        condition: any
  whoami:
    image: traefik/whoami:v1.11.0
    networks:
      - tsp-ingress
    deploy:
      labels:
        tsp.name: whoami
        tsp.whoami.https.443: 80
      replicas: 1
    restart: unless-stopped

networks:
  ts-ingress:
    external: true
    name: tsp-ingress

volumes:
  tsp-state:

secrets:
  tsp_oauth_client_id:
    file: ./secrets/oauth_client_id.txt
  tsp_oauth_client_secret:
    file: ./secrets/oauth_client_secret.txt

```
