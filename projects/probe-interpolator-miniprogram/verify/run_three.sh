#!/usr/bin/env bash
# 一键数值校验（三孔）：生成 Go 参考 -> Node 对比 JS 端口
set -e
cd "$(dirname "$0")"
GOLDEN="../../../shared/algorithms/go/threehole/interpolation/testdata/golden/threehole"
PRB="../../../shared/algorithms/go/threehole/interpolation/testdata/0.8.prb"
echo ">> go mod tidy"
GOWORK=off go mod tidy
echo ">> 生成 Go 参考 (reference_three.json)"
GOWORK=off go run genref_three.go "$GOLDEN" reference_three.json "$PRB"
echo ">> Node 对比 JS 端口"
node verify_three.js
