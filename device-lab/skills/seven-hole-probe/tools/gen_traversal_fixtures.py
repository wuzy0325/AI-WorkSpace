#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Generate Go<->Python cross-check fixtures for the seven-hole traversal
interpolator (tasks-seven-hole-traversal.md Task 6).

MUST be re-run (and its output diff reviewed) whenever any of these change:
  - the .prb file format or row/column conventions,
  - the seven-hole algorithm itself,
  - the seven_hole.py API (cal_ab signature, helper names),
  - the source dataset CSVs.

Inputs (read-only):
  - Python authority: device-lab/skills/seven-hole-probe/seven_hole.py
    (cal_ab + helper functions; imported, never modified; sympy required).
  - Dataset (GBK): projects/wind-daq/docs/W532.202608.P.7H.1-01/
    1 small-angle CSV (169 rows) + 6 large-angle CSVs (52 rows each).

Degenerate-edge dither: the report CSV rounds coefficients to 3 decimals,
which produces exact ka/kb ties between adjacent grid points. The Python
reference crashes (ZeroDivisionError) when it scans any quadrilateral whose
edge has dka == 0 (or dkb == 0 on edges 2/4) -- ~40% of the 481 dataset
points are unreachable that way. The unrounded physical coefficients were
certainly distinct, so write_prb applies a deterministic <=1e-7 dither that
only fires on exact ties (see nudge_degenerate). Both sides of the
cross-check read the same .prb files, so equivalence is unaffected; the
angle shift is orders of magnitude below the 1e-4 deg tolerance.

Column mapping is by POSITION (the headers carry historical naming errors,
see spec-seven-hole-calibration.md section 12.1). Verified numerically
against the Python formulas:
  col0/col1  : inner = (a,b); outer = (phi,theta)  -> .prb a,b
               (inner: a=col0,b=col1; outer: a=theta=col1,b=phi=col0)
  col5..col11: P1..P7 (gauge Pa)
  col12/13   : ka,kb  (inner Kalpha/Kbeta; outer Ktheta[n]/Kphi[n])
  col14/15   : cpt,cps (K0[n], Ks[n])
  col16/17   : pa (abs Pa), t (degC)

Outputs (committed):
  shared/algorithms/go/sevenhole/interpolation/testdata/prb/{7,1..6}.prb
  shared/algorithms/go/sevenhole/interpolation/testdata/golden/golden.json
  shared/algorithms/go/sevenhole/interpolation/testdata/golden/boundary.json
