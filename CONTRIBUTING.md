# Contributing to mcTUI

Thanks for your interest in contributing to mcTUI. Bug reports, documentation improvements, and code contributions are welcome.

## Before you start

- Check the [documentation](https://agmonetti.github.io/mcTuiDocs/) and existing issues to see whether your question or idea is already covered.
- For a significant change, open an issue first to discuss the approach.
- Keep pull requests focused and explain the problem they solve.

## Development

1. Fork the repository and create a branch for your change.
2. Make the change, following the existing Go formatting and project conventions.
3. Run `gofmt` on changed Go files and run the relevant tests with `go test ./...`.
4. Update documentation or tests when your change affects user-facing behavior.
5. Open a pull request with a clear summary and any relevant context.

Please do not include compiled binaries, personal configuration, credentials, or unrelated generated files in a pull request.

## Reporting bugs

When opening an issue, include the operating system, mcTUI version, steps to reproduce the problem, and any relevant error output. Remove account details, tokens, and other sensitive information before sharing logs.

## License

By contributing, you agree that your contributions will be licensed under the MIT License that covers this project.
