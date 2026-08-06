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

Coefficient precision: .prb coefficients are recomputed from the pressure
columns at full float64 precision instead of copying the report's rounded
coefficient columns. write_prb retains a deterministic exact-tie dither as a
defensive guard for future datasets, then applies sub-1e-9 node jitter so the
float-fragile Python reference can still read the generated fixtures. Existing
3-decimal PRB exports must be regenerated from their source pressure CSVs;
rounded PRB coefficients cannot be restored without those pressures.

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
STRICT_NODE_EPS = 1e-9
FALLBACK_NODE_EPS = 1e-6
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


def recompute_inner_coeffs(r):
    """Recompute inner-zone ka/kb/cpt/cps at full float64 precision from the
    raw gauge pressures, instead of using the report's 3-decimal rounded
    coefficient columns (col12..15).

    The report CSV rounds Kalpha/Kbeta/K0/Ks to 3 decimals, which is far above
    the 1e-6 grid-point recovery tolerance used by the Go inner-zone fallback
    (inner_zone.go innerFindGridPointByKaKb). Self-extracted reverse inference
    then fails on grid-boundary nodes: the pressure-recomputed (ka,kb) lands
    ~5e-4 away from the stored node and can be classified OUTSIDE the boundary
    polygon. Recomputing from the pressures keeps the fixture PRB consistent
    with any later same-source reverse inference (spec section 2.1B: ka/kb and
    cpt/cps are pressure-difference ratios, so gauge/absolute cancels).

    Formulas (spec section 4.1 / 4.2): inner zone centered on P7.
    """
    p1, p2, p3, p4, p5, p6, p7 = (r[i] for i in range(5, 12))
    pt, ps = r[3], r[4]  # 来流总压 / 来流静压 (gauge Pa)
    p_avg = (p1 + p2 + p3 + p4 + p5 + p6) / 6.0
    denom = p7 - p_avg
    cpa = (p4 - p1) / denom
    cpb = (p5 - p2) / denom
    cpc = (p6 - p3) / denom
    ka = (cpb + cpc) / math.sqrt(3.0)
    kb = -(2.0*cpa + cpb - cpc) / 3.0
    denom_c = pt - ps
    cpt = (p7 - pt) / denom_c
    cps = (ps - p_avg) / denom_c
    return ka, kb, cpt, cps


def recompute_outer_coeffs(r, n):
    """Recompute large-angle sector n ka/kb/cpt/cps at full float64 precision
    (spec section 4.2): centered on the max-pressure hole Pn, ring neighbors
    wrapped mod 6.
    """
    p = [r[i] for i in range(5, 11)]  # P1..P6
    p7 = r[11]
    pt, ps = r[3], r[4]
    pc = p[n - 1]
    pl = p[(n - 2) % 6]
    pr = p[n % 6]
    denom = pc - (pl + pr) / 2.0
    ka = (pc - p7) / denom
    kb = (pl - pr) / denom
    denom_c = pt - ps
    cpt = (pc - pt) / denom_c
    cps = (ps - (pl + pr) / 2.0) / denom_c
    return ka, kb, cpt, cps


