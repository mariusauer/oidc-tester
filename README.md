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

Example release tag:

```bash
git tag v1.0.0
git push origin v1.0.0
```

## Notes

- The app currently disables TLS certificate verification for outbound OIDC requests. That is useful for some test environments, but it is not appropriate for production use.
- `config.json` contains secrets and should not be committed with real credentials.
