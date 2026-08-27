## ADDED Requirements

### Requirement: Auto collection
The system SHALL support automatic collection flow: for each pressure point, automatically pressurize → wait for stability → collect data → check alarm.

#### Scenario: Start auto collection
- **WHEN** user clicks "开始采集" button in auto mode
- **THEN** system sequentially processes each pressure point with pressurize → stabilize → collect → alarm check

#### Scenario: Pause collection
- **WHEN** user clicks "暂停" button during collection
- **THEN** current collection is paused, resume can continue from current point

#### Scenario: Resume collection
- **WHEN** user clicks "恢复" button while paused
- **THEN** collection resumes from the current point

#### Scenario: Stop collection
- **WHEN** user clicks "停止" button during collection
- **THEN** collection stops immediately, all states reset

### Requirement: Manual pressurize and collect
The system SHALL support manual mode: user manually pressurizes, then manually triggers data collection.

#### Scenario: Manual pressurize
- **WHEN** user clicks "手动打压" button in manual mode
- **THEN** system sends pressurize command for current target pressure

#### Scenario: Manual collect
- **WHEN** user clicks "采集" button when pressure is stable in manual mode
- **THEN** system collects data for the current pressure point

### Requirement: Reset collection
The system SHALL allow resetting all collected data while keeping the pressure table.

#### Scenario: Reset collection
- **WHEN** user clicks "重置" button after some points are collected
- **THEN** all points' status reset to "pending", collected data cleared

### Requirement: Start enabled conditions
The start button SHALL be enabled only when devices are connected, unit consistency is valid, and pressure table is generated.

#### Scenario: Start button disabled
- **WHEN** devices not connected OR unit inconsistent OR no pressure table
- **THEN** start button is disabled
