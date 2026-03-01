# UI Authentication

The operator's web UI can be protected with an authentication provider. Two providers are supported: **OIDC** (OpenID Connect) and **GitHub OAuth**. If neither is configured, the UI is publicly accessible.

Only one provider can be active at a time. OIDC takes precedence over GitHub OAuth.

---

## OIDC

Compatible with any OIDC-compliant identity provider (Keycloak, Dex, Google, Azure AD, Okta, etc.).

### Helm Configuration

```yaml
auth:
  oidc:
    enabled: true
    issuerUrl: "https://accounts.google.com"   # OIDC provider issuer URL
    clientId: "your-client-id"
    existingSecret: "oidc-secret"              # Kubernetes secret name
    secretKey: "client-secret"                 # Key inside the secret
    sessionSecretKey: ""                       # Optional: key for session encryption secret
    redirectUrl: ""                            # Optional: auto-detected from ingress
    insecureSkipVerify: false                  # Do not use in production
    logoutUrl: ""                              # Optional: auto-discovered via OIDC metadata
```

### Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: oidc-secret
stringData:
  client-secret: "<your-oidc-client-secret>"
```

### OIDC Provider Setup

Register a confidential OAuth client with your identity provider and set the callback URL to:

```
https://<your-operator-host>/auth/callback
```

Required scopes: `openid`, `email`, `profile`

---

## GitHub OAuth

Authenticates users via a GitHub OAuth App.

### Helm Configuration

```yaml
auth:
  github:
    enabled: true
    clientId: "your-github-client-id"
    existingSecret: "github-oauth-secret"     # Kubernetes secret name
    secretKey: "client-secret"                # Key inside the secret
    sessionSecretKey: ""                      # Optional session encryption key
    redirectUrl: ""                           # Optional: auto-detected from ingress
```

### Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: github-oauth-secret
stringData:
  client-secret: "<your-github-client-secret>"
```

### GitHub OAuth App Setup

Create an OAuth App at **GitHub → Settings → Developer settings → OAuth Apps** with the callback URL:

```
https://<your-operator-host>/auth/callback
```

The operator requests the `read:user` and `user:email` scopes. On logout, the OAuth token is automatically revoked.

---

## Session Security

Sessions are stored as AES-256-GCM encrypted cookies and expire after **24 hours**.

If you run multiple operator replicas, you **must** set a static session secret. Otherwise each pod generates its own key and users will be logged out when requests hit a different replica.

Set the session secret via `sessionSecretKey` pointing to a key in your existing secret, or the operator will auto-generate one per startup.

---

## Notes

- Auth protects the **web UI only**. The webhook endpoints are unaffected.
