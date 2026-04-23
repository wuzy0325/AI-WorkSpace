# Hardware Command Docs

This folder stores reusable hardware command specifications shared across projects.

## What to Put Here

- Protocol command sets that can be reused by multiple services.
- Stable request/response formats and error code mappings.
- Timing, retry, and safety requirements used by adapters and simulators.

## File Naming Suggestion

- `<protocol>-<vendor>-<device>.md`
- Example: `modbus-acme-temp-sensor-t100.md`

## Template

Start from `COMMAND-SPEC-TEMPLATE.md` when adding a new device command spec.

## Current Specs

- `daq-p-1604.md` (DAQ-P-1604 pressure scanning valve command interface)
