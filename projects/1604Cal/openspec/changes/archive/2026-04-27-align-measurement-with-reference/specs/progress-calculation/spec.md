## MODIFIED Requirements

### Requirement: Progress based on current point index
The progress display SHALL be calculated as `currentPointIndex / totalPoints` instead of filtering by completed status.

#### Scenario: Progress after first point collection
- **WHEN** first point collection is complete (currentPointIndex = 1, total = 5)
- **THEN** progress shows "1/5" and 20%

#### Scenario: Progress during collection
- **WHEN** collection is ongoing (currentPointIndex = 3, total = 5)
- **THEN** progress shows "3/5" and 60%
