# Phase 6: Distribution and Polish - Research

**Researched:** 2026-04-17
**Domain:** Flox build/publish/install pipeline for Go binary distribution
**Confidence:** HIGH

## Summary

Phase 6 delivers the final distribution step: publishing the sandflox Go binary to FloxHub so users can install it via `flox install`. The existing `.flox/pkgs/sandflox.nix` Nix expression (created and validated in Phase 1) already handles hermetic source selection (`lib.fileset.toSource`) and reproducible builds (`-trimpath`), satisfying DIST-05. The remaining work is: (1) fix the FloxHub environment linkage that blocks `flox build`, (2) push all 46 unpushed commits to the git remote, (3) run `flox build` to verify the build with current source, (4) run `flox publish` to upload to the FloxHub catalog, and (5) verify `flox install sandflox` works in a fresh environment.

A critical blocker was discovered: the `.flox/env.json` file has been locally modified from a path environment (`{"name":"sandflox","version":1}`) to a FloxHub-linked environment (`{"owner":"8BitTacoSupreme",...}`), causing `flox build` to fail with "Cannot build from an environment on FloxHub." The fix is straightforward -- revert `env.json` to its committed state, which is already a valid local path environment. Additionally, the branch is 46 commits ahead of `origin/main` -- `flox publish` requires the current revision to exist on the remote.

**Primary recommendation:** Revert `env.json` to committed state, push all commits to origin, then run `flox build sandflox && flox publish sandflox` followed by `flox install 8BitTacoSupreme/sandflox` in a fresh environment to verify.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
None -- all implementation choices are at Claude's discretion for this infrastructure phase.

### Claude's Discretion
All implementation choices are at Claude's discretion -- pure infrastructure phase. Use ROADMAP phase goal, success criteria, and codebase conventions to guide decisions.

### Deferred Ideas (OUT OF SCOPE)
None -- infrastructure phase.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| DIST-02 | sandflox publishes to FloxHub via `flox publish` | Publish workflow fully documented: revert env.json, push to remote, `flox publish sandflox`. Auth confirmed (8BitTacoSupreme). |
| DIST-03 | sandflox is installable into any Flox environment via `flox install sandflox` | After publish, package appears as `8BitTacoSupreme/sandflox` in catalog; install via `flox install 8BitTacoSupreme/sandflox`. |
| DIST-05 | Nix expression uses `lib.fileset.toSource` for hermetic source selection and `-trimpath` in build flags | Already implemented in `.flox/pkgs/sandflox.nix` from Phase 1. Verified present in current file. |
</phase_requirements>

## Standard Stack

### Core
| Tool | Version | Purpose | Why Standard |
|------|---------|---------|--------------|
| Flox CLI | 1.11.2 | Build, publish, install workflow | Verified installed; provides `flox build`, `flox publish`, `flox install` |
| Go | 1.26.x (via Flox) | Compile sandflox binary | Provided by manifest `go.pkg-path = "go"`; go.mod specifies `go 1.22` minimum |
| Nix (via Flox) | -- | Backend for `buildGoModule` derivation | Flox delegates to Nix for `.flox/pkgs/` nix expression builds |

### Supporting
| Tool | Version | Purpose | When to Use |
|------|---------|---------|-------------|
| git | system | Push commits to remote before publish | `flox publish` requires current revision on remote |
| `sandbox-exec` | macOS system | Verify binary works after install | Installed binary should function end-to-end |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `.flox/pkgs/` nix expression | `[build]` table in manifest.toml | Manifest builds cannot use `lib.fileset.toSource` (DIST-05 requires it); nix expression approach is mandatory |
| `--stability` flag | No flag | Blog and docs show `flox build`/`flox publish` without `--stability` for `.flox/pkgs/` builds; flag is optional for pinning base package set |

## Architecture Patterns

### Build and Publish Workflow
```
1. Ensure .flox/env.json is a local path environment
2. Ensure all tracked files are committed and pushed to remote
3. flox build sandflox         # builds from .flox/pkgs/sandflox.nix
4. flox publish sandflox       # uploads to FloxHub catalog as 8BitTacoSupreme/sandflox
5. flox install 8BitTacoSupreme/sandflox  # verify in fresh env
```

