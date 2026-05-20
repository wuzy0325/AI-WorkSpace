# Wind-DAQ Claude Rules

Single source of truth: `../../CLAUDE.md` and `../../AGENTS.md`.

## Project Addendum

### UI Design

All frontend UI work must follow `DESIGN.md` in this project root.

### Migration Strategy

This project is being rebuilt from the TS/Electron reference. Do not copy the old Electron IPC architecture. Implement backend behavior in Go hexagonal layers and frontend behavior in Vue/Wails.
