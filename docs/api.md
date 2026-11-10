# PayFlow API

Base URL: `http://localhost:8080/api`

| Method | Path | Auth | Purpose |
|---|---|---|---|
| POST | `/auth/register` | No | Register user |
| POST | `/auth/login` | No | Login and receive access token |
| POST | `/accounts` | Yes | Create INR account |
| GET | `/accounts` | Yes | List own accounts |
| GET | `/accounts/{id}` | Yes | Get own account |
| POST | `/transactions/deposit` | Yes | Deposit demo funds |
| POST | `/transactions/withdraw` | Yes | Withdraw funds |
| POST | `/transactions/transfer` | Yes | Transfer funds |
| GET | `/transactions` | Yes | List transactions |
| GET | `/ledger/{account_id}` | Yes | Read ledger entries |

Money values are integer minor units (paise). For example, ₹100.00 is `10000`.

Protected requests require `Authorization: Bearer <access_token>`. Each money-changing request requires an `Idempotency-Key` header.

Deposit body:
```json
{"account_id":"<account-uuid>","amount":10000}
```

Transfer body:
```json
{"from_account_id":"<source-uuid>","to_account_id":"<destination-uuid>","amount":10000}
```
