## ADDED Requirements

### Requirement: Auto-save alarm config on change
The system SHALL automatically save alarm configuration to the backend when any alarm setting changes, with 250ms debounce.

#### Scenario: Alarm enabled toggled
- **WHEN** user toggles alarm "启用" checkbox
- **THEN** after 250ms debounce, saveMeasurementAlarmConfig is called with current settings

#### Scenario: Sound enabled toggled
- **WHEN** user toggles alarm "声音" checkbox
- **THEN** after 250ms debounce, saveMeasurementAlarmConfig is called with current settings

#### Scenario: Confirm on alarm toggled
- **WHEN** user toggles alarm "报警确认" checkbox
- **THEN** after 250ms debounce, saveMeasurementAlarmConfig is called with current settings

#### Scenario: Enabled channels changed
- **WHEN** user confirms channel selection in the channel select dialog
- **THEN** after 250ms debounce, saveMeasurementAlarmConfig is called with current settings

#### Scenario: Multiple rapid changes
- **WHEN** user changes multiple settings rapidly
- **THEN** only one save call is made 250ms after the last change
