# JavaScript OpenAI SDK

## Install

```bash
npm install openai
```

## Configure

```bash
export OPENAI_BASE_URL=http://127.0.0.1:30000/v1
export OPENAI_API_KEY=local-qwen-tunnel
export OPENAI_MODEL=qwen3.8-27b-nvfp4
```

`OPENAI_API_KEY` is a non-secret compatibility value for the current keyless V1 route.

## Call the route

```js
import OpenAI from "openai";

const client = new OpenAI({
  baseURL: process.env.OPENAI_BASE_URL,
  apiKey: process.env.OPENAI_API_KEY,
});

const response = await client.chat.completions.create({
  model: process.env.OPENAI_MODEL,
  messages: [{ role: "user", content: "Summarize this approved text." }],
});

console.log(response.choices[0].message.content);
```

If it fails to connect, run `sovkit doctor`; do not add a public-provider fallback.
