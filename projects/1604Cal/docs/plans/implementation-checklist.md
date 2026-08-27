# 实施检查清单

## 1. 开发前

- [ ] 已阅读 `AGENTS.md` 规范
- [ ] 已阅读 `docs/plans/2026-04-10-unified-calibration-design.md`
- [ ] 已阅读 `docs/plans/2026-04-10-unified-calibration-implementation-plan.md`

## 2. 代码质量

- [ ] Go 代码通过 `go test ./...`
- [ ] Go 代码通过 `go vet ./...`
- [ ] 前端通过 `npm --prefix web run typecheck`
- [ ] 前端通过 `npm --prefix web run lint`
- [ ] 前端通过 `npm --prefix web run test`

## 3. 关键功能检查

- [ ] 健康接口 `/api/v1/health` 可访问
- [ ] SSE 接口 `/api/v1/events/stream` 可接收事件
- [ ] 设备单位一致性检查可运行
- [ ] 会话状态机合法迁移可验证
- [ ] 模板选择 `2-11 + s/m` 规则正确

## 4. 文档与交接

- [ ] 变更点补充至设计文档或实施计划
- [ ] Session Handoff 内容已更新

## 5. 连接可靠性配置

- [ ] 已确认是否需要设置 `CAL1604_CONFIG`
- [ ] 配置文件已基于 `configs/app.example.json` 生成并校验
- [ ] timeout/retry 参数与现场网络条件匹配
