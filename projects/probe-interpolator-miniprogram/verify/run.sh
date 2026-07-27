#!/usr/bin/env bash
# 一键数值校验：生成 Go 参考 -> Node 对比 TS 端口
set -e
cd "$(dirname "$0")"
GOLDEN="../../../shared/algorithms/go/fivehole/interpolation/testdata/golden/prb"
echo ">> 生成 Go 参考 (reference.json)"
GOWORK=off go run genref.go "$GOLDEN" reference.json
echo ">> Node 对比 TS 端口"
node verify.js
