# MODELS

## Provider Strategy
- Primary provider: `openai`
- Baseline model: `gpt-5-codex`
- Local fallback behavior: `echo`

## Runtime Guidance
- Use strict timeout and error classification.
- Return concise fallback messages on provider failures.
