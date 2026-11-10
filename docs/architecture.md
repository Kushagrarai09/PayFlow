# PayFlow architecture

PayFlow is an educational microservice-style banking simulation.

## Services
- API Gateway: forwards public `/api/...` routes to service processes.
- Auth: registration and login; issues HS256 JWT access tokens.
- Account: account lifecycle, balance reads, locked balance adjustment, adjustment idempotency.
- Transaction: deposit, withdrawal and transfer orchestration.
- Ledger: balanced double-entry journal batches.

## Storage boundaries
PostgreSQL is split into four databases: `payflow_auth`, `payflow_accounts`, `payflow_transactions`, and `payflow_ledger`.

Redis and Kafka containers are included in Compose as infrastructure for future caching and asynchronous event delivery. The current MVP does not yet integrate application producers/consumers with them.

## Transfer flow
1. The transaction service checks its idempotency key.
2. It requests a debit from the account service.
3. For transfers, it requests a recipient credit.
4. It posts a balanced journal batch to the ledger service.
5. It records the transaction result.
6. If a later step fails, the service attempts compensating account adjustments.

This is a simplified Saga demonstration, not durable orchestration. Real financial systems need durable workflow state, transactional outbox, ledger-first accounting, reconciliation, and recovery tooling.

## Safety
This repository is for local learning only. It is not authorized for real funds or production banking.
