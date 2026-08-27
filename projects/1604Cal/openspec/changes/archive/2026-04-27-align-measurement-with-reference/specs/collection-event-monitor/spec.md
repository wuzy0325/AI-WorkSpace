## ADDED Requirements

### Requirement: Monitor collection events via SSE
The system SHALL listen to SSE events for collection state changes, point status updates, and point completion, syncing the frontend store accordingly.

#### Scenario: State changed event received
- **WHEN** backend sends "measurement.state_changed" event
- **THEN** frontend store's state is updated to reflect the new state

#### Scenario: Point status updated
- **WHEN** backend sends "measurement.point.status" event with point data
- **THEN** the corresponding point in the pressure points list is updated

#### Scenario: Point collected
- **WHEN** backend sends "measurement.data.collected" event with collected data
- **THEN** the point status is set to "completed" and collected data is stored

#### Scenario: Stability update received
- **WHEN** backend sends "measurement.stability.update" event
- **THEN** isStable, currentPressure, and stabilityState are updated

#### Scenario: Alarm triggered
- **WHEN** backend sends "measurement.alarm.triggered" event
- **THEN** alarmPending is set to true and alarmData is populated
