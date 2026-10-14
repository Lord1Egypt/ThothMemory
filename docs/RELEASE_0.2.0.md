# ThothMemory v0.2.0 — one memory for every AI agent

## What's new?

Choose how to keep your project context locally:

- **UltraLite 0.1.0:** one SQLite executable. No Docker, server or dashboard.
- **Full 0.2.0:** PostgreSQL 18 with a persistent MCP service and browser dashboard.
- **Mini 0.1.0:** one compact MariaDB container, designed for Docker-compatible
  runtimes including ThothDock. Android is a preview with documented limits.

Connected agents can save and retrieve ideas, notes, tasks, decisions, failed
attempts, evidence and handoffs. Portable archives move projects between all
three editions; this is transfer, not continuous synchronization.

## Which file should I download?

| Your computer | Recommended UltraLite asset |
|---|---|
| Windows Intel/AMD 64-bit | thothmemory-ultralite-windows-amd64.exe |
| Linux Intel/AMD 64-bit | thothmemory-ultralite-linux-amd64 |
| Linux ARM64 | thothmemory-ultralite-linux-arm64 |
| Apple Silicon | thothmemory-ultralite-darwin-arm64 (experimental, build-only) |
| Android ARM64 | thothmemory-ultralite-android-arm64 (experimental) |

Full clients are `thothmemory-linux-{amd64,arm64}` and
`thothmemory-windows-{amd64,arm64}.exe`. The source package includes the Full
Docker installer. Mini uses `thothmini.sh` plus MINI_IMAGE_DIGEST.txt; its image
repository is **lord1egypt/thoth-memory-mini**, separate from Full.

## Quick setup

Windows: open PowerShell beside the downloaded file:

```powershell
.\thothmemory-ultralite-windows-amd64.exe init
# Change to your project's folder, then use the full path to the executable:
& "$env:USERPROFILE\Downloads\thothmemory-ultralite-windows-amd64.exe" connect all
& "$env:USERPROFILE\Downloads\thothmemory-ultralite-windows-amd64.exe" instructions
& "$env:USERPROFILE\Downloads\thothmemory-ultralite-windows-amd64.exe" doctor
```

Linux (from the download directory):

```sh
mkdir -p ~/.local/bin
install -m 0755 thothmemory-ultralite-linux-amd64 ~/.local/bin/thothmemory-ultralite
~/.local/bin/thothmemory-ultralite init
# In your Git project:
~/.local/bin/thothmemory-ultralite connect all
~/.local/bin/thothmemory-ultralite instructions
~/.local/bin/thothmemory-ultralite doctor
```

Choose the `linux-arm64` file instead on ARM. Install your chosen AI client's CLI
first; restart it and approve normal project/MCP trust prompts after registration.
See the release source README, docs/GETTING_STARTED.md and docs/README_AR.md.

## Important limitations

MCP registration makes tools available; the agent protocol asks the model to save
meaningful context. It does not save every conversation. Claude lifecycle capture
was demonstrated; Codex semantic writes and Codex→Claude handoff were demonstrated,
but Codex hook capture remains unverified. OpenCode connection/plugin fixtures
passed; real-model coverage is not claimed. Native Windows command hooks are
unsupported. Prompt capture is opt-in; full transcripts are not collected.

Memory remains local, but context retrieved into a cloud AI client can be sent to
that client's provider. Secret screening reduces accidental capture, not a
guarantee. Stored content is untrusted data, not permission to execute instructions.

Full migrations 005–007 are additive and were rehearsed on disposable data;
existing installations must back up and test restore before upgrading. This
release does not automatically upgrade any owner's production database.

## Checksums and verification

Download SHA256SUMS, IMAGE_DIGEST.txt, MINI_IMAGE_DIGEST.txt and PLATFORMS.md.
Verify binaries with `sha256sum -c SHA256SUMS` (download the listed files first),
or compare `Get-FileHash -Algorithm SHA256` on Windows. The digest files pin the
Full and Mini registry manifests. Use docs/UPGRADE.md for backup and recovery.
