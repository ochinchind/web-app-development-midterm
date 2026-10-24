# Cargo Transport Service API

Base URL: `http://localhost:8080` · Interactive docs (Swagger UI): `http://localhost:8081`

## Authentication

Register or log in to get a JWT, then send it on every protected request:

```
Authorization: Bearer <access_token>
```

## Endpoints

| Method | Path | Auth | Description |
|--------|------|:----:|-------------|
| POST | `/v1/auth/register` | – | Create an account |
| POST | `/v1/auth/login` | – | Log in with email or phone |
| GET | `/v1/users/me` | ✔ | Current user |
| POST | `/v1/organizations` | ✔ | Create organization |
| GET | `/v1/organizations` | – | List organizations |
| GET | `/v1/organizations/{id}` | – | Get organization |
| PUT | `/v1/organizations/{id}` | ✔ | Rename (members only) |
| DELETE | `/v1/organizations/{id}` | ✔ | Delete (members only) |
| POST | `/v1/vehicles` | ✔ | Create my vehicle |
| GET | `/v1/vehicles` | ✔ | List my vehicles |
| GET | `/v1/vehicles/{id}` | ✔ | Get my vehicle |
| PUT | `/v1/vehicles/{id}` | ✔ | Update my vehicle |
| DELETE | `/v1/vehicles/{id}` | ✔ | Delete my vehicle |
| POST | `/v1/orders` | ✔ | Create order (status `NEW`) |
| GET | `/v1/orders` | ✔ | List orders (filters below) |
| GET | `/v1/orders/{id}` | ✔ | Get order |
| PUT | `/v1/orders/{id}` | ✔ | Update order (creator, while `NEW`) |
| DELETE | `/v1/orders/{id}` | ✔ | Delete order (creator, while `NEW`) |
| POST | `/v1/orders/{id}/take` | ✔ | Driver takes order: `NEW → TAKEN` |
| POST | `/v1/orders/{id}/deliver` | ✔ | Driver completes order: `TAKEN → DELIVERED` |

---

### Auth

#### `POST /v1/auth/register`

```json
{ "name": "Alice", "email": "alice@example.com", "phone": "+77015551234", "password": "secret123", "organization_id": "" }
```

Response `200`:

```json
{
  "access_token": "eyJhbGciOi...",
  "token_type": "Bearer",
  "expires_in": 86400,
  "user": { "id": "…", "name": "Alice", "email": "alice@example.com", "phone": "+77015551234", "organization_id": "", "created_at": "2026-01-01T10:00:00Z" }
}
```

#### `POST /v1/auth/login`

```json
{ "phone": "+77015551234", "password": "secret123" }
```

---

### Organizations

`Organization`: `{ "id", "name", "created_at" }`

- `POST /v1/organizations`: body `{ "name": "Acme Logistics" }`. The creator joins the organization if they have none yet.
- `GET /v1/organizations`: `{ "organizations": [ … ] }`
- `GET /v1/organizations/{id}`: one organization, `404` if unknown.
- `PUT /v1/organizations/{id}`: body `{ "name": "New name" }`. `403` if the caller is not a member.
- `DELETE /v1/organizations/{id}`: returns `{}`. `403` if the caller is not a member. Members and orders keep existing; their `organization_id` becomes empty.

---

### Vehicles

A driver is simply a user with at least one vehicle. Vehicles are private to their owner (other users get `404`).

`Vehicle`: `{ "id", "owner_id", "plate_number", "model", "capacity_kg", "created_at" }`

- `POST /v1/vehicles`: body `{ "plate_number": "123ABC", "model": "Volvo FH16", "capacity_kg": 20000 }`. The plate is upper-cased and must be unique (`409`).
- `GET /v1/vehicles`: `{ "vehicles": [ … ] }` (only mine)
- `GET /v1/vehicles/{id}`
- `PUT /v1/vehicles/{id}`: same body as create (full replace)
- `DELETE /v1/vehicles/{id}`: returns `{}`

---

### Orders

Lifecycle: `NEW → TAKEN → DELIVERED`.

`Order`:

```json
{
  "id": "…", "created_by": "…", "organization_id": "",
  "from_address": "Almaty, Abay Ave 1", "to_address": "Astana, Mangilik El Ave 5",
  "cargo_description": "10 pallets of tiles", "weight_kg": 1500,
  "status": "NEW", "driver_id": "", "vehicle_id": "",
  "created_at": "…", "updated_at": "…", "taken_at": "…", "delivered_at": "…"
}
```

`taken_at` / `delivered_at` are only present once set. `organization_id` is copied from the creator.

- `POST /v1/orders`: body `{ "from_address", "to_address", "cargo_description", "weight_kg" }`. Addresses are required and `weight_kg` must be > 0.
- `GET /v1/orders`: `{ "orders": [ … ] }`, newest first. Query parameters:

  | Param | Description |
  |-------|-------------|
  | `status` | `NEW`, `TAKEN` or `DELIVERED` |
  | `created_by_me` | `true` → only orders I created |
  | `taken_by_me` | `true` → only orders where I am the driver |
  | `limit` / `offset` | Paging (default limit 50, max 100) |

  Drivers look for work with `GET /v1/orders?status=NEW`.
- `GET /v1/orders/{id}`
- `PUT /v1/orders/{id}`: same body as create (full replace). Only the creator, and only while `NEW` (`403` / `400` otherwise).
- `DELETE /v1/orders/{id}`: returns `{}`. Same rules as update.
- `POST /v1/orders/{id}/take`: body `{ "vehicle_id": "<one of my vehicles>" }`. Sets `status=TAKEN`, `driver_id`, `vehicle_id` and `taken_at`. Returns `400` if the order is not `NEW`, you created it yourself, or the vehicle is not yours. The check is atomic, so two drivers cannot take the same order.
- `POST /v1/orders/{id}/deliver`: body `{}`. Sets `status=DELIVERED` and `delivered_at`. Returns `403` if you are not the driver, `400` if the order is not `TAKEN`.
