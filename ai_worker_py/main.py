"""
Superintendent AI Worker - Gemini reasoning and ElevenLabs TTS.
"""
import base64
import hashlib
import json
import os
import time
from typing import Any

import google.generativeai as genai
from fastapi import FastAPI
from pydantic import BaseModel
import httpx
from dotenv import load_dotenv

load_dotenv()

app = FastAPI(title="Superintendent AI Worker")

GEMINI_KEY = os.getenv("GEMINI_API_KEY", "")
ELEVEN_KEY = os.getenv("ELEVEN_API_KEY", "")
ELEVEN_VOICE = os.getenv("ELEVEN_VOICE_ID", "21m00Tcm4TlvDq8ikWAM")

if GEMINI_KEY:
    genai.configure(api_key=GEMINI_KEY)


class ReasonRequest(BaseModel):
    telemetry_summary: str = ""
    recent_decisions: list[str] = []
    context: dict[str, Any] = {}


class ReasonResponse(BaseModel):
    summary: str
    risk: str
    actions: dict[str, str]
    audio_text: str
    explain: str


class SpeakRequest(BaseModel):
    text: str


class SpeakResponse(BaseModel):
    audio_url: str


REASON_CACHE_TTL = 60
_reason_cache: dict[str, tuple[float, ReasonResponse]] = {}


def _cache_key(summary: str) -> str:
    return hashlib.sha256((summary or "")[:2000].encode()).hexdigest()


REASON_PROMPT = """You are The Superintendent, an AI civic intelligence system. Analyze the following urban telemetry/sensor data and produce a structured response.

Telemetry summary:
{telemetry_summary}

Respond with valid JSON only, in this exact structure:
{{
  "summary": "1-2 sentence executive summary of current city conditions",
  "risk": "low|medium|high",
  "actions": {{
    "conservative": "Recommended cautious action",
    "aggressive": "Optional more assertive action"
  }},
  "audio_text": "A 1-2 sentence natural spoken advisory for a human operator (conversational, emotionally tuned)",
  "explain": "Brief technical explanation of your reasoning"
}}
"""


@app.post("/reason", response_model=ReasonResponse)
async def reason(req: ReasonRequest) -> ReasonResponse:
    """Call Gemini to reason over telemetry and return structured JSON."""
    key = _cache_key(req.telemetry_summary)
    now = time.time()
    if key in _reason_cache:
        ts, cached = _reason_cache[key]
        if now - ts < REASON_CACHE_TTL:
            return cached
        del _reason_cache[key]
    for k, (ts, _) in list(_reason_cache.items()):
        if now - ts >= REASON_CACHE_TTL:
            del _reason_cache[k]

    if not GEMINI_KEY:
        return ReasonResponse(
            summary="AI worker not configured (missing GEMINI_API_KEY).",
            risk="low",
            actions={"conservative": "Check configuration.", "aggressive": "Add GEMINI_API_KEY."},
            audio_text="AI reasoning is offline. Please check system configuration.",
            explain="No API key configured.",
        )

    prompt = REASON_PROMPT.format(telemetry_summary=req.telemetry_summary or "No data provided.")
    model = genai.GenerativeModel("gemini-1.5-flash")
    response = model.generate_content(prompt)

    text = response.text.strip()
    # Extract JSON from markdown code block if present
    if "```json" in text:
        text = text.split("```json")[1].split("```")[0].strip()
    elif "```" in text:
        text = text.split("```")[1].split("```")[0].strip()

    try:
        data = json.loads(text)
    except json.JSONDecodeError:
        return ReasonResponse(
            summary=text[:200] if text else "Parse error",
            risk="medium",
            actions={"conservative": "Review AI output.", "aggressive": "Retry reasoning."},
            audio_text="Unable to parse AI response. Please try again.",
            explain="Gemini returned non-JSON output.",
        )

    resp = ReasonResponse(
        summary=data.get("summary", ""),
        risk=data.get("risk", "low"),
        actions=data.get("actions", {"conservative": "", "aggressive": ""}),
        audio_text=data.get("audio_text", ""),
        explain=data.get("explain", ""),
    )
    _reason_cache[key] = (time.time(), resp)
    return resp


@app.post("/speak", response_model=SpeakResponse)
async def speak(req: SpeakRequest) -> SpeakResponse:
    """Call ElevenLabs TTS and return MP3 URL."""
    if not ELEVEN_KEY or not req.text:
        return SpeakResponse(audio_url="")

    url = f"https://api.elevenlabs.io/v1/text-to-speech/{ELEVEN_VOICE}"
    headers = {
        "Accept": "audio/mpeg",
        "Content-Type": "application/json",
        "xi-api-key": ELEVEN_KEY,
    }
    payload = {
        "text": req.text[:1000],  # Limit length
        "model_id": "eleven_monolingual_v1",
    }

    async with httpx.AsyncClient(timeout=30.0) as client:
        resp = await client.post(url, json=payload, headers=headers)
        if resp.status_code != 200:
            return SpeakResponse(audio_url="")
        b64 = base64.b64encode(resp.content).decode("ascii")
        data_url = f"data:audio/mpeg;base64,{b64}"
        return SpeakResponse(audio_url=data_url)


@app.get("/health")
async def health():
    return {"ok": True}
