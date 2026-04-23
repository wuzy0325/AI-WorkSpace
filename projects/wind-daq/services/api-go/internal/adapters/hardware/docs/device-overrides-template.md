# Device Overrides Template

Use this file only for project-specific command behavior that differs from shared specs.

## Reference Shared Spec

- Shared spec path:
- Shared spec version:

## Override Scope

- Device:
- Firmware range:
- Affected commands:
- Reason for override:

## Override Details

| Command | Shared Behavior | Project Override | Reason |
|---|---|---|---|
| ReadStatus | 3 retries | 1 retry | Device watchdog reset risk |

## Validation

- Integration tests updated:
- HIL tests updated:
- Rollback plan:
