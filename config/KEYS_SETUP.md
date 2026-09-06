# Keys Setup Guide

This project now reads secrets only from environment variables or local `.env` files. Real keys should never be written into source code, scripts, or docs.

## Required For Competition Demo

| Variable | Source | Required | Notes |
|---|---|---|---|
| `JWT_SECRET` | self-generated | yes | Use a random string with at least 32 characters |
| `SECURITY_ENCRYPTION_SECRET` | self-generated | yes | Use a different random string with at least 32 characters for security service encryption |
| `DEMO_AUTH_PASSWORD` | self-defined | yes | Demo login password for judges and operators |
| `EMBEDDING_API_KEY` | your embedding provider console | no for local Ollama | Keep `not_required` when using local Ollama |
| `VECTOR_DB_API_KEY` or `QDRANT_API_KEY` | your vector store console | depends | Fill only for the storage mode you actually use |
| `DEEPSEEK_API_KEY` / `OPENAI_API_KEY` / `CLAUDE_API_KEY` / `QIANWEN_API_KEY` | corresponding model platform | optional | Fill only if you use a cloud LLM |

## Where To Put Them

1. Keep real values only in your local `.env`.
2. Keep placeholders in `.env.example`, `config/env.template`, and `config/.env.competition.example`.
3. Do not paste real keys into screenshots, demo scripts, or PPT.

## Generate Secrets

PowerShell:

```powershell
[guid]::NewGuid().ToString("N") + [guid]::NewGuid().ToString("N")
```

Generate one value for `JWT_SECRET` and another different value for `SECURITY_ENCRYPTION_SECRET`.

## Recommended Demo Modes

### Local-first mode

- `LLM_PROVIDER=ollama_local`
- `EMBEDDING_API_KEY=not_required`
- `QDRANT_API_KEY=` can stay empty for local Qdrant

This is the safest mode for competition demos because it supports the claim that sensitive data stays local.

### Cloud-assisted mode

- Fill `EMBEDDING_API_KEY`
- Fill the vector database key for your provider
- Fill one cloud LLM key only if the demo depends on it

Use this only when local inference quality is insufficient for the demo.
