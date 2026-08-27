## ADDED Requirements

### Requirement: Export report dialog
The system SHALL provide an export dialog for generating calibration reports with path selection, template info, point count, and pressure mode display.

#### Scenario: Open export dialog
- **WHEN** user clicks "导出报告" button after collection is completed
- **THEN** a dialog opens showing export path selector, report template name, calibration point count, and pressure mode

#### Scenario: Select export path
- **WHEN** user clicks "选择路径" in the export dialog
- **THEN** a folder selection dialog opens for choosing the export destination

#### Scenario: Export report
- **WHEN** user clicks "导出报告" after selecting a path
- **THEN** the system generates and saves the report file, showing success message

#### Scenario: Cancel export
- **WHEN** user clicks "取消"
- **THEN** dialog closes without exporting
