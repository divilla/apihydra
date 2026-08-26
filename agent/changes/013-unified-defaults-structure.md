## Unified Defaults Structure

- StepsDefinition.Spec.Defaults now uses domain.Defaults.
- Each `Step.Request.Defaults` now uses `domain.Defaults`, replacing the previously duplicated default-related request fields.
- Defaults propagate from directory defaults to steps-file defaults and then down to individual steps.
- Every level uses the same domain.Defaults type and structure.
- Defaults are propagated as values throughout the resolution process; `*domain.Defaults` pointers are not used.
- Remove or revise documentation that describes the previous structure or propagation behavior.

Treat skeleton/internal/domain/suite.go as the binding reference. Report any conflict between the skeleton, PRD, and specifications rather than resolving it silently.

- `Step.Response.ActualBody` uses `YAMLString` to support proper marshaling and unmarshaling.
