# OAuth adapter (MVP)

Minimal server-side OAuth client for linking accounts (Spotify + GitHub).
This is intentionally small for learning.

## What this does
- Redirects the user to provider consent (Spotify or GitHub)
- Exchanges code for tokens
- Stores tokens in SQLite
- Fetches provider data (Spotify recently played, GitHub user profile)

## Setup
1. Create a Spotify app (optional) and/or a GitHub OAuth App.
2. Copy `config.example` to `.env` and fill in values.
3. Install deps and run:
   ```bash
   pip install -r requirements.txt
   uvicorn main:app --reload --port 8080
   ```

## Docker (dev)
1. Create `.env` from `config.example`.
2. Run:
   ```bash
   docker compose up --build
   ```
3. Edit code locally; changes auto-reload inside the container.

## Endpoints
- `POST /users` → create an app user (returns `app_user_id`)
- `GET /connect/spotify?app_user_id=...` → redirect to Spotify auth
- `GET /oauth/spotify/callback` → exchange code and store tokens
- `GET /spotify/recently-played?app_user_id=...` → fetch data
- `GET /connect/github?app_user_id=...` → redirect to GitHub auth
- `GET /oauth/github/callback` → exchange code and store tokens
- `GET /github/user?app_user_id=...` → fetch data

## Notes
- This uses the Authorization Code flow with client secret.
- For production, add PKCE, stronger state storage, and encryption at rest.


