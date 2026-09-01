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

- The user-maintained skeleton/ directory is read-only and defines the 
  repository’s binding architecture, API, and complete external contract.
- All shared/public types, methods, and functions must be declared in 
  skeleton/. Before creating or changing such an item anywhere, obtain the
  user’s agreement, update skeleton/ first, then align the PRD, specifications,
  documentation, tests, mocks, scaffolding, and implementation.
- If any artifact conflicts with skeleton/, or an alternative design seems 
  preferable, stop and report it. Continue only after the user decides whether 
  to align the artifact with skeleton/ or explicitly revise skeleton/.
- Deviations and one-off exceptions are prohibited. Approval of an alternative
  authorizes revising skeleton/, after which every affected artifact must be 
  aligned before the work is complete.

## Permissions

- Never modify this `AGENTS.md` file or any file or directory under `skeleton/`
  unless the user explicitly directs that specific protected path to be
  changed.
- A general request to implement, fix, refactor, format, test, or document the
  project is not explicit authorization to modify `AGENTS.md` or `skeleton/`.
