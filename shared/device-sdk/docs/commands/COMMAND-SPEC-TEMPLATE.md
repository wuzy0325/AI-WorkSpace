# Device Command Spec Template

## 1. Document Metadata

- Device Name:
- Vendor:
- Model:
- Protocol:
- Spec Version:
- Firmware Version Range:
- Last Updated:
- Owner:

## 2. Communication Profile

- Physical Link: (UART / RS485 / CAN / TCP / BLE)
- Framing: (RTU / ASCII / custom frame)
- Endianness:
- Baud/Bitrate:
- Address/Node ID Rules:
- Timeout (ms):
- Retry Policy:

## 3. Command List

| Command Name | Code | Direction | Description | Idempotent |
|---|---|---|---|---|
| ReadStatus | 0x01 | Request/Response | Read current status | Yes |

## 4. Command Details

### 4.1 <Command Name>

- Code:
- Purpose:
- Preconditions:
- Side Effects:
- Safety Notes:

#### Request

- Fields:

| Field | Type | Size | Required | Range | Notes |
|---|---|---|---|---|---|
| example | uint16 | 2 bytes | Yes | 0-1000 | sample |

- Binary Layout (if needed):

```text
Byte0 Byte1 Byte2 Byte3
```

#### Response (Success)

- Fields:

| Field | Type | Size | Meaning | Notes |
|---|---|---|---|---|
| ok | uint8 | 1 byte | 1 means success | sample |

#### Response (Error)

| Error Code | Meaning | Recoverable | Client Action |
|---|---|---|---|
| 0xE1 | Invalid parameter | Yes | Fix parameter and retry |

## 5. State Machine Impact

- Device states affected by each command.
- Allowed command order.
- Busy/locked behavior.

## 6. Timing and Reliability

- Typical response time:
- P95 response time:
- Max response time:
- Timeout recommendation:
- Backoff strategy:

## 7. Security and Safety

- Authentication or key requirements.
- Risky commands and protection rules.
- Audit logging requirements.

## 8. Examples

### 8.1 Raw Hex Example

```text
Request : 01 03 00 00 00 02 C4 0B
Response: 01 03 04 00 64 00 C8 BA 12
```

### 8.2 Pseudocode Example

```text
send(ReadStatus)
if timeout then retry up to 3 times
if error 0xE1 then stop and report invalid input
```

## 9. Simulator Expectations

- Cases to support in simulator:
  - normal response
  - timeout
  - CRC/frame error
  - device busy
  - invalid data

## 10. Test Matrix

| Test Case | Layer | Input | Expected Output |
|---|---|---|---|
| ReadStatus normal | integration | valid node id | parsed status |
| ReadStatus timeout | integration | delayed reply | timeout error |
| ReadStatus invalid CRC | integration | bad frame | frame error |
| Critical command on locked state | hil | lock enabled | rejected |

## 11. Compatibility Notes

- Known differences by firmware version.
- Deprecated commands.
- Migration guidance.
