# Controlled automation

Use the model to produce a proposed classification, draft, extraction, or action description. A client-owned worker validates and executes any real action.

## When it fits

- draft a customer response for review;
- extract fields from an approved document;
- classify an incoming request;
- propose an action for an existing workflow engine.

## Route

```text
input → private route → proposed structured result → client validation → approved worker action
```

## Safe first version

1. Require structured output in the client application and validate it against a schema.
2. Apply business rules outside the model.
3. Require human approval for external writes, money movement, access changes, or irreversible actions.
4. Log the input reference, proposed result, validation outcome, approver, and executed action in the client system.
5. Keep the worker's network permissions separate from the inference route.

## Not supplied by Sovereign Kit

The kit does not execute shell commands, browser actions, web searches, MCP tools, or business-system writes. It does not hold workflow credentials or make authorization decisions.

That separation is intentional: private inference does not make a tool invocation safe or authorized.