### Existing Nix Expression (`.flox/pkgs/sandflox.nix`)
```nix
{ buildGoModule, lib }:

buildGoModule {
  pname = "sandflox";
  version = "0.1.0";

  src = lib.fileset.toSource {
    root = ../../.;
    fileset = lib.fileset.unions [
      (lib.fileset.fileFilter (file: lib.hasSuffix ".go" file.name) ../../.)
      ../../go.mod
    ];
  };

  vendorHash = null;  # zero external dependencies (CORE-01)
  env.CGO_ENABLED = "0";  # static binary, no C dependencies
  buildFlags = [ "-trimpath" ];  # reproducible builds, no local path leaks
  ldflags = [ "-s" "-w" "-X main.Version=0.1.0" ];
  doCheck = false;

  meta = with lib; {
    description = "macOS-native sandbox for AI coding agents";
    license = licenses.mit;
    platforms = platforms.darwin;
  };
}
```

### Key Files
```
.flox/
  env.json           # MUST be local path format: {"name":"sandflox","version":1}
  env/
    manifest.toml    # Minimal: only go in [install], no [build] table needed
    manifest.lock    # Auto-generated lockfile
  pkgs/
    sandflox.nix     # Nix expression with buildGoModule (already complete)
```

### Anti-Patterns to Avoid
- **Mixing `.flox/pkgs/` with `[build]` table:** Nix expression build names cannot conflict with manifest `[build]` entries. Since we use `.flox/pkgs/sandflox.nix`, do NOT add a `[build.sandflox]` entry to manifest.toml.
- **Publishing from a FloxHub-linked environment:** `flox build` fails with "Cannot build from an environment on FloxHub." The env.json must describe a local path environment.
- **Publishing before pushing:** `flox publish` clones the repo from the remote and does a clean build. If the current commit is not on the remote, publish fails.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Go binary compilation for Nix | Custom Makefile/build script | `buildGoModule` in Nix expression | Handles Go module proxy, caching, cross-compilation, CGO; battle-tested in nixpkgs |
| Hermetic source selection | Manual file copying | `lib.fileset.toSource` | Precisely controls which files enter the Nix build sandbox; prevents build-path leaks |
| Binary stripping and version embedding | Manual `go build` flags | `ldflags = ["-s" "-w" "-X main.Version=..."]` in Nix expression | Consistent across all builds |
| Package distribution | Manual binary hosting | `flox publish` + FloxHub catalog | Handles closure uploading, package metadata, catalog indexing, cross-platform distribution |

**Key insight:** The entire build-to-install pipeline is handled by three Flox commands (`build`, `publish`, `install`). No custom distribution infrastructure is needed.

## Common Pitfalls

### Pitfall 1: FloxHub-Linked Environment Blocks Build
**What goes wrong:** `flox build` returns "Cannot build from an environment on FloxHub" and exits.
**Why it happens:** The `.flox/env.json` file contains `"owner"` and `"floxhub_url"` fields, making Flox treat it as a remote-managed environment. Builds are only supported for local path environments.
**How to avoid:** Ensure `env.json` contains only `{"name": "sandflox", "version": 1}` (no `owner` or `floxhub_url` fields). The committed version in git is already correct; the working copy was modified by a `flox push` or `flox pull` operation.
**Warning signs:** `cat .flox/env.json` shows `"owner"` field.

### Pitfall 2: Unpushed Commits Block Publish
**What goes wrong:** `flox publish` fails because it clones the repo from the remote and cannot find the current source revision.
**Why it happens:** The git precondition requires the current commit to exist on the remote. Currently 46 commits ahead of `origin/main`.
**How to avoid:** Run `git push origin main` before `flox publish`.
**Warning signs:** `git status` shows "Your branch is ahead of 'origin/main' by N commits."

### Pitfall 3: Untracked or Uncommitted .flox Files
**What goes wrong:** `flox publish` fails with "environment pointer file not found" or similar.
**Why it happens:** `flox publish` clones the repo to a temp directory and does a clean build. All `.flox/` files (env.json, manifest.toml, manifest.lock, pkgs/sandflox.nix) must be committed and pushed.
**How to avoid:** Ensure `git status` shows no uncommitted changes under `.flox/` before publishing. The `.flox/env.lock` file (untracked) is a FloxHub artifact and should NOT be committed -- it will not exist once env.json is reverted to local path format.
**Warning signs:** `git status` shows modified or untracked files under `.flox/`.

