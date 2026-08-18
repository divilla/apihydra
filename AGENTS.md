# Repository Instructions

This file provides guidance to AGENTS when working with code in this repository.

## About This Project

APIHydra, with `apih` as its application name, is an API Integration Tester
designed with an agent-first philosophy.

## Development Commands

The project uses a Makefile for common development tasks:

- `make check` - Run linting, vetting, and race condition tests (default target)
- `make init` - Install required linting tools (golint, staticcheck)
- `make lint` - Run goimports, staticcheck and golint
- `make vet` - Run go vet
- `make test` - Run short tests
- `make race` - Run tests with race detector
- `make benchmark` - Run benchmarks
- `make coverage` - Display test coverage

Example commands for development:
```bash
# Setup development environment
make init

# Run all checks (lint, vet, race)
make check

# Run specific tests
go test ./middleware/...
go test -race ./...

# Run benchmarks
make benchmark
```

## Unit-test requirements

- Implement unit tests for all production code and keep unit-test coverage
  greater than 95%.
- Avoid tests that do not add coverage unless a test exists to prove a specific
  Acceptance Criteria bullet.
- Every individual Acceptance Criteria bullet must have at least one unit test
  implemented and maintained in the test suite.

## User-maintained reference skeleton

- The user maintains the `skeleton/` directory. It contains the reference
  architecture and API for this repository.
- Treat the entire `skeleton/` directory as read-only reference material.
- Treat `skeleton/` as the binding reference architecture and API. All agent
  work must align completely with it and obey its contracts.
- `skeleton/` reflects the complete external contract: all shared types, public
  types, public methods, and public functions must be declared there. An agent
  must not design, specify, document, test, mock, scaffold, or implement any new
  item in those categories before consulting the user and obtaining agreement
  on the corresponding change to `skeleton/`. The agreed skeleton change must
  be made before the new contract item is produced anywhere else in the
  repository.
- `skeleton/` is the primary and authoritative source of truth for the
  repository. The PRD, specifications, documentation, tests, and implementation
  code must all match the skeleton reference.
- When the PRD, a specification, documentation, a test, or implementation code
  conflicts with `skeleton/`, do not treat the conflicting artifact as authority
  and do not silently reconcile the difference. Report the mismatch to the user.
  Resolve it either by changing every conflicting artifact to match `skeleton/`,
  or, only when the user explicitly directs a change to `skeleton/`, by changing
  the skeleton reference and bringing every affected artifact into alignment
  with the revised reference.
- If an agent has doubts about the reference, identifies a conflict, or believes
  any part of the work would be better implemented differently, the agent must
  stop, explain the concern and proposed alternative to the user, and obtain the
  user's explicit decision before continuing.
- Deviation from `skeleton/` is never allowed. User approval of an alternative
  authorizes changing the skeleton reference itself, not implementing an
  exception to it. After any such change, the PRD, specifications,
  documentation, tests, and implementation code must all match the updated
  skeleton before the work is complete.
- Never modify this `AGENTS.md` file or any file or directory under `skeleton/`
  unless the user explicitly directs that specific protected path to be
  changed.
- A general request to implement, fix, refactor, format, test, or document the
  project is not explicit authorization to modify `AGENTS.md` or `skeleton/`.