"""
import io
import json
import math
import os
import sys
import tempfile
from pathlib import Path

SCRIPT_DIR = Path(__file__).resolve().parent
SKILL_DIR = SCRIPT_DIR.parent                      # seven-hole-probe/
REPO_ROOT = SCRIPT_DIR.parents[3]                  # device-lab/..
DATASET_DIR = REPO_ROOT / "projects" / "wind-daq" / "docs" / "W532.202608.P.7H.1-01"
TESTDATA_PRB = REPO_ROOT / "shared" / "algorithms" / "go" / "sevenhole" / "interpolation" / "testdata" / "prb"
TESTDATA_GOLDEN = REPO_ROOT / "shared" / "algorithms" / "go" / "sevenhole" / "interpolation" / "testdata" / "golden"
TRANSCODE_DIR = SCRIPT_DIR / "_transcoded"         # temporary UTF-8 copies

sys.path.insert(0, str(SKILL_DIR))
import seven_hole  # noqa: E402

DATASET_STEM = "W532.202608.P.7H.1-01-85米每秒（0.242Ma）"
CSV_FILES = {
    "inner": DATASET_STEM + "(小角度区).csv",
    1: DATASET_STEM + "(大角度1区).csv",
    2: DATASET_STEM + "(大角度2区).csv",
    3: DATASET_STEM + "(大角度3区).csv",
    4: DATASET_STEM + "(大角度4区).csv",
    5: DATASET_STEM + "(大角度5区).csv",
    6: DATASET_STEM + "(大角度6区).csv",
}

EXPECTED_ROWS = {"inner": 169, "outer": 52}
SECTOR_PHI_LINES = {  # spec-seven-hole-traversal section 2.1
    1: [30, 25, 20, 15, 10, 5, 0, 355, 350, 345, 340, 335, 330],
    2: [90, 85, 80, 75, 70, 65, 60, 55, 50, 45, 40, 35, 30],
    3: [150, 145, 140, 135, 130, 125, 120, 115, 110, 105, 100, 95, 90],
    4: [210, 205, 200, 195, 190, 185, 180, 175, 170, 165, 160, 155, 150],
    5: [270, 265, 260, 255, 250, 245, 240, 235, 230, 225, 220, 215, 210],
    6: [330, 325, 320, 315, 310, 305, 300, 295, 290, 285, 280, 275, 270],
}


def transcode_and_parse(name):
    """GBK -> UTF-8 copy under tools/_transcoded/, then parse rows by column
    position. Returns a list of 18-float rows (header excluded)."""
    src = DATASET_DIR / name
    TRANSCODE_DIR.mkdir(exist_ok=True)
    dst = TRANSCODE_DIR / (src.stem + ".utf8.csv")
    with io.open(src, encoding="gbk") as f:
        text = f.read()
    with io.open(dst, "w", encoding="utf-8") as f:
        f.write(text)
    rows = []
    for lineno, line in enumerate(text.splitlines()):
        if lineno == 0:
            continue  # header (names are historically wrong; positions rule)
        parts = line.split(",")
        if len(parts) != 18:
            raise AssertionError(f"{name} line {lineno + 1}: expect 18 columns, got {len(parts)}")
        rows.append([float(x) for x in parts])
    return rows


def fmt(x):
    """Shortest round-trip float formatting for .prb rows."""
    return repr(float(x))


def nudge_degenerate(points, a_vals, b_vals):
    """Break exact ka/kb ties that form degenerate quadrilateral edges.

    points: dict (a,b) -> [ka,kb,cpt,cps], mutated in place.
    a_vals/b_vals: grid-line coordinates in index order (adjacent indices are
    grid neighbors, including across the 0/360 wrap order of sector 1).

    A quadrilateral edge crashes the Python reference when:
      - any edge has dka == 0 (vertical edge: k = dkb/0), or
      - a b-direction edge (edges 2/4) has dkb == 0 (x2/x4 divide by zero k).
    The dither (+1e-9, increasing per nudge) fires only on exact ties and
    approximates the precision lost by the report's 3-decimal rounding.
    """
    nudges = 0

    def bad_edges():
        bad = []
        na, nb = len(a_vals), len(b_vals)
        for bi in range(nb):
            for ai in range(na):
                p = points[(a_vals[ai], b_vals[bi])]
                if ai + 1 < na:  # a-direction edge (edges 1/3): dka must be nonzero
                    q = points[(a_vals[ai + 1], b_vals[bi])]
                    if q[0] == p[0]:
                        bad.append((0, (a_vals[ai + 1], b_vals[bi])))
                if bi + 1 < nb:  # b-direction edge (edges 2/4): dka and dkb must be nonzero
                    q = points[(a_vals[ai], b_vals[bi + 1])]
                    if q[0] == p[0]:
                        bad.append((0, (a_vals[ai], b_vals[bi + 1])))
                    if q[1] == p[1]:
                        bad.append((1, (a_vals[ai], b_vals[bi + 1])))
        return bad

    for _ in range(100):
        bad = bad_edges()
        if not bad:
            return nudges
        for field, key in bad:
            nudges += 1
            points[key][field] += 1e-9 * nudges
    raise AssertionError("degenerate-edge nudging did not converge")


def write_prb(path, rows, a_vals, b_vals):
    """rows: iterable of (ka,kb,cpt,cps,a,b) in stable output order;
    a_vals/b_vals define the grid-line index order for tie detection.

    a/b MUST be integer-formatted: the Python reference matches grid points
    via int(calibration['a']) / int(calibration['b']) on the raw strings
    (little/big_create_line, little/big_create_square), so "30.0" would raise
    ValueError where "30" works. All grid coordinates are multiples of 5.
    """
    rows = list(rows)
    points = {(a, b): [ka, kb, cpt, cps] for ka, kb, cpt, cps, a, b in rows}
    assert len(points) == len(rows), f"duplicate grid coordinates in {path.name}"
    nudges = nudge_degenerate(points, a_vals, b_vals)
    lines = ["ka kb cpt cps a b"]
    for ka, kb, cpt, cps, a, b in rows:
        assert float(a).is_integer() and float(b).is_integer(), f"non-integer grid coord ({a},{b})"
        nka, nkb, ncpt, ncps = points[(a, b)]
        lines.append(" ".join([fmt(nka), fmt(nkb), fmt(ncpt), fmt(ncps), str(int(a)), str(int(b))]))
    path.parent.mkdir(parents=True, exist_ok=True)
    with io.open(path, "w", encoding="utf-8", newline="\n") as f:
        f.write("\n".join(lines) + "\n")
    if nudges:
        print(f"{path.name}: {nudges} degenerate-edge dither(s) applied")
    return len(rows)


def hole_dict(p):
    return {"p1": p[0], "p2": p[1], "p3": p[2], "p4": p[3], "p5": p[4],
            "p6": p[5], "p7": p[6], "t": p[7], "pa": p[8]}


def run_cal_ab(hd):
    """Run the authoritative cal_ab on one input. Returns the parsed output
    dict, or None when cal_ab fails ('no-return')."""
    with tempfile.NamedTemporaryFile("w", suffix=".json", delete=False, encoding="utf-8") as f:
        json.dump(hd, f)
        tmp = f.name
    try:
        out = seven_hole.cal_ab(tmp, str(TESTDATA_PRB))
    finally:
        os.unlink(tmp)
    if not isinstance(out, str) or out == "no-return":
        return None
    return json.loads(out)


def classify(hd, big_cal):
    """Return (mode, sector, fallback) using seven_hole's own helpers:
    'little' when the inner polygon accepts the point; else 'big' with the
    sector that accepts (first or second candidate)."""
    little_pt = seven_hole.little_cal_kakb(hd)
    little_lines = seven_hole.little_create_line(seven_hole.little_read_file(str(TESTDATA_PRB)))
    if seven_hole.point_in_polygon(little_pt, little_lines) in (0, 1):
        return "little", 0, False
    max_keys = seven_hole.big_max_pressure(hd)
    first, second = max_keys["first"], max_keys["second"]
    d1 = seven_hole.big_cal_kakb(hd, first)
    if seven_hole.point_in_polygon(d1, seven_hole.big_create_line(big_cal, first)) in (0, 1):
        return "big", int(first), False
    d2 = seven_hole.big_cal_kakb(hd, second)
    if seven_hole.point_in_polygon(d2, seven_hole.big_create_line(big_cal, second)) in (0, 1):
        return "big", int(second), True
    return "out", 0, False


def entry(index, hd, out, mode, sector, fallback):
    return {
        "index": index,
        "mode": mode,
        "sector": sector,
        "fallback": fallback,
        "input": {"p1": hd["p1"], "p2": hd["p2"], "p3": hd["p3"], "p4": hd["p4"],
                  "p5": hd["p5"], "p6": hd["p6"], "p7": hd["p7"], "pa": hd["pa"], "t": hd["t"]},
        "output": {"alpha": out["a"], "beta": out["b"], "pt": out["pt"],
                   "ps": out["ps"], "ma": out["ma"], "v": out["v"]},
    }


def assert_finite(e):
    for k, v in e["output"].items():
        if not isinstance(v, (int, float)) or not math.isfinite(v):
            raise AssertionError(f"point {e['index']}: output {k} not finite: {v!r}")


def main():
    # 1. Parse dataset (GBK -> UTF-8 copies, then by column position).
    inner_rows = transcode_and_parse(CSV_FILES["inner"])
    outer_rows = {n: transcode_and_parse(CSV_FILES[n]) for n in range(1, 7)}
    assert len(inner_rows) == EXPECTED_ROWS["inner"], f"inner rows {len(inner_rows)} != 169"
    for n in range(1, 7):
        assert len(outer_rows[n]) == EXPECTED_ROWS["outer"], f"sector {n} rows {len(outer_rows[n])} != 52"

    # Grid coverage assertions (angles must match the spec grid exactly).
    inner_ab = {(r[0], r[1]) for r in inner_rows}
    grid = [-30 + 5 * i for i in range(13)]
    assert inner_ab == {(a, b) for b in grid for a in grid}, "inner (a,b) grid coverage mismatch"
    for n in range(1, 7):
        got = {(r[1], r[0]) for r in outer_rows[n]}  # (theta, phi)
        want = {(t, p) for p in SECTOR_PHI_LINES[n] for t in (30, 35, 40, 45)}
        assert got == want, f"sector {n} (theta,phi) grid coverage mismatch"

    # 2. Emit the 7 .prb files (coefficients by column position; exact ties
    # that would crash the Python reference are dithered, see write_prb).
    grid_lines = [-30 + 5 * i for i in range(13)]
    n_inner = write_prb(TESTDATA_PRB / "7.prb",
                        [(r[12], r[13], r[14], r[15], r[0], r[1]) for r in inner_rows],
                        grid_lines, grid_lines)
    assert n_inner == 169
    for n in range(1, 7):
        n_outer = write_prb(TESTDATA_PRB / f"{n}.prb",
                            [(r[12], r[13], r[14], r[15], r[1], r[0]) for r in outer_rows[n]],
                            [30, 35, 40, 45], SECTOR_PHI_LINES[n])
        assert n_outer == 52, f"sector {n} .prb rows {n_outer} != 52"

    # 3. Golden: run cal_ab on every dataset point.
    big_cal = seven_hole.big_read_file(str(TESTDATA_PRB))
    golden = []
    index = 0
    for rows, tag in [(inner_rows, "inner")] + [(outer_rows[n], n) for n in range(1, 7)]:
        for r in rows:
            hd = hole_dict((r[5], r[6], r[7], r[8], r[9], r[10], r[11], r[17], r[16]))
            out = run_cal_ab(hd)
            assert out is not None, f"cal_ab failed on dataset point {index} (region {tag})"
            mode, sector, fallback = classify(hd, big_cal)
            e = entry(index, hd, out, mode, sector, fallback)
            assert_finite(e)
            golden.append(e)
            index += 1
    assert len(golden) == 481, f"golden count {len(golden)} != 481"

    # 4. Boundary cases (8 canonical + constructed guards). Dataset row
    # order: inner indices 0..168 (b outer, a inner); outer sector n at
    # 169+(n-1)*52 (phi outer in SECTOR_PHI_LINES order, theta inner).
    boundary = []

    def add_dataset_case(idx, why):
        e = dict(golden[idx])
        e["why"] = why
        boundary.append(e)

    add_dataset_case(0, "inner grid corner a=-30,b=-30 (boundary grid line)")
    add_dataset_case(87, "pure sideslip beta=0 (a=15,b=0)")
    add_dataset_case(123, "pure attack alpha=0 (a=0,b=15)")
    add_dataset_case(169, "theta=30 inner/outer junction (sector 1, phi=330)")
    add_dataset_case(172, "theta=45 outer boundary (sector 1, phi=330)")
    fallbacks = [e for e in golden if e["fallback"]]
    assert fallbacks, "no second-candidate fallback point found in dataset"
    add_dataset_case(fallbacks[0]["index"], "second-candidate sector fallback")
    add_dataset_case(290, "general in-sector point (sector 3, theta=35, phi=130)")

    # Constructed cases (still inside the legal grid; golden via cal_ab).
    constructed = [
        ({"p1": 1000.0, "p2": 1000.0, "p3": 1000.0, "p4": 1000.0,
          "p5": 1000.0, "p6": 1000.0, "p7": 1500.0, "t": 28.0, "pa": 98869.0},
         "constructed origin ka=kb=0"),
        # p1..p6=-1000 keeps pt>ps when p7=0 (pt-ps = 1000/D, D>0), so the
        # case exercises P7=0 as a legal input, not the pt<ps guard.
        ({"p1": -1000.0, "p2": -1000.0, "p3": -1000.0, "p4": -1000.0,
          "p5": -1000.0, "p6": -1000.0, "p7": 0.0, "t": 28.0, "pa": 98869.0},
         "constructed P7=0 (must not be rejected)"),
    ]
    for hd, why in constructed:
        out = run_cal_ab(hd)
        assert out is not None, f"cal_ab failed on constructed case: {why}"
        mode, sector, fallback = classify(hd, big_cal)
        e = entry(len(golden) + len(boundary), hd, out, mode, sector, fallback)
        e["why"] = why
        assert_finite(e)
        boundary.append(e)

    # 5. Write golden JSON.
    TESTDATA_GOLDEN.mkdir(parents=True, exist_ok=True)
    with io.open(TESTDATA_GOLDEN / "golden.json", "w", encoding="utf-8", newline="\n") as f:
        json.dump(golden, f, ensure_ascii=False, indent=1)
    with io.open(TESTDATA_GOLDEN / "boundary.json", "w", encoding="utf-8", newline="\n") as f:
        json.dump(boundary, f, ensure_ascii=False, indent=1)

    modes = {}
    for e in golden:
        modes[e["mode"]] = modes.get(e["mode"], 0) + 1
    print(f"golden: {len(golden)}/481 points ({modes}), boundary: {len(boundary)} cases")
    print(f"fallback points in dataset: {len(fallbacks)}")
    print("OK")


if __name__ == "__main__":
    main()
