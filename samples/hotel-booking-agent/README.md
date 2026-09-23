# Hotel Booking Agent - Deployment Guide

## Overview

The Hotel Booking Agent is an AI-powered assistant that helps users search hotels, check availability, answer policy questions, and create/edit/cancel bookings. It is built with LangGraph and FastAPI.

## Prerequisites

Before deploying this agent, ensure you have:

### Required API Keys

- **OpenAI API Key**: for model inference

Pinecone is optional and only configured on the Hotel API (see below).

### Supporting Service

- **Hotel API**: the hotel API service must be running (locally or deployed). Set `HOTEL_API_BASE_URL` to point to it.

## Deployment Instructions

### Step 1: Access Agent Manager Platform

1. Navigate to the **Default** project
2. Click **"Add Agent"**
3. Select **Platform-Hosted Agent** Card

### Step 2: Configure Agent Details

Fill in the agent creation form with these values:

| Field                 | Value                                                   |
| --------------------- | ------------------------------------------------------- |
| **Display Name**      | `Hotel Booking Agent`                                   |
| **Description**       | `AI-powered hotel booking assistant`                    |
| **GitHub Repository** | `https://github.com/wso2/agent-manager`                 |
| **Branch**            | `main`                                                  |
| **App Path**          | `samples/hotel-booking-agent/agent`                     |
| **Language**          | `Python`                                                |
| **Language Version**  | `3.11`                                                  |
| **Start Command**     | `python -m uvicorn app:app --host 0.0.0.0 --port 8000` |
| **Port**              | `8000`                                                  |

### Step 3: Select Agent Interface

- Choose **"Chat Agent"** as the agent interface type (standard `POST /chat` on port `8000`)

### Step 4: Configure Environment Variables

Add the following environment variables in the create form:

```env
OPENAI_API_KEY=<your-openai-api-key>
HOTEL_API_BASE_URL=<your-hotel-api-base-url>
```

Optional (with defaults):

```env
OPENAI_MODEL=gpt-4o-mini
WEATHER_API_KEY=
WEATHER_API_BASE_URL=http://api.weatherapi.com/v1
```

### Step 5: Deploy the Agent

1. Review all configuration details
2. Click **"Deploy"**
3. Wait for the build to complete

## Testing Your Agent

### Step 1: Navigate to Chat Interface

Click on the **"Try It"** section on the left navigation.

### Step 2: Test Sample Interactions

Try these sample questions in the chat interface:

**Hotel Search:**

```text
Find hotels in Tokyo for Feb 7 to Feb 8, 2026 for 1 guest.
```

**Availability:**

```text
Check availability for Brooklyn Heights Loft Hotel from Feb 7 to Feb 8, 2026 for 1 guest and 1 room.
```

**Booking:**

```text
Book 1 room at Brooklyn Heights Loft Hotel from Feb 7 to Feb 8, 2026 for 1 guest. Guest: Alex Doe, alex@example.com, +1-555-0100.
```

### Step 3: Observe Traces (Optional)

1. Click on the **"Observability"** tab on left navigation and select **Traces**
2. View traces

## Hotel API (Required Supporting Service)

The Hotel API must be running locally or deployed, and the agent must point to it via `HOTEL_API_BASE_URL`.

### Local Run (Hotel API)
```bash
cd samples/hotel-booking-agent/services/hotel_api
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
python -m uvicorn service:app --host 0.0.0.0 --port 9091
```

### Deploy Hotel API
Deploy the hotel API as a separate service, then set:
```env
HOTEL_API_BASE_URL=<deployed-hotel-api-base-url>
```

### Policy Search
The agent answers policy questions through the Hotel API (`GET /hotels/{hotel_id}/policies/search`). The Hotel API loads the policy PDFs in `services/hotel_api/resources/policy_pdfs/` on startup into one of two stores:

| Hotel API env | Store | Notes |
|---|---|---|
| `OPENAI_API_KEY` only | In-memory | Default. Rebuilt on every start; nothing else to set up. |
| `OPENAI_API_KEY` + `PINECONE_API_KEY` | Pinecone | Index `hotel-policies` is created (serverless, AWS `us-east-1`) and filled if missing or empty. |
| No `OPENAI_API_KEY` | None | Policy search returns 503; the rest of the API works. |

Optional Pinecone settings: `PINECONE_INDEX_NAME` (default `hotel-policies`) and `PINECONE_SERVICE_URL` (use an existing index host directly). `OPENAI_EMBEDDING_MODEL` defaults to `text-embedding-3-small`.
