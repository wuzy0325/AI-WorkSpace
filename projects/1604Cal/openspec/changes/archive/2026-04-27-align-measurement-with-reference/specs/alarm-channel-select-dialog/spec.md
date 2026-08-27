## ADDED Requirements

### Requirement: Alarm channel select dialog
The system SHALL provide a channel selection dialog for choosing which channels participate in alarm detection, with 16-channel grid, select all, and deselect all controls.

#### Scenario: Open channel select dialog
- **WHEN** user clicks "通道选择" button in the alarm settings group
- **THEN** a dialog opens showing a 4x4 grid of 16 channels with current selection highlighted

#### Scenario: Toggle individual channel
- **WHEN** user clicks a channel in the grid
- **THEN** the channel selection toggles (selected ↔ unselected)

#### Scenario: Select all channels
- **WHEN** user clicks "全选" button
- **THEN** all 16 channels become selected

#### Scenario: Deselect all channels
- **WHEN** user clicks "全不选" button
- **THEN** all channels become deselected

#### Scenario: Confirm channel selection
- **WHEN** user clicks "确定" button
- **THEN** dialog closes and enabledChannels is updated in alarm config

#### Scenario: Cancel channel selection
- **WHEN** user clicks "取消" button
- **THEN** dialog closes without changing the current selection

### Requirement: Default enabled channels
The alarm enabledChannels SHALL default to all 16 channels [0..15].

#### Scenario: Initial state
- **WHEN** alarm config is initialized
- **THEN** enabledChannels contains all 16 channels
