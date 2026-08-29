# CLAUDE.md - AI Agent Instructions

> **Verbindliche UI-Regel:** Vor UI-Arbeit `AGENTS.md` sowie im Umbrella-Checkout `../docs/DESIGN_SYSTEM.md` und `../theme/README.md` lesen. Generierte `cores-theme.css`-/`cores-design.ts`-Kopien nicht direkt bearbeiten; Umbrella-Sync und Designprüfung vor dem Release ausführen.

## Operations Access

- Use the preconfigured local SSH, GitHub, Docker Hub, and Komodo sessions.
- Keep credentials in the runtime environment or a secret manager, never in repository files.

### Source Hosting
- GitHub (`github.com/nbt4`) is the only source-code remote and source of truth.
- Keep source-hosting credentials out of project instructions.

## Project: WarehouseCore
Warehouse mgmt system for Tsunami Events.

---

Minimal compression — text was mostly structured credential data with no natural language filler to remove. Only `management` → `mgmt`.
