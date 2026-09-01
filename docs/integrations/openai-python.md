# Python OpenAI SDK

## Install

```bash
python3 -m venv .venv
. .venv/bin/activate
python -m pip install openai
```

## Configure

```bash
export OPENAI_BASE_URL=http://127.0.0.1:30000/v1
export OPENAI_API_KEY=local-qwen-tunnel
export OPENAI_MODEL=qwen3.8-27b-nvfp4
```

`OPENAI_API_KEY` is a non-secret compatibility value for the current keyless V1 route.

## Call the route

```python
import os
from openai import OpenAI

client = OpenAI(
    base_url=os.environ["OPENAI_BASE_URL"],
    api_key=os.environ["OPENAI_API_KEY"],
)

response = client.chat.completions.create(
    model=os.environ["OPENAI_MODEL"],
    messages=[{"role": "user", "content": "Summarize this approved text."}],
)
print(response.choices[0].message.content)
```

If the request cannot connect, run `sovkit doctor`. Do not replace the base URL with a public API as an automatic fallback.
