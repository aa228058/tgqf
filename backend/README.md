# Backend

FastAPI application and asynchronous Telegram workers.

## Local bootstrap

```bash
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
uvicorn app.main:app --reload --app-dir backend
```

The API is intentionally separated from Telethon worker processes. Do not place long-running Telegram client loops inside request handlers.

Before production deployment, configure environment-based settings, explicit CORS origins, administrator authentication, encrypted session storage, database migrations, and worker supervision. The current `/health` route is only a process liveness check, not a full dependency or worker health report.
