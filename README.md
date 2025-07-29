# Go LLM Chat – Multi-LLM Orchestrator Demo

This project is a proof-of-concept chat server written in Go. It shows how to:

1. Orchestrate multiple LLMs (OpenAI ChatGPT models) with different prompts.
2. Stream answers to the caller using Server-Sent Events (SSE).
3. Enrich answers with domain data (a MongoDB collection of fictional flight data).
4. Run everything locally with **Docker Compose**.

---

## Architecture

```mermaid
graph TD
    subgraph Client
        A[Browser / curl]
    end

    subgraph Server (Go)
        direction TB
        H((/api)) -->|SSE| A
        H[HTTP Handler]
        H --> O[Orchestrator]
        O -->|prompt| L1[(LLM 1)]
        O -->|prompt| L2[(LLM 2)]
        O -->|future| L3[(LLM 3 – coming soon)]
        O -->|query| DB[(MongoDB flights)]
    end

    subgraph Docker Compose
        DB ---|network| Server
    end"```

* **LLM 1** – concise, formal replies (or *list of flights* when the question is about flights).
* **LLM 2** – verbose, friendly replies (or *duration & cost* when the question is about flights).
* **LLM 3** – aggregation layer (not implemented yet).

When the user’s question mentions *flights* (in EN or ES) the orchestrator:

1. Extracts **origin** / **destination** city names using a simple synonym map.
2. Queries MongoDB for matching flights (case-insensitive, supports wildcard searches).
3. Feeds the flight list to both LLMs with different prompts.
4. Streams back a combined answer via SSE.

For non-flight questions, LLM1/LLM2 are just given the user’s question with their respective style prompts.

---

## Getting Started

### Prerequisites

* Go 1.22+
* Docker & Docker Compose (optional but easiest)
* An **OpenAI API key** (set `OPENAI_API_KEY`).

### Clone & run with Docker Compose (recommended)

```bash
# 1. Clone the repo
$ git clone https://github.com/<you>/go-llm-chat.git
$ cd go-llm-chat

# 2. Export your OpenAI key (or use a .env file)
$ export OPENAI_API_KEY="sk-…"

# 3. Start everything
$ docker-compose up --build
```

Docker Compose spins up:

* **MongoDB** on `mongodb://mongo:27017` (aliased as `MONGO_URI`).
* **Go server** on `http://localhost:8080`.

On first start the server **seeds** the `flightdb.flights` collection with a set of 20 sample flights (Madrid ↔ Paris, London ↔ Berlin, Tokyo → LA, …). Seeding is done via **upsert**, so re-starts won’t duplicate data.

### Run natively (Go only)

1. Start MongoDB locally (`brew services start mongodb-community@7` or similar).
2. Export environment variables:
   ```bash
   export MONGO_URI="mongodb://localhost:27017"
   export OPENAI_API_KEY="sk-…"
   ```
3. Run the server:
   ```bash
   go run ./cmd/server
   ```

---

## API

`POST /api` with **plain-text** body. The response is an **SSE** stream.

### Events

| Event `Type` | Meaning                               | Example `Data`                   |
|--------------|---------------------------------------|----------------------------------|
| `Status`     | Internal status update (invoking LLM) | `Invoking LLM 1`                 |
| `Message`    | Final combined answer                 | See example below                |

### Curl Examples

List all flights:
```bash
curl -N -X POST -d "Que vuelos hay en general?" http://localhost:8080/api
```

Ask for specific route:
```bash
curl -N -X POST -d "hay vuelos a londres?" http://localhost:8080/api
```

General (non-flight) question:
```bash
curl -N -X POST -d "Explain quantum teleportation in simple terms" http://localhost:8080/api
```

The `-N` flag keeps the connection open so you see the `Status` events followed by the `Message`.

---

## Project Layout

```
cmd/
  server/            # main.go – HTTP + SSE + orchestration wiring
internal/
  db/                # MongoDB client, models & seed data
  llmclient/         # Thin wrapper around OpenAI ChatCompletion
  orchestrator/      # Core logic (detect flights, prompt LLMs, merge)
  sse/               # Minimal SSE helper
Dockerfile           # Builds the Go binary for prod
Docker-compose.yml   # Mongo + server services
```