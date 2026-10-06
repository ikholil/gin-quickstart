# Ecommerce API

A single-process Gin ecommerce API backed by a local SQLite database. Payment processing and shipping-carrier integrations are not included.

## Run locally

Use Go 1.27.1 or newer. Configure the required signing secret and start the server:

```sh
export JWT_SECRET="$(openssl rand -hex 32)"
go run .
```

Configuration:

| Variable | Required | Default | Notes |
|---|---|---|---|
| `PORT` | No | `8080` | Listen port (1-65535) |
| `DB_PATH` | No | `./data/ecommerce.db` | SQLite file path; parent directory is created |
| `JWT_SECRET` | Yes | — | At least 32 bytes; HS256 signing key |
| `JWT_TTL` | No | `15m` | Positive Go duration |
| `ADMIN_EMAIL` | No | — | Supply with `ADMIN_PASSWORD` to provision the initial admin |
| `ADMIN_PASSWORD` | No | — | At least 12 characters; never overwritten for an existing account |

Migrations run automatically at startup. Startup fails on invalid configuration, database errors, or migration errors. If both admin variables are omitted, the API starts without creating an admin account.

## API

Business routes use the `/api/v1` prefix. Liveness and readiness probes are `/healthz` and `/readyz`.

- Public: `POST /auth/register`, `POST /auth/login`, `GET /categories`, `GET /categories/:id`, `GET /products`, `GET /products/:id`
- Customer bearer token: `GET /cart`, `POST /cart/items`, `PATCH /cart/items/:productID`, `DELETE /cart/items/:productID`, `POST /orders`, `GET /orders`, `GET /orders/:id`
- Admin bearer token: category/product list/detail (including archived records), creation/update/archive routes, order listing/detail, and `POST /admin/orders/:id/cancel`

Prices and totals are integer USD cents. `POST /cart/items` increments a line; `PATCH` sets its quantity and rejects zero. Checkout requires a shipping address, creates a pending order, decrements stock, and clears the cart in one transaction. Admin cancellation is pending-only and restores stock once. Collections support `page` and `page_size` (default 20, maximum 100). Successful responses use a `data` envelope; failures use `{"error":{"code":"...","message":"..."}}`.

Customer registration passwords must contain 12 to 72 bytes. `POST /cart/items` returns the updated cart, and `POST /orders` returns the created order.

## Develop and test

```sh
gofmt -w .
go test ./...
go vet ./...
```
