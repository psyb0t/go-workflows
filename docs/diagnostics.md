# Diagnostics

`diag.NewServeMux(backend)` returns an HTTP mux for the embedded UI and read-only JSON API. The backend must implement `diag.Backend`. The application chooses whether to expose it and owns the HTTP server lifecycle.

The mux does not authenticate users, restrict tenants or redact stored payloads. Protect the entire mount, including `/api/`, with application middleware or an authenticated reverse proxy. Worker names and queues are not access-control boundaries. Bind loopback for local-only inspection.

## Mounting

Mounted directly, the UI lives at `/` and the API at `/api/`. To mount under `/diag/`, use `http.StripPrefix("/diag", diag.NewServeMux(backend))` as the handler for that prefix. See the [web sample](../samples/web/) for application wiring.

Set server timeouts, limit exposure and stop the HTTP server during application shutdown. More than one application can expose the same persisted data; diagnostics does not select a leader or own workflow execution.

![Workflow diagnostics](images/diag-list.png)

## API

Paths below are relative to the diagnostics mount. API methods are GET only; other methods return 405.

| Path | Response |
| --- | --- |
| `/api/` | Array of workflow execution references |
| `/api/stats` | Backend workflow and activity statistics |
| `/api/{instanceID}/{executionID}` | Execution reference and recorded history |
| `/api/{instanceID}/{executionID}/tree` | Workflow tree rooted at that execution |

Listing uses `count` and `after`. The default count is 25. Construct the next cursor from the last returned execution as `instanceID:executionID`. The response is an array, not a `total`/`hasMore` envelope. Continue until no further records are returned. The current parser does not provide a strict positive count bound or reject every malformed cursor; validate client input in an application wrapper when exposing the API beyond trusted operators.

Detail and tree lookups return 404 when the execution cannot be obtained. Storage or history failures can return 500. Treat this as a diagnostics surface, not a versioned application API contract.

History attributes may include workflow inputs, activity results and error details. Avoid storing secrets in them and do not publish screenshots or responses without checking their contents.

## Metrics and tracing

`backend.WithMetrics` accepts an implementation of `backend/metrics.Client`. `backend.WithTracerProvider` accepts an OpenTelemetry tracer provider. Both default to no-op implementations. The host application configures exporters and any metrics endpoint; the diagnostics mux does not create them.

Use [scope and logging](guide.md#scope-and-logging) for workflow/activity context propagation. Use [UI development](../diag/app/README.md) for the embedded React application and asset build.