### Pitfall 4: Large Closure Upload Timeout
**What goes wrong:** `flox publish` times out or fails with "ExpiredToken" during upload.
**Why it happens:** The published package includes runtime dependencies (the "closure"). If the environment has many packages, the closure can be large.
**How to avoid:** The sandflox manifest is minimal (only `go` in `[install]`), and `go` is only a build-time dependency. The built binary has zero runtime dependencies (`CGO_ENABLED=0`, static binary). The closure should be very small. If timeout occurs, retry -- subsequent attempts often succeed.
**Warning signs:** Very long upload time during `flox publish`.

### Pitfall 5: env.lock Confusion
**What goes wrong:** The `.flox/env.lock` file (tracking FloxHub revision) is confused with `.flox/env/manifest.lock` (Nix lockfile).
**Why it happens:** FloxHub-linked environments create `.flox/env.lock` to track the remote revision. This file does not exist for local path environments.
**How to avoid:** After reverting env.json to local path format, delete `.flox/env.lock` (it's untracked). Do NOT confuse it with `.flox/env/manifest.lock` which is committed and required.
**Warning signs:** Untracked `.flox/env.lock` file in git status.

### Pitfall 6: Package Naming After Publish
**What goes wrong:** User tries `flox install sandflox` but the package is actually `8BitTacoSupreme/sandflox`.
**Why it happens:** Published packages are namespaced under the publisher's catalog (user handle or org name).
**How to avoid:** After publishing, use `flox search sandflox` to confirm the fully qualified name, then install with `flox install 8BitTacoSupreme/sandflox`.
**Warning signs:** `flox install sandflox` returns "package not found" or installs something else.

## Code Examples

### Reverting env.json to Local Path Environment
```bash
# Check current state
cat .flox/env.json
# If it shows "owner" field, restore from git:
git restore .flox/env.json
# Verify
cat .flox/env.json
# Should show: {"name": "sandflox", "version": 1}
```

### Build and Publish Workflow
```bash
# 1. Ensure env.json is local path (no "owner" field)
git restore .flox/env.json

# 2. Remove FloxHub artifact
rm -f .flox/env.lock

# 3. Ensure all code is committed and pushed
git push origin main

# 4. Build the package
flox build sandflox
# Output: result-sandflox -> /nix/store/...-sandflox-0.1.0

# 5. Verify the built binary
./result-sandflox/bin/sandflox validate -policy policy.toml

# 6. Publish to FloxHub catalog
flox publish sandflox
# Package uploaded as 8BitTacoSupreme/sandflox

# 7. Verify it appears in catalog
flox search sandflox
# Should show: 8BitTacoSupreme/sandflox
```

### Verify Installation in Fresh Environment
```bash
# Create a temporary test environment
cd /tmp
mkdir sandflox-test && cd sandflox-test
flox init
flox install 8BitTacoSupreme/sandflox

# Verify the binary is available
flox activate -- sandflox validate -policy /path/to/policy.toml

# Cleanup
cd /tmp && rm -rf sandflox-test
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `flox build` with `.flox/pkgs/` only | `flox build` supports both `[build]` table and `.flox/pkgs/` | Flox 1.10+ | Nix expressions still fully supported; manifest builds added as simpler alternative |
| Manual binary distribution | `flox publish` + FloxHub catalog | Flox 1.6+ | Packages discoverable via `flox search`, installable via `flox install` |

## Open Questions

1. **Package visibility after publish**
   - What we know: By default, packages published to a user's personal catalog are visible only to that user. Sharing requires an organization (Flox for Teams paid feature).
   - What's unclear: Whether the `8BitTacoSupreme` catalog allows public access or requires org setup.
   - Recommendation: Proceed with personal catalog publish first. If broader distribution is needed, investigate Flox for Teams organization setup separately. This is outside the DIST-02/03 requirements which just say "publishes to FloxHub" and "installable via flox install" -- both work with personal catalogs.

2. **Darwin-only platform restriction**
   - What we know: The `sandflox.nix` meta specifies `platforms = platforms.darwin`. Users on Linux cannot install it.
   - What's unclear: Whether `flox install` on Linux will give a clear error or silently fail.
   - Recommendation: This is by design (sandflox is macOS-only). No action needed for v1.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Flox CLI | Build/publish/install | Yes | 1.11.2 | -- |
| Go (via Flox) | Nix build of sandflox | Yes | 1.26.x (in Flox catalog) | -- |
| git | Push commits, publish precondition | Yes | system | -- |
| FloxHub auth | `flox publish` | Yes | Authenticated as 8BitTacoSupreme | -- |
| GitHub remote | `flox publish` precondition | Yes | origin: 8BitTacoSupreme/sandflox.git | -- |

**Missing dependencies with no fallback:** None.

**Missing dependencies with fallback:** None.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib), go 1.26 |
| Config file | None (Go convention: `*_test.go` files) |
| Quick run command | `go test ./... -count=1 -short` |
| Full suite command | `go test ./... -count=1 -v` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DIST-02 | `flox publish` uploads sandflox to FloxHub | manual | `flox publish sandflox` (side-effecting, cannot automate in test suite) | N/A -- manual verification |
| DIST-03 | `flox install sandflox` makes command available | manual | `flox install 8BitTacoSupreme/sandflox && sandflox validate` (requires fresh env) | N/A -- manual verification |
| DIST-05 | Nix expression uses `lib.fileset.toSource` and `-trimpath` | unit (static check) | `grep -q 'fileset.toSource' .flox/pkgs/sandflox.nix && grep -q 'trimpath' .flox/pkgs/sandflox.nix` | Yes -- `.flox/pkgs/sandflox.nix` |

### Sampling Rate
- **Per task commit:** `go test ./... -count=1 -short`
- **Per wave merge:** `go test ./... -count=1 -v`
- **Phase gate:** Full suite green + successful `flox build sandflox` + `flox publish sandflox` + `flox install` verification

### Wave 0 Gaps
None -- existing test infrastructure covers code requirements. DIST-02 and DIST-03 are inherently manual (side-effecting operations on external services) and cannot be covered by automated unit tests.

## Sources

### Primary (HIGH confidence)
- `flox publish --help` and `man flox-publish` -- Preconditions: git repo, clean tree, remote pushed, at least one package
- `flox build --help` and `man flox-build` -- Supports both `[build]` table and `.flox/pkgs/` nix expressions
- `man manifest.toml` -- `[build]` table syntax and relationship to nix expression builds
- `.flox/env.json` committed vs working copy -- Confirmed local path env format vs FloxHub-linked format
- `.flox/pkgs/sandflox.nix` -- Existing nix expression already satisfies DIST-05

### Secondary (MEDIUM confidence)
- [Flox Build and Publish Tutorial](https://flox.dev/docs/tutorials/build-and-publish/) -- Complete workflow example
- [Introducing Flox Build and Publish (blog)](https://flox.dev/blog/introducing-flox-build-and-publish/) -- Preconditions and git requirements
- [Reproducible Builds Made Simple (blog)](https://flox.dev/blog/reproducible-builds-made-simple-with-nix-and-flox/) -- Nix expression build + publish workflow example
- [Nix Expression Builds (docs)](https://flox.dev/docs/concepts/nix-expression-builds/) -- `.flox/pkgs/` naming conventions and git tracking requirements
- [FloxHub Environment Concepts](https://flox.dev/docs/concepts/environments/) -- Path vs FloxHub environment distinction

### Tertiary (LOW confidence)
- [Discourse: Issues with flox publish](https://discourse.flox.dev/t/issue-s-with-flox-publish/1152) -- Community reports on large closure timeouts and auth issues

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- Flox CLI verified installed and authenticated; build/publish/install commands documented in man pages
- Architecture: HIGH -- Existing `.flox/pkgs/sandflox.nix` already complete; workflow is three CLI commands
- Pitfalls: HIGH -- FloxHub env.json blocker confirmed by direct testing; git push requirement confirmed by man page preconditions

**Research date:** 2026-04-17
**Valid until:** 2026-05-17 (Flox CLI stable; sandflox.nix unchanged)
