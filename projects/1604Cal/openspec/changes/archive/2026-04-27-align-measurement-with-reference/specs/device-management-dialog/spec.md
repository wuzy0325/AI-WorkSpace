## ADDED Requirements

### Requirement: Device add/edit dialog
The system SHALL provide a DeviceFormDialog for adding and editing measurement and pressure devices from the measurement workbench.

#### Scenario: Open add device dialog
- **WHEN** user clicks "添加设备" button in the sidebar
- **THEN** DeviceFormDialog opens with mode="add" and default device type matching the section (pressure/measure)

#### Scenario: Open edit device dialog
- **WHEN** user clicks "edit" button on a device card
- **THEN** DeviceFormDialog opens with mode="edit" and current device data pre-filled

#### Scenario: Submit device form
- **WHEN** user submits the device form
- **THEN** deviceStore.addDevice(data) is called and unit consistency is rechecked

#### Scenario: Delete device
- **WHEN** user clicks "delete" button on a device card
- **THEN** deviceStore.removeDevice(deviceId) is called and unit consistency is rechecked
