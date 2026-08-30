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

- `<raw-curl-command>` contains the complete Curl executable and arguments used
  by `apih`. It is not redacted or hidden. An empty body adds no data argument;
  a body of up to 1,024 Unicode characters is the final `--data-binary` value;
  and a longer body uses final value `@-`. `runner.CurlRaw` attempts jq compact
  formatting for a non-`@-` final data value, retains the original on invalid
  JSON or jq failure, POSIX-quotes header and data values, and encodes embedded
  single quotes as `'\''`. All other values remain untransformed, and execution
  receives the original arguments and request body.
- `<step-json>` preserves every member and value from `Step`, projecting only
  valid JSON in the request, expected-response, and actual-response body strings
  as structured JSON before prettifying and coloring the result. Empty or
  invalid JSON bodies remain strings. It contains the latest possible runtime
  values captured immediately before the step finishes or processing stops
  because of a terminal error.
- Remove or revise documentation and tests that require the previous Debug suppression of
  members and values.

Treat `skeleton/internal/domain/suite.go`,
`skeleton/internal/reporting/reporter.go`, and `skeleton/pkg/runner/runner.go` as
the binding references. Report any conflict between the skeleton, PRD, and
specifications rather than resolving it silently.
