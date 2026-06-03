---
target: wind-daq dashboard
total_score: 17
p0_count: 2
p1_count: 2
timestamp: 2026-06-03T06-18-43Z
slug: projects-wind-daq-apps-desktop-wails-frontend-src
---
#### Design Health Score

| # | Heuristic | Score | Key Issue |
|---|-----------|-------|-----------|
| 1 | Visibility of System Status | 3/4 | Good pulse animations, no stale-data indication |
| 2 | Match System / Real World | 2/4 | Channel mapping correct but raw model numbers exposed |
| 3 | User Control and Freedom | 2/4 | View modes available, no confirmation before destructive ops |
| 4 | Consistency and Standards | 1/4 | Accent color mismatch (#10b981 vs DESIGN.md #38bdf8) |
| 5 | Error Prevention | 1/4 | No confirmation on tare-all or disconnect-during-acquisition |
| 6 | Recognition Rather Than Recall | 2/4 | Status dots intuitive, action icons require memorization |
| 7 | Flexibility and Efficiency | 2/4 | Multi-view modes, no keyboard shortcuts or channel search |
| 8 | Aesthetic and Minimalist Design | 2/4 | Dark base correct, 5+ violations adding visual noise |
| 9 | Error Recovery | 1/4 | Error state shown but zero diagnostic detail |
| 10 | Help and Documentation | 1/4 | No help access, title-attribute tooltips only |
| Total | | 17/40 | Poor |

#### Anti-Patterns Verdict

AI Slop: YES. 3 markers hit: colored left border stripes (DeviceOverviewPanel.vue:136), glassmorphism on content panels (DeviceDetailPanel.vue:550,682), tiny uppercase tracked eyebrows (3 locations). Second-order reflex: dark glassmorphic SaaS with emerald glow - the exact aesthetic DESIGN.md rejects.

#### Cognitive Load

3/8 pass. Failures: single focus (6+ focal points), chunking (16+ channels at once), one-thing-at-a-time (multiple simultaneous streams), working memory (sidebar-context-to-detail-panel bridging). Partial: progressive disclosure (chart modal good, overview dump all bad).

#### Priority Issues

* P0 - Glassmorphism on content (DeviceDetailPanel backdrop-filter blur)
* P0 - Colored 4px left-border accent stripes
* P1 - Accent color mismatch (#10b981 vs DESIGN.md #38bdf8)
* P1 - Tiny uppercase tracked eyebrows proliferating
* P2 - No confirmation before destructive ops (tare-all, disconnect)

#### Persona Red Flags

Alex (Power User): no keyboard shortcuts, grid not customizable, gradient accents become noise
Jordan (First-Timer): empty state is dead end, icon-only actions opaque, raw model numbers as labels

#### Minor Observations

Stray ceramic-shell theme class, competing pulse animations in bottom bar, hidden scrollbar on sidebar, min font-size too small for CJK, arbitrary z-index values, abrupt view mode transitions.
