
# tsp

`tsp` connects Docker Swarm services to Tailscale Services.

It watches Docker Swarm service labels, creates or updates matching Tailscale Services, and configures the local node with `tailscale serve`.

The goal is to make this flow automatic:

1. deploy a Swarm service
2. add a few `tsp.*` labels
3. get a stable `svc:<name>` Tailscale Service
4. have traffic routed to the service VIP on the Swarm overlay network

No manual TailVIP setup. No manually keeping `tailscale serve` in sync.

## What it does

`tsp` runs on a Swarm manager and watches Docker service events.

For each labelled service, it:

- reads `tsp.*` labels from the Swarm service spec
- finds the service VIP on the configured Swarm overlay network
- creates or updates the matching Tailscale Service
- configures the local node as a service host using `tailscale serve`

## Label format

Add labels under `deploy.labels`:

```yaml
deploy:
  labels:
    tsp.whoami.https.443: "80"
    tsp.ssh.tcp.22: "22"
```

The format is:

```text
tsp.<service-name>.<protocol>.<exposed-port>: "<target-port>"
```

Supported protocols:

- `https`
- `tcp`

Examples:

```yaml
tsp.whoami.https.443: "80"
```

This creates or updates:

```text
svc:whoami
```

and serves:

```text
https://svc:whoami:443 -> http://<swarm-service-vip>:80
```

Another example:

```yaml
tsp.ssh.tcp.22: "22"
```

This serves:

```text
tcp://svc:ssh:22 -> tcp://<swarm-service-vip>:22
```

## Tailscale OAuth setup

`tsp` uses a Tailscale OAuth client to:

1. get API access tokens
2. create auth keys when the local `tailscaled` node has no saved state
3. create and update Tailscale Services

The OAuth client needs permission to:

- create auth keys
- manage Tailscale Services

In the Tailscale admin console, create or select an OAuth client under **Trust credentials** and grant the smallest write scope that covers those two things.

Using `all` also works, but it is broader than necessary.

Relevant docs:

- [OAuth clients](https://tailscale.com/docs/features/oauth-clients)
- [Tags](https://tailscale.com/docs/features/tags)
- [Trust credentials](https://tailscale.com/docs/reference/trust-credentials)

## Tags

OAuth-created auth keys must use tags.

Pick a tag for the `tsp` node, for example:

```text
tag:docker
```

Then:

1. create the OAuth client with `tag:docker`
2. set:

```env
TS_TAGS=tag:docker
```

3. make sure your tailnet policy allows that tag

Example policy section:

```json
{
  "tagOwners": {
    "tag:docker": ["autogroup:admin"]
  }
}
```

## Auto-approving services

Tailscale Service hosts usually need approval before they become active.

To avoid approving every service manually, add an `autoApprovers.services` rule to your tailnet policy.

For example:

```json
{
  "tagOwners": {
    "tag:docker": ["autogroup:admin"]
  },
  "autoApprovers": {
    "services": {
      "tag:docker": ["tag:docker"]
    }
  }
}
```

This allows nodes authenticated as `tag:docker` to advertise services tagged as `tag:docker`.

You can also approve individual services instead:

```json
{
  "autoApprovers": {
    "services": {
      "svc:whoami": ["tag:docker"]
    }
  }
}
```

Relevant docs:

- [Tailscale Services: automatic approval](https://tailscale.com/docs/features/tailscale-services)
- [Tailnet policy syntax](https://tailscale.com/kb/1337/acl-syntax/)

## Deploy `tsp`

### 1. Create the shared Swarm network

Create the overlay network used by `tsp` and your workload stacks:

```bash
docker network create \
  --driver overlay \
  --attachable \
  ts-ingress
```

This network is external so other stacks can join it by name.

### 2. Add OAuth secrets

The example stack expects these files:

```text
examples/tsp/secrets/oauth_client_id.txt
examples/tsp/secrets/oauth_client_secret.txt
```

Create them:

```bash
mkdir -p examples/tsp/secrets

printf '%s' 'YOUR_OAUTH_CLIENT_ID' \
  > examples/tsp/secrets/oauth_client_id.txt

printf '%s' 'YOUR_OAUTH_CLIENT_SECRET' \
  > examples/tsp/secrets/oauth_client_secret.txt
```

The stack reads them through Docker secrets using paths like:

```text
file:/run/secrets/...
```

`tsp` resolves those values with `util.EnvValue(...)`.

### 3. Build the image

```bash
docker build -t tsp:local .
```

### 4. Deploy the stack

```bash
docker stack deploy -c examples/tsp/compose.yml ts-ingress
```

This runs one `tsp` instance, pinned to a Swarm manager.

It will:

- persist Tailscale state in `/data`
- watch Swarm services
- authenticate as the configured tag
- manage Tailscale Services
- configure local `tailscale serve` state

## Deploy an example workload

The example workload is here:

```text
examples/whoami/compose.yml
```

It joins the same external network and adds a `tsp` label:

```yaml
deploy:
  labels:
    tsp.whoami.https.443: "80"
```

Deploy it:

```bash
docker stack deploy -c examples/whoami/compose.yml whoami
```

After that:

1. Swarm creates the `whoami` service
2. `tsp` sees the label
3. `tsp` creates or updates `svc:whoami`
4. the local `tsp` node advertises the service
5. Tailscale routes traffic to the Swarm service VIP

## Notes

- `tsp` works with Docker Swarm services, not standalone containers.
- `tsp` must run on a Swarm manager because it watches Swarm service events through the Docker socket.
- `SWARM_NETWORK` must match the external overlay network used by your workload stacks.
