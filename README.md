# Ecommerce API

A single-process Gin ecommerce API backed by a local SQLite database. Payment processing and shipping-carrier integrations are not included.

## Run locally

Use Go 1.27.1 or newer. Create a local environment file from the template:

```sh
cp .env.example .env
```

Set `JWT_SECRET` in `.env` to a random value with at least 32 bytes. For example, generate one with:

```sh
openssl rand -hex 32
```

Then start the server:

```sh
go run main.go
# or
make run
```

The app automatically reads `.env` from the current working directory. Existing environment variables override values in that file, so this works with shells and deployment platforms too. A missing `.env` is allowed if configuration is supplied by the environment. `.env` is local-only and ignored by Git.

Configuration:

| Variable | Required | Default | Notes |
|---|---|---|---|
| `APP_ENV` | No | `production` | Set to `development` in local `.env`; required by the development seeder |
| `PORT` | No | `8080` | Listen port (1-65535) |
| `DB_PATH` | No | `./data/ecommerce.db` | SQLite file path; parent directory is created |
| `JWT_SECRET` | Yes | — | At least 32 bytes; HS256 signing key |
| `JWT_TTL` | No | `15m` | Positive Go duration |
| `ADMIN_EMAIL` | No | — | Supply with `ADMIN_PASSWORD` to provision the initial admin |
| `ADMIN_PASSWORD` | No | — | At least 12 characters; never overwritten for an existing account |

Migrations run automatically at startup. Startup fails on invalid configuration, database errors, or migration errors. If both admin variables are omitted, the API starts without creating an admin account.

## Run with Docker Compose

Compose requires a local `.env` file:

```sh
cp .env.example .env
# Set JWT_SECRET and, optionally, both ADMIN_EMAIL and ADMIN_PASSWORD in .env.
docker compose up --build
```

The SQLite database persists in the `ecommerce-data` named volume. The API runs as a non-root user and Compose checks `/readyz` for health. Use `docker compose down` to stop it, or `docker compose down -v` only when you intentionally want to delete the local database.

## Seed local test data

With `APP_ENV=development` configured in `.env`, run:

```sh
make seed
```

This idempotently creates three categories, twelve products with varied prices/stock, and two demo customers. It also provisions the configured initial admin if supplied. It refuses to run in `test` or `production`, and app startup never inserts sample records automatically.

For the Compose named volume instead, use `make seed-compose`; it runs the same guarded seeder against the database used by the container.

Demo customer accounts:

| Email | Password |
|---|---|
| `alex@example.test` | `demo-alex-password` |
| `sam@example.test` | `demo-sam-password` |

These credentials and records are for local development only. The seed command resets stock and reactivates its own sample products each time it runs.

The local `.env` created in this workspace also provisions `admin@example.com` with password `local-dev-admin-password`. This is an insecure development-only credential; replace it for any non-local environment.

## Postman

Import [`postman/ecommerce-api.postman_collection.json`](./postman/ecommerce-api.postman_collection.json) into Postman. Start the local API and run `make seed`. In the collection's **Variables** tab, set `adminEmail` and `adminPassword` to the matching values from `.env`; the customer variables default to the seeded Alex account. Change `baseUrl` if the API is not listening at `http://localhost:8080`.

All request URLs use the `baseUrl` collection variable directly (for example, `{{baseUrl}}/api/v1/products`) so Postman can resolve the host immediately after import without relying on nested variables.

Run the collection with the Collection Runner in order. Login requests save `customerToken` and `adminToken`; create requests save resource IDs for subsequent calls. The register request creates a fresh uniquely named test customer each time. The admin catalog requests create a new Postman category and products when run, so reruns add more demo catalog entries. Checkout creates a pending order; the final admin request cancels it and restores inventory. Seed data again with `make seed` to reset seeded product stock.

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
make build
make seed
```
