# Rate limiting — the half that belongs at the proxy

The API limits by **identity**: the address being logged into, the token being
redeemed, the signed-in user. Those limits are in the service and need no
configuration.

It cannot limit by **IP address**, and this is not an oversight. The API has no
public URL. Visitors reach `waterloostar.com`, the reverse proxy hands them to
the Next.js server, and that server calls the API over the private Docker
network. Every request therefore arrives at the API from *one* address — the web
container's. Counting per IP inside the API would treat the entire internet as a
single client and throttle the whole site the moment one person retried a
password.

The reverse proxy is the only thing in the stack that sees real client addresses,
so per-IP limiting belongs there.

## The nginx rule

In 1Panel, the site's nginx configuration. The zone declaration goes in the
`http` block, the two `limit_req` lines in the site's `server` block.

```nginx
# http block — 10MB of state holds roughly 160,000 addresses.
limit_req_zone $binary_remote_addr zone=waterloostar_auth:10m rate=20r/m;
limit_req_zone $binary_remote_addr zone=waterloostar_all:10m rate=600r/m;

# In the server block for waterloostar.com:

# Sign-in, sign-up and password reset. 20 requests a minute per address, with a
# burst of 10 absorbed rather than refused, because a page load can legitimately
# make several requests at once.
location /api/auth/ {
    limit_req zone=waterloostar_auth burst=10 nodelay;
    limit_req_status 429;
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host              $host;
    proxy_set_header X-Real-IP         $remote_addr;
    proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}

# Everything else, as a backstop against a crawler hammering the site.
location / {
    limit_req zone=waterloostar_all burst=100 nodelay;
    limit_req_status 429;
    proxy_pass http://127.0.0.1:3000;
    proxy_set_header Host              $host;
    proxy_set_header X-Real-IP         $remote_addr;
    proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

`limit_req_status 429` matters: nginx answers **503** by default, which tells a
client the service is broken rather than that they are going too fast.

## Optional: telling two people apart inside the API

The API reads an `X-Client-IP` request header when one is present and uses it as
a second dimension of its own key, so two people behind the one frontend get
their own allowances rather than sharing one.

Nothing sends it today. For the frontend to do so, its API client would forward
the address of the visitor it is acting for. Until then the API's per-identity
limits and the nginx rules above cover the same ground from two directions.

**That header is trusted only because the API is unreachable from the internet.**
Anyone who can reach the API can forge it and hand themselves a fresh allowance.
If the API is ever given a public URL, the header must stop being trusted the
same day.

## What the limits are

They are constants in `internal/ratelimit/policy.go`, each with a comment saying
what it protects and why the number. Changing one is a one-line commit and a
deploy.

Two consequences of holding the state in the process, neither hidden:

- **A restart forgets the counters.** An attacker cannot restart the container,
  and a deploy is rare, so this is accepted rather than solved with Redis.
- **Limits are per instance.** One API container serves the site today. A second
  replica would double every effective limit, because each keeps its own buckets.
  If the service is ever scaled out, the limiter needs shared state first.
