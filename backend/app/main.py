from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

app = FastAPI(
    title="TG Manager API",
    version="0.1.0",
    description="管理经授权的 Telegram 用户账号与自动化任务。",
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=[],  # Configure explicit frontend origins via settings before deployment.
    allow_credentials=True,
    allow_methods=["GET", "POST", "PUT", "PATCH", "DELETE"],
    allow_headers=["Authorization", "Content-Type"],
)


@app.get("/health", tags=["system"])
async def health_check() -> dict[str, str]:
    return {"status": "ok", "service": "tg-manager-api"}
