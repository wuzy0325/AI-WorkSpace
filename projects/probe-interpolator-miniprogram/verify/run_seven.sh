#!/usr/bin/env bash
# 七孔移植数值校验：Go 原版 → reference_seven.json → Node 对比
set -e
cd "$(dirname "$0")"

PB="../../../shared/algorithms/go/sevenhole/interpolation/testdata/prb"
GL="../../../shared/algorithms/go/sevenhole/interpolation/testdata/golden"

GOWORK=off go mod tidy
GOWORK=off go run genref_seven.go "$PB" "$GL/golden.json" "$GL/boundary.json" reference_seven.json
node verify_seven.js
node verify_seven_csv.js
