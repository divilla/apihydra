## Enhanced Debug

- Debug output is complete and unredacted. It never hides, masks, filters, or
  omits any member or value, including security-sensitive values such as
  `Authorization: Bearer ...` headers and cookie-jar contents when present.
- Remove every implemented security mechanism that prevents Debug from
  displaying the complete members and values defined by this change.
- A Debug dump uses exactly this layout:

  ```text
  stage: <Step.DirectoryStage()>
  dir-path: <Step.DirectoryPath()>
  file-path: <Step.FilePath()>

  curl-command:
  <raw-curl-command>

  <step-json>
  ```

- `<raw-curl-command>` is the exact Curl command executed by `apih`. It is not
  altered, redacted, hidden, stringified, destringified, or otherwise
  transformed.
- The displayed Curl command can be copied, pasted, and executed as-is, and is
  the same command that `apih` executes for the step.
- `<step-json>` is the raw JSON encoding of `Step`. It contains the latest
  possible runtime values captured immediately before the step finishes or
  processing stops because of a terminal error.
- Remove or revise documentation and tests that require the previous Debug
  layout, jq normalization or colorization, or suppression of members and
  values.

Treat `skeleton/internal/domain/suite.go`,
`skeleton/internal/reporting/reporter.go`, and `skeleton/pkg/runner/runner.go` as
the binding references. Report any conflict between the skeleton, PRD, and
specifications rather than resolving it silently.
