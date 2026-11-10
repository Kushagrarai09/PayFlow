# PayFlow — Banking & Payment Processing Simulation

A runnable educational full-stack banking simulation built with Go microservices, PostgreSQL (database per service), Redis, Kafka, Docker Compose, and React/Vite.

> **Safety:** This is a learning project. It is not a bank, payment institution, or production money-movement system. Do not connect real funds or store real financial credentials. A production system requires regulated banking/payment partners, independent security review, compliance controls, operational controls, and much more rigorous financial-ledger design.

## Architecture

- **API Gateway**: public HTTP entry point and path-based routing.
- **Auth service**: user registration/login and signed JWT access tokens.
- **Account service**: account creation, balance reads, and idempotent internal balance adjustments.
- **Transaction service**: deposit, withdrawal and transfer orchestration with compensation on partial failure.
- **Ledger service**: append-only, balanced double-entry journal batches.
- **PostgreSQL**: one logical database per service (`payflow_auth`, `payflow_accounts`, `payflow_transactions`, `payflow_ledger`).
- **Redis and Kafka**: included in local infrastructure for extension and event-driven workflows.
- **Web dashboard**: React/Vite UI for registration, login, accounts, deposits, withdrawals, transfers and history.

The services are independently runnable from the same Go module. The gateway routes public requests to the service processes. The transaction service communicates with account and ledger services over HTTP.

## Quick start

Requirements: Docker Desktop with Compose.

```bash
cp .env.example .env
docker compose up --build
```

Open:
- Web dashboard: http://localhost:5173
- API gateway: http://localhost:8080/health
- Gateway API: http://localhost:8080/api/...

The first startup creates the service databases and tables automatically.

## Local development without Docker

You need Go 1.23+, PostgreSQL, Redis and Kafka. Create four PostgreSQL databases and set the DSNs in `.env`.

```bash
go mod download
go test ./...
```

Start each service in a separate terminal:

```bash
SERVICE=auth PORT=8081 go run ./cmd/service
SERVICE=account PORT=8082 go run ./cmd/service
SERVICE=ledger PORT=8083 go run ./cmd/service
SERVICE=transaction PORT=8084 go run ./cmd/service
GATEWAY_PORT=8080 AUTH_URL=http://localhost:8081 ACCOUNT_URL=http://localhost:8082 LEDGER_URL=http://localhost:8083 TRANSACTION_URL=http://localhost:8084 go run ./cmd/gateway
```

Start the frontend:

```bash
cd web
npm install
npm run dev -- --host 0.0.0.0
```

## Demo flow

1. Register a user and log in.
2. Create an account.
3. Deposit demo funds.
4. Create a second account using another user.
5. Transfer funds between accounts.
6. Inspect transaction history and ledger entries.

All amounts are integer minor units (paise) in the API and database. The UI accepts rupees and converts to paise. This avoids floating-point arithmetic for money.

## API overview

All public endpoints are under `/api`.

| Method | Endpoint | Description |
|---|---|---|
| POST | `/api/auth/register` | Create user |
| POST | `/api/auth/login` | Login, receive JWT access token |
| POST | `/api/accounts` | Create account |
| GET | `/api/accounts` | List current user's accounts |
| GET | `/api/accounts/{id}` | Get account |
| POST | `/api/transactions/deposit` | Deposit demo funds |
| POST | `/api/transactions/withdraw` | Withdraw funds |
| POST | `/api/transactions/transfer` | Transfer between accounts |
| GET | `/api/transactions` | List user's transactions |
| GET | `/api/ledger/{account_id}` | Read ledger entries |

Protected requests use `Authorization: Bearer <token>`. Financial write requests require an `Idempotency-Key` header.

## Engineering notes and limitations

- The ledger uses integer minor units and rejects unbalanced journal batches.
- Account adjustments are guarded by a database transaction and row lock; negative balances are rejected.
- Adjustment idempotency is stored in the account service database.
- Transaction orchestration uses compensating actions if later steps fail. This is a simplified Saga, not a complete durable workflow engine.
- The gateway performs routing; protected services validate JWTs themselves.
- JWT uses HS256 with a configurable secret. Passwords use PostgreSQL pgcrypto bcrypt.
- Kafka and Redis are provisioned but not yet integrated for durable event delivery/caching in this MVP. The next extension is transactional outbox + Kafka consumer groups + Redis rate limiting.
- For production-grade accounting, ledger entries should be the source of truth, with materialized balances and reconciliation.
- Add TLS, request limits, structured logs, metrics, tracing, refresh-token rotation, fraud controls, KYC/AML integrations, backups, disaster recovery and penetration testing before any production use.

## Tests

```bash
go test ./...
```

Tests cover amount validation, balanced journal validation, idempotency-key generation and gateway helpers. Integration tests against PostgreSQL are an important next step.
