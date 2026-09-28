# Order refund sequence

```mermaid
sequenceDiagram
  actor Manager
  participant API
  participant Refunds as Refund use case
  participant DB as PostgreSQL
  participant Provider
  Manager->>API: POST refund + amount_minor + Idempotency-Key
  API->>Refunds: Authenticate permission and validate request
  Refunds->>DB: Lock paid order and payment; reserve within remaining balance
  Refunds->>Provider: Refund with stable refund idempotency key
  Provider-->>Refunds: Provider refund reference
  Refunds->>DB: Mark refund succeeded; mark payment/order REFUNDED only at zero balance; audit
  DB-->>Refunds: Commit
  Refunds-->>API: Refund result
  API-->>Manager: Created or replayed refund
```

If the provider call times out, the refund reservation stays `PENDING`. A retry with the same merchant key reuses the provider key and can finish persistence without duplicating the provider refund. Physical stock return is recorded separately after goods are received.
