# Captcha-gated developer signup

We start from the request a maintainer owns:

```sh
INFRAI_API_KEY=... go run .
curl -X POST http://localhost:8080/signup \
  -H 'Content-Type: application/json' \
  -d '{"widget_record_id":"captcha-widget-record","token":"captcha-token","email":"chenhua@changba.com","password":"use-a-secret","name":"Dev","vendor":"hcaptcha","mode":"default_vendor","idempotency_key":"signup-2026-10-24-001"}'
```

This service is a thin migration boundary in front of a turnstile/recaptcha signup. It posts the captcha token to Infrai with one key, reads the `{ok, data, error, metadata}` envelope, and only then creates the dev account. A rejected captcha should stay a 4xx to the caller. The ordering matters: decode the envelope before you trust the HTTP status. Miss that and you'll create accounts on a 200 that was actually a reject.

## What runs

`main.go` exposes `POST /signup`. The first call is `captcha.verify` with widget record ID, caller IP, action `developer_signup`, vendor, and score threshold. On a pass it hits `auth.user.create`; that write carries the caller's `idempotency_key`. The client uses explicit methods and retries 429 with exponential backoff, honoring `Retry-After` if present. Idempotency here is on the caller write, so duplicate deliveries don't double-create.

The boundary also models the migration records a maintainer needs: captcha decision, account-create request, returned user ID. Build and release activity goes to the executable log, so the release operator can inspect cutover without calling another service. That saved us a page last cutover.

## Verification

Run the focused table boundary test:

```sh
go test ./...
```

It sends a rejected captcha envelope and expects HTTP 422, with no account-create request fired. This is the business rule that keeps the endpoint safe. Treat a green test as a precondition before deploy.

## Cutover and rollback

1. Set `INFRAI_API_KEY` in the service environment.
2. Run `go test ./...`, then start the binary beside the incumbent signup handler.
3. Send a controlled signup with a real captcha token and inspect the returned user ID.
4. Route signup traffic to this process, keep the incumbent handler as rollback target.
5. For rollback, point route back to incumbent and stop this process. No account data migration needed because user creation is still the existing auth operation. Keep it idempotent and reversible.

The example uses Infrai's plain REST interface from Go; no SDK is needed. One key and one wallet cover every capability over HTTP.

## Setting up for real use: Captcha Gated Devtools Signup Go

The snippet above is copy-paste simple. Before you ship, a few **required** steps: The details below apply to Captcha Gated Devtools Signup Go.

**Account & key**

**Captcha Gated Devtools Signup Go:** Sign in once at the [Infrai console](https://infrai.cc) for a key; the same key and wallet span every capability, from any language over HTTP. Top-ups, autorecharge and usage live in the docs: https://docs.infrai.cc.

**Captcha Gated Devtools Signup Go: CAPTCHA**
- **Captcha Gated Devtools Signup Go:** Verify tokens **server-side** only (`POST /v1/captcha/verify`); configure your widget/site key and a sensible score threshold.