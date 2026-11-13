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
| `GET /api/keys?limit=10&after=cursor` | JSON `{ "items": [{ "key": "…", "value": "…" }], "next": "…" }` |
| `GET /api/keys/{key}` | Plain-text value, or 404 |
| `PUT /api/keys/{key}` | Set the plain-text request body; 201 if new, 204 if replaced |
| `DELETE /api/keys/{key}` | Delete; 204 even if already absent |
| `GET /healthz` | Check connectivity to the store |

PUT and DELETE require `Authorization: Bearer YOUR_WRITE_SECRET`. Missing or incorrect secrets return 401. Keys contain 1–128 ASCII letters, digits, dots, underscores or hyphens, with no leading dot. Values are UTF-8 text, up to 16 KiB; empty values are valid. PATCH is not supported because each write replaces one complete string.

Pagination is newest update first, with 1–100 entries per page and alphabetical ordering for ties. Entries saved before timestamp tracking sort last until updated. Pass the returned `next` as the URL-encoded `after` parameter; an empty `next` means the end. The cursor includes the update time and key. Pages reflect current data rather than a snapshot, so a key updated during pagination moves ahead of the cursor and is visible when you refresh from the first page.

```sh
curl "$BASE_URL/api/keys"
curl -X PUT -H "Authorization: Bearer $WRITE_SECRET" -H 'Content-Type: text/plain' --data 'hello world' "$BASE_URL/api/keys/greeting"
curl "$BASE_URL/api/keys/greeting"
curl -X DELETE -H "Authorization: Bearer $WRITE_SECRET" "$BASE_URL/api/keys/greeting"
```

## Deploy to Render

Create a Blueprint from this repository using `render.yaml`. It provisions a Free Go web service and a Free Key Value instance in Oregon, grouped under the `web-key-value` project and `production` environment. Key Value uses the `noeviction` memory policy and no external access. The web service receives its internal connection URL automatically.

Render generates `WRITE_SECRET` during setup. Copy it from the web service's **Environment** page in the Render Dashboard and enter it in the app to save or delete values. Anyone you share the secret with can write. The secret is preserved across Blueprint syncs.

- Build: `go build -o bin/web-key-value .`
- Start: `./bin/web-key-value`
- Source branch: `main`
- Health check: `/healthz`

Commits to `main` automatically deploy the web service.

The app stores values in a Redis hash named `web-key-value` and update timestamps in `web-key-value-updated`, in the same Key Value instance. Writes and deletes update both hashes in a transaction. Listing reads both hashes in a transaction and sorts in memory, intentionally suited to a small demo. There are no expirations.

Data survives a web-service redeploy because it lives in Key Value. Free Key Value has no persistence: restarting or upgrading that instance loses its data. The Free web service sleeps after 15 minutes without traffic and takes about a minute to wake up. Listing loads all entries into memory, so keep this app's dataset small.

## Checks

```sh
go test ./...
go vet ./...
```

The local tests cover authorization, request validation, and pagination without a running store. Exercise successful reads and writes against the deployed instance to verify the complete connection. Request logs include method, path, status, and duration; authorization headers and values are not logged.
