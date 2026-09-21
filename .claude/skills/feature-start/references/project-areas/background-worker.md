# Project Areas — Background Worker / Job Service

Use this file to populate the `index.md` layers table and assess phase scope
for background worker and job service projects.

| Area | Examples |
|---|---|
| Job definition | Job type, payload schema, retry policy |
| Queue / broker | Queue registration, topic/subscription setup |
| Message contract | Event schema, versioning |
| Processing logic | Handler, transformer, aggregator |
| External calls | API clients, webhooks, downstream services |
| Error handling | Dead-letter queue, poison message strategy |
| Idempotency | Deduplication keys, at-least-once guarantees |
| State persistence | DB writes, cache updates |
| Configuration | Worker concurrency, timeout, schedule |
| Observability | Logging, metrics, alerting, tracing |
| Tests | Unit, integration, message contract |
| Deployment | Container config, scaling, scheduling |