## ADDED Requirements

### Requirement: Target value edit syncs to store
The system SHALL persist target pressure edits from the data table to the measurement store.

#### Scenario: Edit target value
- **WHEN** user edits the target value input in the pressure table
- **THEN** the store's corresponding pressure point's targetPressure is updated

## MODIFIED Requirements

### Requirement: Empty state display
The system SHALL show an empty state message when no pressure points have been generated.

#### Scenario: No points generated
- **WHEN** the pressure points list is empty
- **THEN** an el-empty component is displayed with "请配置参数并生成压力表" message
