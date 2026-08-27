## ADDED Requirements

### Requirement: Check unit consistency before generating pressure table
The system SHALL verify device unit consistency before generating the pressure table, showing a warning if units are inconsistent.

#### Scenario: Unit consistent
- **WHEN** user clicks "生成压力表" and all connected devices have consistent units
- **THEN** pressure table is generated normally

#### Scenario: Unit inconsistent
- **WHEN** user clicks "生成压力表" and device units are inconsistent
- **THEN** a warning is shown and the user is advised to fix units before proceeding

### Requirement: Recheck unit consistency after device changes
The system SHALL recheck unit consistency after adding, editing, or deleting a device.

#### Scenario: After device add
- **WHEN** a new device is added via DeviceFormDialog
- **THEN** deviceStore.checkUnitConsistency() is called

#### Scenario: After device delete
- **WHEN** a device is deleted
- **THEN** deviceStore.checkUnitConsistency() is called
