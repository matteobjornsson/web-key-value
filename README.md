# Web Key Value

A small Go web app backed by Render Key Value. Anyone can read; saving and deleting require a shared write secret. The frontend is embedded in the executable, with no JavaScript dependencies or build step. The only direct Go dependency is [go-redis](https://github.com/redis/go-redis).

## Run

With Go 1.26+ and a reachable Redis-compatible store, set `REDIS_URL` and `WRITE_SECRET` in your environment, then run:

```sh
go run .
```

Open http://localhost:8080. Set `PORT` to use another port. Enter your write secret in the page to make changes. The app does not save the secret to browser storage.

## HTTP API

| Request | Result |
| --- | --- |
| `GET /api/keys?limit=10&after=key` | JSON `{ "items": [{ "key": "…", "value": "…" }], "next": "…" }` |
| `GET /api/keys/{key}` | Plain-text value, or 404 |
| `PUT /api/keys/{key}` | Set the plain-text request body; 201 if new, 204 if replaced |
| `DELETE /api/keys/{key}` | Delete; 204 even if already absent |
| `GET /healthz` | Check connectivity to the store |

PUT and DELETE require `Authorization: Bearer YOUR_WRITE_SECRET`. Missing or incorrect secrets return 401. Keys contain 1–128 ASCII letters, digits, dots, underscores or hyphens, with no leading dot. Values are UTF-8 text, up to 16 KiB; empty values are valid. PATCH is not supported because each write replaces one complete string.

Pagination is alphabetical, with 1–100 entries per page. Pass the returned `next` as `after`; an empty `next` means the end. Pages reflect the current data rather than a snapshot.

```sh
curl "$BASE_URL/api/keys"
curl -X PUT -H "Authorization: Bearer $WRITE_SECRET" -H 'Content-Type: text/plain' --data 'hello world' "$BASE_URL/api/keys/greeting"
curl "$BASE_URL/api/keys/greeting"
curl -X DELETE -H "Authorization: Bearer $WRITE_SECRET" "$BASE_URL/api/keys/greeting"
```

## Deploy to Render

Create one Key Value instance and one Go web service in the same region. Set the web service's `REDIS_URL` to the Key Value instance's internal connection URL and `WRITE_SECRET` to your chosen secret. Keep external Key Value access disabled and use the `noeviction` memory policy.

- Build: `go build -o bin/web-key-value .`
- Start: `./bin/web-key-value`
- Source branch: `matteo/initial`

The app stores entries as fields in a single Redis hash named `web-key-value`. Listing reads that whole hash and sorts it in memory, intentionally suited to a small demo. Each write is one atomic Redis command. There are no expirations.

Data survives a web-service redeploy because it lives in Key Value. A free Render Key Value instance has no disk persistence: restarting or upgrading that instance can lose the data. This is a disposable demo, not a durable database service.

## Checks

```sh
go test ./...
go vet ./...
```

The local tests cover authorization, request validation, and pagination without a running store. Exercise successful reads and writes against the deployed instance to verify the complete connection. Request logs include method, path, status, and duration; authorization headers and values are not logged.