def nudge_degenerate(points, a_vals, b_vals):
    """Break exact ka/kb ties that form degenerate quadrilateral edges.

    points: dict (a,b) -> [ka,kb,cpt,cps], mutated in place.
    a_vals/b_vals: grid-line coordinates in index order (adjacent indices are
    grid neighbors, including across the 0/360 wrap order of sector 1).

    A quadrilateral edge crashes the Python reference when:
      - any edge has dka == 0 (vertical edge: k = dkb/0), or
      - a b-direction edge (edges 2/4) has dkb == 0 (x2/x4 divide by zero k).
    The dither (+1e-9, increasing per nudge) fires only on exact ties.
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


def jitter_grid_points(points, a_vals, b_vals):
    """Apply a deterministic ~1e-12 dither to every grid point's ka/kb.

    The reference implementation recomputes the .prb coefficients at full
    float64 precision from the raw gauge pressures (recompute_inner/outer_coeffs),
    so a reverse-inferred input (ka,kb) computed from the SAME pressures is
    bit-for-bit equal to its grid node. The Python reference has no grid-point
    fallback and uses exact closed-interval edge tests, so a point lying
    exactly on a cell corner (grid boundary node) fails to locate a quad and
    cal_ab returns 'no-return' (~17 of 481 dataset points).

    The dither shifts each node's ka/kb by an index-dependent amount of
    1e-12..~5e-11:
      - small enough to stay inside the Go boundary-polygon vertex tolerance
        (pointInPolygon/grid-node routing = STRICT_NODE_EPS) and the wider
        recovery fallback (FALLBACK_NODE_EPS), so reverse-inferred nodes
        still classify as inner and round-trip to their exact angles
        (measured max angle shift ~2e-9 deg, far below golden 1e-4);
      - large enough to break the exact-corner coincidence that trips the
        Python reference.
    It is deterministic (index-ordered) so the fixtures are reproducible.
    """
    n = 0
    na, nb = len(a_vals), len(b_vals)
    for bi in range(nb):
        for ai in range(na):
            key = (a_vals[ai], b_vals[bi])
            n += 1
            points[key][0] += 1e-12 * n
            points[key][1] += 1e-12 * n
    return n


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
    # Deterministic sub-STRICT_NODE_EPS jitter on every node: keeps exact-node
    # routing active; FALLBACK_NODE_EPS remains only a recovery path.
    njit = jitter_grid_points(points, a_vals, b_vals)
    lines = ["ka kb cpt cps a b"]
    for ka, kb, cpt, cps, a, b in rows:
        assert float(a).is_integer() and float(b).is_integer(), f"non-integer grid coord ({a},{b})"
        nka, nkb, ncpt, ncps = points[(a, b)]
        lines.append(" ".join([fmt(nka), fmt(nkb), fmt(ncpt), fmt(ncps), str(int(a)), str(int(b))]))
    path.parent.mkdir(parents=True, exist_ok=True)
    with io.open(path, "w", encoding="utf-8", newline="\n") as f:
        f.write("\n".join(lines) + "\n")
    if nudges or njit:
        print(f"{path.name}: {nudges} degenerate-edge dither(s), {njit} grid-point jitter(s) applied")
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
    assert max_keys is not None, "big_max_pressure returned no candidates"
    first, second = max_keys["first"], max_keys["second"]
    d1 = seven_hole.big_cal_kakb(hd, first)
    if seven_hole.point_in_polygon(d1, seven_hole.big_create_line(big_cal, first)) in (0, 1):
        return "big", int(first), False
    d2 = seven_hole.big_cal_kakb(hd, second)
    if seven_hole.point_in_polygon(d2, seven_hole.big_create_line(big_cal, second)) in (0, 1):
        return "big", int(second), True
    return "out", 0, False


def inner_grid_node_hit(hd, inner_cal):
    """Return (cpt,cps) of the inner-grid node whose (ka,kb) equals the
    pressure-recomputed point within Go's strict node-routing tolerance, or
    None.

    The Python reference's little branch uses exact edge-equality tests, so a
    calibration point that lies EXACTLY on an inner-grid node (all 169 inner
    calibration rows ARE grid nodes) is classified OUTSIDE the inner polygon
    (point_in_polygon ray cast with no vertex tolerance) and routed to the
    large-angle branch with a slightly-off extrapolated angle. The Go
    implementation instead carries STRICT_NODE_EPS vertex and node matching,
    plus a wider FALLBACK_NODE_EPS recovery path, so it correctly
    resolves such points in the inner zone, reproducing the exact grid node.
    Golden must match Go's (fixed) behaviour, so these points are emitted as
    inner-zone direct hits instead of Python's big-branch output.
    """
    lp = seven_hole.little_cal_kakb(hd)
    ka, kb = lp["ka"], lp["kb"]
    for c in inner_cal:
        nka, nkb = float(c["ka"]), float(c["kb"])
        if abs(nka - ka) < STRICT_NODE_EPS and abs(nkb - kb) < STRICT_NODE_EPS:
            return float(c["cpt"]), float(c["cps"])
    return None


def outer_grid_node_hit(hd, sector, big_cal):
    """Return (cpt,cps,a,b) of the large-angle grid node of `sector` whose
    (ka,kb) equals the pressure-recomputed point within STRICT_NODE_EPS, or None.

    Same rationale as inner_grid_node_hit: the Python reference's big branch
    uses exact edge tests and, for a point exactly on a sector-grid node
    (e.g. the theta=45 outermost line or a sector boundary line), fails to
    locate a quad and falls through to beyond_border extrapolation
    (mode='out'). Go's strict grid-point route resolves such points exactly;
    golden must match that, so these points are
    emitted as big-sector direct hits instead of Python's out extrapolation.
    """
    d = seven_hole.big_cal_kakb(hd, str(sector))
    for c in big_cal[str(sector)]:
        nka, nkb = float(c["ka"]), float(c["kb"])
        if abs(nka - d["ka"]) < STRICT_NODE_EPS and abs(nkb - d["kb"]) < STRICT_NODE_EPS:
            return float(c["cpt"]), float(c["cps"]), float(c["a"]), float(c["b"])
    return None


def solve_outer_ptps(hd, sector, cpt, cps):
    """Closed-form solve of the large-angle Pt/Ps system for sector n
    (SKILL.md section 3.6): cpt=(pc-pt)/(pt-ps),
    cps=(ps-(pl+pr)/2)/(pt-ps), ring neighbors wrapped mod 6."""
    p = [hd["p%d" % i] for i in range(1, 7)]
    p7 = hd["p7"]
    pc = p[sector - 1]
    pl = p[(sector - 2) % 6]
    pr = p[sector % 6]
    p_mid = (pl + pr) / 2.0
    d = 1 + cpt + cps
    pt = (pc * (1 + cps) + cpt * p_mid) / d
    ps = (pc * cps + p_mid * (1 + cpt)) / d
    return pt, ps


def solve_inner_ptps(hd, cpt, cps):
    """Closed-form solve of the inner-zone Pt/Ps system (SKILL.md section
    2.5): cpt=(p7-pt)/(pt-ps), cps=(ps-pAvg)/(pt-ps)."""
    p_avg = (hd["p1"] + hd["p2"] + hd["p3"] + hd["p4"] + hd["p5"] + hd["p6"]) / 6.0
    d = 1 + cpt + cps
    pt = (hd["p7"] * (1 + cps) + cpt * p_avg) / d
    ps = (hd["p7"] * cps + p_avg * (1 + cpt)) / d
    return pt, ps


def midpoint_input(first, second):
    """Build a deterministic non-node pressure input between two CSV rows."""
    values = [(first[i] + second[i]) / 2.0 for i in range(len(first))]
    return hole_dict((values[5], values[6], values[7], values[8], values[9],
                      values[10], values[11], values[17], values[16]))


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

    # 2. Emit the 7 .prb files. Coefficients are RECOMPUTED at full float64
    # precision from the raw gauge pressures (recompute_inner/outer_coeffs),
    # NOT copied from the report's 3-decimal rounded K columns — otherwise the
    # stored grid is ~5e-4 away from any same-source reverse-inferred (ka,kb),
    # which breaks strict grid-node routing and boundary-node round-trips.
    # The nudge_degenerate dither below still only fires on exact ties that
    # would crash the Python reference (see write_prb).
    grid_lines = [-30 + 5 * i for i in range(13)]
    n_inner = write_prb(TESTDATA_PRB / "7.prb",
                        [recompute_inner_coeffs(r) + (r[0], r[1]) for r in inner_rows],
                        grid_lines, grid_lines)
    assert n_inner == 169
    for n in range(1, 7):
        n_outer = write_prb(TESTDATA_PRB / f"{n}.prb",
                            [recompute_outer_coeffs(r, n) + (r[1], r[0]) for r in outer_rows[n]],
                            [30, 35, 40, 45], SECTOR_PHI_LINES[n])
        assert n_outer == 52, f"sector {n} .prb rows {n_outer} != 52"

    # 3. Golden: every source row is itself a calibration grid node, so its
    # authoritative round-trip is the corresponding node rather than Python's
    # float-fragile edge search. This covers inner/outer overlap, shared-sector
    # edges, and the sector-1 355/0 wrap uniformly. Non-node interpolation is
    # independently covered below by direct seven_hole.cal_ab outputs.
    big_cal = seven_hole.big_read_file(str(TESTDATA_PRB))
    inner_cal = seven_hole.little_read_file(str(TESTDATA_PRB))
    golden = []
    index = 0
    for rows, tag in [(inner_rows, "inner")] + [(outer_rows[n], n) for n in range(1, 7)]:
        for r in rows:
            hd = hole_dict((r[5], r[6], r[7], r[8], r[9], r[10], r[11], r[17], r[16]))
            if tag == "inner":
                hit = inner_grid_node_hit(hd, inner_cal)
                assert hit is not None, f"dataset point {index} does not match its inner calibration grid"
                cpt, cps = hit
                pt, ps = solve_inner_ptps(hd, cpt, cps)
                out = {"a": r[0], "b": r[1], "pt": pt, "ps": ps}
                mode, sector, fallback = "little", 0, False
            else:
                sector_hit = int(tag)
                hit = outer_grid_node_hit(hd, sector_hit, big_cal)
                assert hit is not None, f"dataset point {index} does not match outer sector {sector_hit}"
                cpt, cps, theta, phi = hit
                pt, ps = solve_outer_ptps(hd, sector_hit, cpt, cps)
                converted = seven_hole.big_ab_convert({"a": theta, "b": phi})
                out = {"a": converted["a"], "b": converted["b"], "pt": pt, "ps": ps}
                mode, sector, fallback = "big", sector_hit, False
            v = math.sqrt(2 * math.fabs(out["pt"] - out["ps"]) * 287.06 * (hd["t"] + 273.15) / hd["pa"])
            ma = math.sqrt(5 * math.fabs(math.pow((out["pt"] + hd["pa"]) / (out["ps"] + hd["pa"]), 0.4 / 1.4) - 1))
            out["v"], out["ma"] = v, ma
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
        (midpoint_input(inner_rows[84], inner_rows[85]),
         "python parity non-node inner"),
        (midpoint_input(outer_rows[3][18], outer_rows[3][19]),
         "python parity non-node outer"),
    ]
    for hd, why in constructed:
        out = run_cal_ab(hd)
        assert out is not None, f"cal_ab failed on constructed case: {why}"
        mode, sector, fallback = classify(hd, big_cal)
        if why.endswith("inner"):
            assert mode == "little" and inner_grid_node_hit(hd, inner_cal) is None, why
        if why.endswith("outer"):
            assert mode == "big" and outer_grid_node_hit(hd, sector, big_cal) is None, why
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
    print("calibration grid points use exact-node routing; fallback points: 0")
    print("OK")


if __name__ == "__main__":
    main()
