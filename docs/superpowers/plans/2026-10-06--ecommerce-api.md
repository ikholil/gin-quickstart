# Ecommerce API MVP Implementation Plan

Implement the approved design in the existing `gin-quickstart` module. Keep the current Gin stack, use `database/sql` with a pure-Go SQLite driver, and add only the JWT and SQLite dependencies needed. Do not add a payment, shipping, or frontend subsystem.

## 1. Establish application configuration and persistence

- Promote Gin to a direct dependency; add the pure-Go SQLite driver and JWT library. Keep bcrypt from `golang.org/x/crypto`.
- Add typed environment configuration for `PORT` (default `8080`), `DB_PATH` (default `./data/ecommerce.db`), required 32-byte-minimum `JWT_SECRET`, `JWT_TTL` (default `15m`), and the optional all-or-none admin credentials.
- Add startup validation with explicit errors; create the database parent directory; open SQLite with foreign keys, a busy timeout, and suitable local concurrency settings.
- Add embedded, versioned SQL migrations for users, categories, products, carts, cart items, orders, and order items. Add indexes and constraints for email/cart uniqueness, nonnegative stock/prices, positive quantities, and supported order states.
- Add opaque ID generation, UTC timestamps, a database health check, and a small application lifecycle with graceful HTTP shutdown.

**Verify:** configuration boundary tests; migrations apply to a fresh temporary database and safely no-op on a current schema; foreign keys and constraints are active.

## 2. Implement common HTTP and authentication foundations

- Create the versioned router, consistent success/error JSON envelopes, request decoding/validation helpers, bounded pagination parsing, request logging, recovery, `/healthz`, and `/readyz`.
- Implement customer registration/login, email normalization, bcrypt hashing/comparison, and an idempotent admin bootstrap that never promotes/overwrites an existing account.
- Sign and verify HS256 JWTs with `sub`, `role`, `iat`, and `exp`; reject other algorithms and expired/malformed tokens.
- Add bearer authentication, role middleware, and typed authenticated-user context.

**Verify:** auth/service tests, route tests for public/customer/admin access, token expiry/algorithm rejection, and no credential/secret leakage in responses.

## 3. Implement catalog and customer cart

- Add category/product repository and service operations, including public active-only lists/details, filtering, pagination, admin list/detail for active and archived records, create/update/archive, and validation of category references and price/stock values.
- Add a persistent per-customer cart with add-as-increment, set-quantity, remove, and read operations. Return server-calculated line and cart totals using integer cents.
- Ensure carts cannot be accessed across customer identities and archived products cannot be newly added or purchased.

**Verify:** repository/service/handler tests for CRUD, filtering, pagination, validation, cart add vs set semantics, removal, ownership, and archived resources.

## 4. Implement checkout and admin order operations

- Add checkout input validation and transactional service behavior: load cart, re-read active products and prices, verify stock, snapshot order lines and shipping address, compute totals, decrement stock, create a pending order, and clear the cart atomically.
- Add customer-scoped order list/detail and admin order list/detail with bounded pagination.
- Implement pending-only admin cancellation as one transaction that updates state and restores each line's stock exactly once.
- Return conflict responses for empty carts, insufficient stock, repeated cancellation, and disallowed transitions; never partially mutate inventory/orders/cart.

**Verify:** transaction rollback tests, customer order ownership, admin authorization, concurrent oversell prevention, and idempotency of cancellation attempts.

## 5. Integrate, document, and validate the deliverable

- Replace the starter `/ping` endpoint with the complete route set while preserving standard Gin startup behavior.
- Add a concise README section for configuration, local run/migration behavior, route groups, and test commands; add a `.gitignore` entry for the local SQLite database if needed.
- Add optional dotenv loading, Docker/Compose/Makefile local workflows, and an explicit `APP_ENV=development`-guarded idempotent sample-data seeder.
- Add an importable Postman v2.1 collection covering health, auth, public/admin catalog, customer cart/orders, and admin order management, with collection variables and token/resource ID scripts.
- Run formatting, focused tests as each layer lands, then `go test ./...` and `go vet ./...`.
- Manually smoke-test startup with a temporary database and explicit test JWT/admin configuration; verify liveness/readiness, registration/login, product/cart/checkout, and admin cancellation over HTTP.

**Completion gate:** all routes in the approved spec are wired, authorization/ownership is enforced, database transactions preserve inventory invariants, tests pass, and local setup is documented.
