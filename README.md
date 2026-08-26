# Chirpy

A lightweight, production-grade microblogging REST API and server written in **Go**, backed by **PostgreSQL**.

---

## 📖 What Chirpy Does

Chirpy is a Twitter-like microblogging API server designed to handle user management, secure authentication, content moderation, and chirp (post) lifecycle operations:

- **User Management & Authentication**: User registration and login using industry-standard **Argon2id** password hashing, short-lived **JWT access tokens**, and database-backed **refresh tokens** with revocation and expiration support.
- **Chirp Operations**: Create, read, and delete chirps (capped at 140 characters) with built-in automated profanity filtering.
- **Filtering & Sorting**: Query chirps by author (`?author_id=...`) and sort results chronologically or reverse-chronologically (`?sort=desc`).
- **Webhooks & Premium Tiers**: Secure webhook integration with Polka payments to upgrade users to "Chirpy Red" premium membership using API key authentication.
- **Admin & Observability**: File server hit-tracking metrics via HTTP middleware and environment-restricted admin reset controls.

---

## 💡 Why Someone Should Care

- **Idiomatic Standard Library Go**: Built primarily with Go's standard library (`net/http` enhanced routing in modern Go), demonstrating clean code organization without unnecessary third-party framework overhead.
- **Production-Grade Auth Flow**: Implements a complete, secure token lifecycle (short-lived JWT access tokens + revocable refresh tokens) and Argon2id password hashing following security best practices.
- **Type-Safe Database Interactions**: Uses **Goose** for schema migrations and **sqlc** for generating compile-time type-safe Go code from raw SQL queries.
- **Clean Architecture**: Decoupled package structure separating database queries (`internal/database`), auth and cryptography utilities (`internal/auth`), and HTTP handlers (`main.go`).

---

## 🛠 Tech Stack

- **Language**: Go 1.22+
- **Database**: PostgreSQL
- **Database Tooling**: [sqlc](https://sqlc.dev/) (type-safe SQL) & [Goose](https://github.com/pressly/goose) (migrations)
- **Authentication**: JWT (`golang-jwt/jwt/v5`), Argon2id (`alexedwards/argon2id`)
- **Configuration**: `godotenv`

---

## 🚀 Getting Started

### Prerequisites

- [Go](https://golang.org/dl/) (1.22 or newer)
- [PostgreSQL](https://www.postgresql.org/) (running locally or in Docker)
- [Goose](https://github.com/pressly/goose) (CLI for database migrations)

### 1. Clone the Repository

```bash
git clone https://github.com/AJaxx86/chirpy.git
cd chirpy
```

### 2. Configure Environment Variables

Create a `.env` file in the project root:

```env
DB_URL="postgres://postgres:postgres@localhost:5432/chirpy?sslmode=disable"
SECRET="your-jwt-secret-key"
POLKA_KEY="your-polka-webhook-api-key"
PLATFORM="dev"
```

### 3. Setup Database & Run Migrations

Ensure your PostgreSQL database exists, then run the schema migrations using Goose:

```bash
cd sql/schema
goose postgres "postgres://postgres:postgres@localhost:5432/chirpy?sslmode=disable" up
cd ../..
```

### 4. Install Dependencies & Run the Server

```bash
go mod tidy
go run main.go
```

The server will start on `http://localhost:8080`.

---

## 📡 API Endpoints

### Public & Health
| Method | Endpoint | Description | Auth Required |
|---|---|---|---|
| `GET` | `/app/` | Serves static assets / web client | None |
| `GET` | `/api/healthz` | Readiness check (returns `200 OK`) | None |

### Authentication & Users
| Method | Endpoint | Description | Auth Required |
|---|---|---|---|
| `POST` | `/api/users` | Register a new user | None |
| `POST` | `/api/login` | Login with email & password (returns JWT + Refresh Token) | None |
| `PUT` | `/api/users` | Update user email & password | Bearer JWT |
| `POST` | `/api/refresh` | Issue new JWT access token using refresh token | Bearer Refresh Token |
| `POST` | `/api/revoke` | Revoke a refresh token | Bearer Refresh Token |

### Chirps
| Method | Endpoint | Description | Auth Required |
|---|---|---|---|
| `POST` | `/api/chirps` | Create a new chirp (max 140 chars, profanity filtered) | Bearer JWT |
| `GET` | `/api/chirps` | Get all chirps (supports `?author_id=` and `?sort=desc`) | None |
| `GET` | `/api/chirps/{chirpID}` | Get a single chirp by ID | None |
| `DELETE` | `/api/chirps/{chirpID}` | Delete a chirp (author only) | Bearer JWT |

### Webhooks & Administration
| Method | Endpoint | Description | Auth Required |
|---|---|---|---|
| `POST` | `/api/polka/webhooks` | Polka webhook to upgrade user to Chirpy Red | `ApiKey <POLKA_KEY>` |
| `GET` | `/admin/metrics` | View fileserver visit metrics | None |
| `POST` | `/admin/reset` | Reset metrics & clear user table (dev only) | `PLATFORM="dev"` |

---

## 🧪 Running Tests

To run unit tests across packages:

```bash
go test ./...
```

## AI Usage
AI was used for writing this README, as well as the tests, and everything else was hand-written by me.
