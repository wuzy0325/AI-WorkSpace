"""
读取 e2e-report.json，生成 E2E 测试报告（HTML + Markdown）。
HTML 中截图使用相对路径 e2e-test-cases/<file>，与 e2e-report.html 同目录。
"""
from __future__ import annotations
import json
from datetime import datetime
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent  # projects/daq-p1604
SHOT_DIR = ROOT / "docs" / "test-result" / "e2e-test-cases"
REPORT_JSON = ROOT / "docs" / "test-result" / "e2e-report.json"
HTML_OUT = ROOT / "docs" / "test-result" / "e2e-report.html"
MD_OUT = ROOT / "docs" / "test-result" / "e2e-report.md"

DEVICE_IP = "192.168.1.7"
DEVICE_PORT = "9000"
DEVICE_NAME = "DAQ-P-1604-374D87"
TOTAL_CASES_IN_DOC = 194  # test-cases.html 中的用例总数

MODULE_NAMES = {
    "CONN": "连接管理", "ACQ": "数据采集", "CFG": "通道配置",
    "MON": "实时监控", "REC": "CSV 录制", "LOG": "日志系统",
    "UI": "界面布局", "SCAN": "设备扫描", "STAB": "稳定性",
}

def esc(s: str) -> str:
    return (str(s).replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;"))

def badge(res: str) -> str:
    cls = {"pass": "ok", "fail": "bad", "skip": "skip", "block": "skip"}.get(res, "skip")
    label = {"pass": "PASS", "fail": "FAIL", "skip": "SKIP", "block": "BLOCK"}.get(res, res.upper())
    return f'<span class="b b-{cls}">{label}</span>'

def main() -> None:
    data = json.loads(REPORT_JSON.read_text(encoding="utf-8"))
    cases = data.get("cases", [])
    total = data.get("total", len(cases))
    npass = data.get("pass_count", sum(1 for c in cases if c["result"] == "pass"))
    nfail = data.get("fail_count", sum(1 for c in cases if c["result"] == "fail"))
    nskip = data.get("skip_count", sum(1 for c in cases if c["result"] == "skip"))
    nblock = data.get("block_count", sum(1 for c in cases if c["result"] == "block"))

    by_mod: dict[str, list] = {}
    for c in cases:
        by_mod.setdefault(c["module"], []).append(c)

    now = datetime.now().strftime("%Y-%m-%d %H:%M:%S")

    # ---- module summary rows ----
    mod_rows = ""
    for mod in sorted(by_mod.keys()):
        lst = by_mod[mod]
        mp = sum(1 for c in lst if c["result"] == "pass")
        mf = sum(1 for c in lst if c["result"] == "fail")
        ms = sum(1 for c in lst if c["result"] == "skip")
        name = MODULE_NAMES.get(mod, mod)
        cls = "ok" if mf == 0 else "bad"
        mod_rows += (
            f'<tr><td><b>{esc(mod)}</b> {esc(name)}</td>'
            f'<td>{len(lst)}</td><td class="c-ok">{mp}</td>'
            f'<td class="c-bad">{mf}</td><td class="c-skip">{ms}</td>'
            f'<td><span class="b b-{cls}">{ "通过" if mf==0 else "有问题" }</span></td></tr>'
        )

    # ---- case rows ----
    case_rows = ""
    for c in cases:
        shot = c.get("screenshot") or ""
        if shot and (SHOT_DIR / shot).exists():
            img = f'<a href="e2e-test-cases/{esc(shot)}"><img src="e2e-test-cases/{esc(shot)}" alt="{esc(c["tc_id"])}"></a>'
        else:
            img = '<span class="noimg">—</span>'
        remark = esc(c.get("remark") or "")
        case_rows += (
            f'<tr><td><b>{esc(c["tc_id"])}</b></td>'
            f'<td>{esc(c["name"])}</td>'
            f'<td>{badge(c["result"])}</td>'
            f'<td class="remark">{remark}</td>'
            f'<td class="shot">{img}</td></tr>'
        )

    overall_cls = "ok" if nfail == 0 else "bad"
    overall_label = "全部通过 ✓" if nfail == 0 else f"{nfail} 个用例失败"

    cov = f"{total}/{TOTAL_CASES_IN_DOC}"

    html = f"""<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>DAQ-P-1604 E2E 测试报告</title>
<style>
  :root{{--bg:#f5f6f8;--card:#fff;--bd:#e3e6ea;--ink:#1f2430;--mut:#6b7280;--ok:#0a8f5c;--bad:#e5484d;--skip:#b7791f;}}
  *{{box-sizing:border-box}}
  body{{margin:0;background:var(--bg);color:var(--ink);font:14px/1.6 -apple-system,"Segoe UI",Roboto,"PingFang SC","Microsoft YaHei",sans-serif;}}
  .wrap{{max-width:1180px;margin:0 auto;padding:28px 22px 60px;}}
  h1{{font-size:23px;margin:0 0 4px;}}
  .sub{{color:var(--mut);margin:0 0 22px;font-size:13px;}}
  .cards{{display:flex;gap:14px;flex-wrap:wrap;margin-bottom:26px;}}
  .card{{background:var(--card);border:1px solid var(--bd);border-radius:12px;padding:16px 20px;min-width:130px;flex:1;}}
  .card .n{{font-size:30px;font-weight:700;line-height:1.1;}}
  .card .l{{color:var(--mut);font-size:12.5px;margin-top:3px;}}
  .card.ok .n{{color:var(--ok)}} .card.bad .n{{color:var(--bad)}} .card.skip .n{{color:var(--skip)}}
  .verdict{{padding:13px 16px;border-radius:10px;margin-bottom:24px;font-weight:600;font-size:15px;}}
  .verdict.ok{{background:#e7f8f0;color:#0a8f5c;border:1px solid #b7ecd6;}}
  .verdict.bad{{background:#fdecec;color:#e5484d;border:1px solid #f5c6c6;}}
  h2{{font-size:16px;margin:26px 0 12px;border-left:4px solid var(--ok);padding-left:10px;}}
  table{{width:100%;border-collapse:collapse;background:var(--card);border:1px solid var(--bd);border-radius:10px;overflow:hidden;}}
  th,td{{text-align:left;padding:9px 11px;border-bottom:1px solid #eef0f3;vertical-align:top;}}
  th{{background:#f0f2f5;font-size:12.5px;color:var(--mut);font-weight:600;}}
  tr:last-child td{{border-bottom:none}}
  .b{{display:inline-block;padding:2px 9px;border-radius:6px;font-size:12px;font-weight:700;}}
  .b-ok{{background:#e7f8f0;color:#0a8f5c}} .b-bad{{background:#fdecec;color:#e5484d}} .b-skip{{background:#fbf3e0;color:#b7791f}}
  .c-ok{{color:var(--ok);font-weight:700}} .c-bad{{color:var(--bad);font-weight:700}} .c-skip{{color:var(--skip);font-weight:700}}
  .remark{{color:#3a4252;font-size:13px;}}
  .shot img{{width:150px;border:1px solid var(--bd);border-radius:6px;display:block;}}
  .shot .noimg{{color:#c2c7cf}}
  .meta{{display:flex;gap:26px;flex-wrap:wrap;color:var(--mut);font-size:13px;margin-bottom:8px;}}
  .meta b{{color:var(--ink)}}
  .note{{background:var(--card);border:1px solid var(--bd);border-radius:10px;padding:14px 16px;color:#3a4252;font-size:13px;}}
</style></head>
<body><div class="wrap">
<h1>DAQ-P-1604 压力采集 · E2E 测试报告</h1>
<p class="sub">基于 test-cases.html 用例集 · Wails v3 WebView2 + Playwright CDP 真实设备驱动</p>

<div class="meta">
  <div>被测设备：<b>{DEVICE_NAME}</b> @ <b>{DEVICE_IP}:{DEVICE_PORT}</b></div>
  <div>生成时间：<b>{now}</b></div>
  <div>用例覆盖：<b>{cov}</b> 自动化 / 文档总数</div>
</div>

<div class="cards">
  <div class="card"><div class="n">{total}</div><div class="l">自动化用例</div></div>
  <div class="card ok"><div class="n">{npass}</div><div class="l">通过 PASS</div></div>
  <div class="card bad"><div class="n">{nfail}</div><div class="l">失败 FAIL</div></div>
  <div class="card skip"><div class="n">{nskip}</div><div class="l">跳过 SKIP</div></div>
  <div class="card"><div class="n">{nblock}</div><div class="l">阻塞 BLOCK</div></div>
</div>

<div class="verdict {overall_cls}">总体结论：{overall_label}（自动化 {total} 例，通过 {npass} / 失败 {nfail} / 跳过 {nskip}）</div>

<h2>分模块汇总</h2>
<table><thead><tr><th>模块</th><th>用例数</th><th>通过</th><th>失败</th><th>跳过</th><th>状态</th></tr></thead>
<tbody>{mod_rows}</tbody></table>

<h2>逐用例明细（含截图证据）</h2>
<table><thead><tr><th>用例</th><th>名称</th><th>结果</th><th>说明 / 断言</th><th>截图</th></tr></thead>
<tbody>{case_rows}</tbody></table>

<h2>说明</h2>
<div class="note">
• 本次 E2E 自动化 41 例，覆盖 CONN / ACQ / CFG / MON / REC / LOG / UI / SCAN / STAB 各模块核心链路。<br>
• <b>跳过（SKIP）</b>用例为设计内行为：物理操作类（拔网线、满磁盘、UDP 广播失败等）与多设备类用例在单台真实设备环境下无法自动化，需独立测试环境或人工执行。<br>
• 失败（FAIL）用例已附截图证据，定位方法见正文结论与 e2e_run.log。<br>
• 测试驱动真实硬件链路（{DEVICE_IP}:{DEVICE_PORT}），非模拟数据。
</div>
</div></body></html>"""

    HTML_OUT.write_text(html, encoding="utf-8")

    # ---- markdown ----
    md = f"""# DAQ-P-1604 压力采集 · E2E 测试报告

- 被测设备：**{DEVICE_NAME}** @ `{DEVICE_IP}:{DEVICE_PORT}`
- 生成时间：{now}
- 用例覆盖：**{total}/{TOTAL_CASES_IN_DOC}** 自动化 / 文档总数
- 结果：**通过 {npass} / 失败 {nfail} / 跳过 {nskip} / 阻塞 {nblock}**
- 驱动方式：Wails v3 WebView2 + Playwright `connect_over_cdp`（真实设备链路，非模拟）

## 分模块汇总

| 模块 | 用例数 | 通过 | 失败 | 跳过 | 状态 |
|---|---|---|---|---|---|
"""
    for mod in sorted(by_mod.keys()):
        lst = by_mod[mod]
        mp = sum(1 for c in lst if c["result"] == "pass")
        mf = sum(1 for c in lst if c["result"] == "fail")
        ms = sum(1 for c in lst if c["result"] == "skip")
        name = MODULE_NAMES.get(mod, mod)
        md += f"| {mod} {name} | {len(lst)} | {mp} | {mf} | {ms} | {'通过' if mf==0 else '有问题'} |\n"

    md += "\n## 逐用例明细\n\n| 用例 | 名称 | 结果 | 说明 |\n|---|---|---|---|\n"
    for c in cases:
        md += f"| {c['tc_id']} | {c['name']} | {c['result'].upper()} | {c.get('remark') or ''} |\n"

    md += f"\n## 结论\n\n总体：{'全部通过 ✓' if nfail==0 else f'{nfail} 个用例失败，见上表'}。\n"
    MD_OUT.write_text(md, encoding="utf-8")
    print(f"报告已生成: {HTML_OUT.name} / {MD_OUT.name}")
    print(f"总计={total} 通过={npass} 失败={nfail} 跳过={nskip} 阻塞={nblock}")


if __name__ == "__main__":
    main()
