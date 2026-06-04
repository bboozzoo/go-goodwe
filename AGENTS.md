# Agent Instructions

## Core Context
- This project is a Go implementation of a GoodWe inverter library.
- The Python implementation in `python/goodwe/` is the primary reference for protocol logic and implementation details.
- `examples/cmd/goodwe/main.go` is currently a non-functional template for a CLI client.

## Development Workflow
- **Code Quality**: 
  - All code must be `gofmt` clean.
  - Use `golangci-lint` for verification.
- **Task Tracking**: Consult `TODO.md` for the current roadmap and pending items.
- **Testing Strategy**: Refer to the Python test suite (`python/goodwe/tests/`) to understand expected behavior and protocol nuances.
