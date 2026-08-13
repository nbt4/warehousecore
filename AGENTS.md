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
