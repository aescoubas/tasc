# Create Nix Native Flake Prompt

```text
Read AGENTS.md and ROADMAP.md first and follow them.

I want this repo to own a Nix-native build.

This repo appears to be primarily a Go CLI/TUI app with an additional web frontend under `web/`.

Please:
1. Inspect the current build, release, and install flow before editing.
2. Create a minimal `flake.nix` and any supporting files under `nix/`.
3. Export at least:
   - `packages.<system>.default`: the main CLI/TUI package
   - `apps.<system>.default`: runnable CLI app
   - `checks.<system>.default`: meaningful checks
   - `devShells.<system>.default`: a practical dev shell
4. If the web frontend is part of the shipped product, package it appropriately and wire it into the build. If it is only a development adjunct, do not overcomplicate the main package.
5. Prefer Nix-native builds:
   - `buildGoModule` for the Go app
   - `buildNpmPackage` for the web frontend if needed
6. Preserve the current install and runtime behavior.
7. Update README with:
   - `nix develop`
   - `nix build`
   - `nix run`
   - `nix flake check`
8. Run the existing tests/checks and `nix flake check` before finishing.

Support `x86_64-linux` and Darwin if it is straightforward.
```
