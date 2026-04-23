# Drivers Folder Rules

Store vendor original manuals and low-level driver references here.

## Recommended Layout

```text
device-lab/drivers/
  <vendor>/
    <model>/
      manual/
```

## Examples

- `device-lab/drivers/acme/t100/manual/command-spec-v1.3.pdf`
- `device-lab/drivers/acme/t100/manual/register-map.xlsx`

Use `shared/device-sdk/docs/commands/` for normalized Markdown command specs used by code.
