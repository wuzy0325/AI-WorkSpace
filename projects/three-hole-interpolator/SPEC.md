# Spec: Three-Hole Probe Interpolation Desktop Application

## Objective

Build a standalone Wails desktop application that reads three-hole probe measurement data, loads calibration (.prb) files, and performs iterative 2D interpolation to compute total pressure, static pressure, Mach number, and angle of attack.

**User**: Wind tunnel test engineers using three-hole pressure probes.
**Success**: The app loads .prb calibration files, processes .dat measurement data (or manual input), displays results in a table, and exports to CSV — with the same look and feel as the existing five-hole interpolator.

**ASSUMPTIONS I'M MAKING:**
1. Calibration file format (.prb) matches the existing C# ThreeHoleProbeApp: `CMa` on line 1, `Nalpha` on line 2, then `Kb Kt Sb Alpha` on each data line.
2. Measurement data has 3 probe holes (P1, P2, P3) plus atmospheric pressure (Patm) and temperature (Tatm).
3. P2 is the center hole; P1 and P3 are side holes.
4. Output fields: PtProbe, PsProbe, MachProbe, AlphaProbe, IterationCount.
5. The frontend should closely match the five-hole interpolator's Vue 3 UI (CSS variables, transitions, layout).
→ Correct me now or I'll proceed with these.

## Tech Stack

| Component | Technology | Version |
|-----------|-----------|---------|
| Backend | Go | 1.25 |
| Frontend | Vue 3 + TypeScript + Vite | ^3.5.14 / ^5.4.20 |
| Desktop Shell | Wails v3 | latest |
| Algorithm | Pure Go (shared package) | — |
| Test Framework | Go testing + testify | — |

## Commands

```powershell
# Dev mode
cd projects\three-hole-interpolator\apps\desktop-wails
go run github.com/wailsapp/wails/v3/cmd/wails3 dev

# Backend tests
cd projects\three-hole-interpolator\apps\desktop-wails
go test ./...

# Algorithm tests
cd shared\algorithms\go\threehole
go test ./...

# Frontend build
cd projects\three-hole-interpolator\apps\desktop-wails\frontend
npm run build
```

## Project Structure

```
shared/algorithms/go/threehole/
├── go.mod
└── interpolation/
    ├── types.go            → Data structures (InterpolationInput, InterpolationResult, etc.)
    ├── three_hole.go       → Core interpolation algorithm
    └── three_hole_test.go  → Algorithm unit tests

projects/three-hole-interpolator/
├── README.md
├── SPEC.md
└── apps/desktop-wails/
    ├── main.go             → Wails entry point
    ├── go.mod              → Module that replaces shared algorithm
    ├── wails.json          → Wails config
    ├── backend/
    │   ├── app.go          → Wails bindings (LoadPrb, Calculate, BatchCalculate, ImportCsv, Export)
    │   └── app_test.go     → Backend tests
    ├── frontend/
    │   ├── index.html
    │   ├── package.json
    │   ├── vite.config.ts
    │   ├── tsconfig.json
    │   └── src/
    │       ├── main.ts
    │       ├── env.d.ts
    │       ├── App.vue      → Single-file component (modeled after five-hole)
    │       └── wails-adapter.ts → Wails API wrapper
    └── docs/
        └── 用户说明书.html   → Help documentation
```

## Code Style

Follow the five-hole interpolator style:

**Go** — `shared/algorithms/go/fivehole/interpolation/`:
```go
package interpolation

type InterpolationInput struct {
    P1   float64 `json:"P1"`
    P2   float64 `json:"P2"`
    P3   float64 `json:"P3"`
    PAtm float64 `json:"Patm"`
    TAtm float64 `json:"Tatm"`
}

type InterpolationResult struct {
    Alpha      float64 `json:"alpha"`
    MachNumber float64 `json:"machNumber"`
    // ...
    IsValid    bool    `json:"isValid"`
    Warning    string  `json:"warning,omitempty"`
}
```

**Vue 3** — Single-file component with `<script setup lang="ts">`, CSS custom properties, `<Transition>` animations, all CSS in `<style>` block (no separate CSS files).

## Testing Strategy

| Level | Location | Focus |
|-------|----------|-------|
| Unit (algorithm) | `shared/algorithms/go/threehole/interpolation/*_test.go` | Interpolation correctness, edge cases, convergence |
| Unit (backend) | `projects/.../apps/desktop-wails/backend/app_test.go` | Input conversion, validation, error handling |

Coverage target: >80% for algorithm package, >60% for backend bindings.

## Algorithm Summary (extracted from C# ThreeHoleProbeApp)

1. **Kb calculation**: ΔP = 2·P2 − P1 − P3; Kb = (P3 − P1) / ΔP
2. **Iterative solve** (max 20 iterations, tolerance 1e-4):
   a. Find 2 nearest calibration Mach numbers
   b. Linear interpolate Kb/Kt/Sb in Mach direction
   c. Sort by Kb, then linear interpolate to find Alpha/Kt/Sb
   d. Compute Pt = P2 + Kt·ΔP, Ps = Pt − Sb·ΔP
   e. Compute Ma = sqrt(5 · |((Pt+Pa)/(Ps+Pa))^0.2857 − 1|)
   f. Clamp Ma to [minMa, maxMa], check convergence
3. **Final output**: Re-interpolate with converged Ma, compute final values

## Boundaries

- **Always:** Validate inputs before calculation; run tests before committing; use table-driven Go tests; follow five-hole project conventions
- **Ask first:** Changing the calibration file format; adding new output fields; modifying convergence logic; adding external dependencies
- **Never:** Import from `projects/wind-daq` internal packages; hard-code calibration paths; use C# code directly in Go (translate algorithm only)

## Success Criteria

- [x] `shared/algorithms/go/threehole/interpolation/` passes `go test ./...`
- [x] Wails app loads .prb files in the three-hole format
- [x] Manual single-point calculation returns correct Pt/Ps/Ma/Alpha
- [x] Batch CSV import & calculation works end-to-end
- [x] Results table displays all computed fields
- [x] CSV export produces correct output
- [x] Frontend matches five-hole interpolator visual style

## Open Questions

1. Should the .dat file format (7-column from C#) be supported alongside manual input?
2. Should the app support the same three-hole PRB format, or a different extension/format?
3. Any changes needed to the convergence algorithm (maxIterations, tolerance)?
