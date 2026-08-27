## ADDED Requirements

### Requirement: 报告服务 MUST 提供模板列表查询
报告服务 MUST 扫描模板目录并返回可用模板列表。模板命名 MUST 支持 `{pointCount}{modeSuffix}.xlsx` 规则，其中 `modeSuffix` 为 `s`(single) 或 `m`(roundTrip)。

#### Scenario: 获取模板列表
- **WHEN** 客户端调用模板列表接口
- **THEN** 系统 MUST 返回每个模板的名称、点数、模式与文件路径信息

#### Scenario: 跳过不合法模板文件
- **WHEN** 模板目录中存在不符合命名规则的文件
- **THEN** 系统 MUST 忽略该文件且不影响合法模板返回

### Requirement: 报告服务 MUST 按点数与模式匹配模板
报告服务 MUST 根据输入的 `pointCount` 与 `mode` 生成目标文件名并匹配模板路径：`single -> s`，`roundTrip -> m`。若模板不存在，系统 MUST 返回明确错误或受控默认模板。

#### Scenario: 匹配单程模板
- **WHEN** 输入 `pointCount=6` 且 `mode=single`
- **THEN** 系统 MUST 尝试匹配 `6s.xlsx` 并返回其路径

#### Scenario: 匹配回程模板
- **WHEN** 输入 `pointCount=6` 且 `mode=roundTrip`
- **THEN** 系统 MUST 尝试匹配 `6m.xlsx` 并返回其路径

#### Scenario: 模板不存在
- **WHEN** 输入条件对应模板文件不存在
- **THEN** 系统 MUST 返回模板未找到错误或配置的默认模板路径
