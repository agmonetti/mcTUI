# Development, checks, and releases

mcTUI is a Go desktop application built with Bubble Tea. Distribution happens
through GitHub Releases; there is no application server to deploy. The separate
documentation site is maintained in `mcTuiDocs` and is outside these workflows.

## Project map

| Area | Responsibility |
| --- | --- |
| `main.go` | Startup, configuration, localization, update notification, TUI |
| `internal/ui`, `internal/i18n` | Screens, event handling, English/Spanish translations |
| `internal/auth`, `internal/config` | Microsoft/offline accounts and local settings |
| `internal/launcher`, `internal/java`, `internal/mojang`, `internal/fabric` | Runtime preparation, downloads, Java detection, game processes |
| `internal/instances`, `internal/worlds`, `internal/resetconfigs` | Local instances, world copies, configuration resets |
| `internal/modrinth`, `internal/mods`, `internal/packs`, `internal/modpacks` | Content discovery, installation, recovery, import/export |
| `internal/operationlock` | Exclusive operations using OS-specific locks |
| `internal/updater` | GitHub latest-release notification using `main.Version` |

## Continuous integration

`CI` runs on branch pushes, pull requests, manual dispatch, and every Monday at
09:23 UTC. The weekly run also checks newly disclosed vulnerabilities without
requiring a code change. A newer run on the same ref cancels the previous run.

The reusable `validate.yml` workflow performs:

- `gofmt`, `go mod verify`, and a clean `go mod tidy` check.
- Workflow validation with actionlint.
- `go vet` and uncached tests on Linux, macOS, and Windows. Linux also runs the
  race detector. Existing tests that require POSIX shell fixtures skip Windows.
- govulncheck for reachable vulnerabilities in the Linux build and its Go
  standard library. This does not scan every OS-specific execution path.
- Cross-compilation of Linux amd64/arm64, macOS amd64/arm64, and Windows amd64.

Go comes from `go.mod`, keeping local builds, tests, and release builds aligned.
The baseline was updated to Go 1.26.10 and `golang.org/x/text` v0.39.0 to address
reachable vulnerabilities detected while introducing these checks.
Coverage reports and build outputs are retained as workflow artifacts for seven
days. CI artifacts use version `dev`, which disables update notifications.
Cross-compilation checks compilation; native matrix tests exercise OS behavior.
On macOS, CI uses the canonical runner temporary directory so symlink rejection
does not mistake the system `/var` alias for an unsafe application directory.
Interactive TUI, live Microsoft login, and launching real Minecraft still need
manual smoke testing before a release.

Actions are pinned to commit SHAs. Dependabot opens weekly PRs for Go modules and
Actions; review them through the same CI. The actionlint and govulncheck versions
are pinned directly in `validate.yml` and must be updated there when needed.
Only the release publication job receives `contents: write`; no custom secret or
personal access token is needed.

## Publish a release

After merging and smoke testing, tag the intended commit on `main`:

```bash
git switch main
git pull --ff-only
git tag -a v1.6.0 -m 'Release v1.6.0'
git push origin v1.6.0
```

Use the next unused version. Tags such as `v1.6` remain supported for compatibility
with existing releases. Use `v1.6.0-rc.1` for a prerelease.

`Release` checks that the tag points to a commit on `main`, then runs all the
validation jobs against that exact tag. Only if every job passes does it:

1. Download the compiled binaries and verify `checksums.txt`.
2. Create a draft GitHub Release with the binaries, checksums, and generated notes.
3. Publish it, marking stable releases as latest and prereleases as prerelease.

Builds inject the tag through `-X main.Version=...`, enabling the existing update
notification. Binaries retain the existing `mctui-<os>-<arch>` naming convention;
Windows keeps `.exe` and its checked-in amd64 icon resource. Linux arm64 is also
included. Executables are unsigned; macOS notarization and Windows code signing
are not configured.

If validation fails, fix the problem and release a new tag. If publication fails
after creating a draft, inspect and delete that incomplete draft before rerunning
the failed job. Existing releases are never overwritten automatically. Do not
move published tags.

To reproduce the build locally on Linux with Bash and Go installed:

```bash
go vet ./...
go test -race -count=1 -timeout=10m ./...
bash scripts/build-release.sh v1.6.0
(cd dist && sha256sum --check checksums.txt)
```

Outputs are written to ignored `dist/`. Running the script without a version
builds `dev` binaries. The script uses `sha256sum` and is intended for Linux/CI;
macOS users need GNU coreutils to run it unchanged.

On Windows, modpack file contents are flushed before publication, but directory
metadata cannot be flushed through Go's POSIX-style directory sync. Recovery
journals remain enabled; equivalent durability across sudden power loss is not
guaranteed on Windows.

## GitHub repository settings

After the first successful CI run, configure a ruleset for `main` to require PRs
and these checks (select the names shown by GitHub):

- Format and dependencies
- Test (ubuntu-latest)
- Test (macos-latest)
- Test (windows-latest)
- Vulnerabilities
- Build release binaries

Workflows report checks; requiring them before merging is a repository setting.
Restrict creation/deletion of `v*` tags to release maintainers using a tag ruleset.
Ensure repository Actions policy allows the pinned official `actions/*` actions
and GitHub Release creation using `GITHUB_TOKEN`. These settings are not modified
by the files in this change.
