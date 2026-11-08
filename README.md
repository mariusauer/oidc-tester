# oidc-tester

Small Go app for testing an OpenID Connect login flow against an identity provider.

It starts a local HTTP server, redirects the user to the provider login page, exchanges the authorization code for tokens, verifies the ID token, and renders the decoded claims in the browser.

## Requirements

- Go 1.26.1 or newer (for building)
- An OIDC-compatible identity provider
- An application registration with a redirect URI that points to this app

## Configuration

The app reads configuration from `config.json` in the project root.

Example:

```json
{
  "issuer_url": "https://your-adfs-server/adfs",
  "client_id": "your-client-id",
  "client_secret": "your-client-secret",
  "redirect_url": "http://localhost:3000/callback",
  "listen_addr": ":3000",
  "scopes": [
    "openid",
    "profile",
    "email"
  ]
}
```

Field summary:

- `issuer_url`: OIDC issuer or discovery base URL
- `client_id`: OAuth client ID
- `client_secret`: OAuth client secret
- `redirect_url`: callback URL registered with the provider
- `listen_addr`: local bind address for the web app
- `scopes`: scopes requested during login

## Run Locally

```bash
go run .
```

Then open `http://localhost:3000` unless you changed `listen_addr`.

`GET /health` returns `200 OK` with the plain-text body `ok` for startup, readiness, and liveness checks.

## Try locally with Docker Compose and Dex

With Docker and the Compose plugin installed, run from the repository root:

```bash
docker compose up --build -d
```

The example uses Dex v2.46.1. Open <http://localhost:3000>, start the login flow,
and sign in with:

- Email: `admin@example.com`
- Password: `password`

The admin user's `staticPasswords` entry assigns the groups `admins` and
`oidc-testers`. The example app requests the `groups` scope, so the ID token
includes `"groups": ["admins", "oidc-testers"]`. Edit the user's `groups` list
in `examples/dex/config.yaml` to try other memberships.

After login, Dex skips the approval screen and the app displays the verified ID
token claims. This is configured with `oauth2.skipApprovalScreen: true` in
`examples/dex/config.yaml`.
Dex's discovery document is available at
<http://localhost:5556/dex/.well-known/openid-configuration>.

The example mounts `examples/dex/oidc-tester.json` as the app's configuration
and `examples/dex/config.yaml` as Dex's configuration. Your root `config.json`
is not used by Compose. The client ID, secret, and callback URL must match in
both example files if you change them.

The app shares Dex's container network namespace so `http://localhost:5556/dex`
works as the issuer URL for both the browser and the app's token requests.
Both ports are published on the host's loopback interface, and the app starts
after Dex's discovery endpoint passes its health check. Use `localhost` in the
browser, as the registered callback uses that hostname.

View logs or stop the example with:

```bash
docker compose logs -f
docker compose down
```

This setup is for local testing: it uses HTTP, public example credentials, and
Dex's in-memory storage. Restarting Dex discards its state; start a fresh login
afterward. The static user is recreated from the configuration at startup.
Dex's [example configuration](https://github.com/dexidp/dex/blob/v2.46.1/config.yaml.dist)
documents the static client and password settings used here.

## Deploy with Flux

The `deploy/` directory contains a Kustomize example that installs the
HelmRepository and HelmRelease in `flux-system`. The Helm release targets the
`oidc-tester` namespace, where Kustomize also generates the
`oidc-tester-config` Secret from `deploy/config.json`.

Update the example configuration, image, ingress host, and TLS secret for your
environment, then apply it:

```bash
kubectl apply -k deploy
```

The cluster must already have Flux's source and Helm controllers and Stakater
Reloader installed. Do not commit real client secrets; use your cluster's
secret-management workflow instead.

## Build

Build both Linux and Windows x64 binaries:

```bash
make build
```

Build one platform only:

```bash
make linux
make windows
```

Artifacts are written to `dist/`.

## GitHub Actions

The workflow at `.github/workflows/build-and-publish.yml`:

- builds Linux x64 and Windows x64 binaries on pull requests, pushes to `main`, and manual runs
- publishes release assets when a tag matching `v*` is pushed
- publishes Linux AMD64/ARM64 images to GHCR with the Git tag and `latest`

Example release tag:

```bash
git tag v1.0.0
git push origin v1.0.0
```

## Notes

- The app currently disables TLS certificate verification for outbound OIDC requests. That is useful for some test environments, but it is not appropriate for production use.
- `config.json` contains secrets and should not be committed with real credentials.
