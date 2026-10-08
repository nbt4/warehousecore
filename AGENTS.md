# AGENTS.md - AI Agent Instructions

## Operations Access

- Use the preconfigured local SSH, GitHub, Docker Hub, and Komodo sessions.
- Keep credentials in the runtime environment or a secret manager, never in repository files.

### Source Hosting
- GitHub (`github.com/nbt4`) is the only source-code remote and source of truth.
- Keep source-hosting credentials out of project instructions.

## Project: WarehouseCore
Warehouse management system for Tsunami Events.

## UI Theme

- Use only tokens from the established application theme for UI colors.
- Native selects must explicitly style both the `select` and its `option`
  elements with theme background and text colors; never rely on browser
  defaults, which can render white text on a white menu.
- The complete, mandatory suite contract lives in `../docs/DESIGN_SYSTEM.md` when checked out inside `cores` and in the canonical `nbt4/cores` repository otherwise. Read it before every UI change.
- `web/src/cores-theme.css` and `web/src/lib/cores-design.ts` are generated from the umbrella repository and must not be edited directly. Use suite tokens and `suite-*` primitives; run the umbrella sync and design checks before release.
