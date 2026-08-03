// 算法模块统一出口
const { PrbInterpolator } = require('./fivehole/prb-interpolator.js');
const { MultiPrbInterpolator, MODE_NEAREST, MODE_LINEAR } = require('./fivehole/multi-prb-interpolator.js');
const { ThreeHoleInterpolator } = require('./threehole/three-hole.js');
const { AtmosphericDataCalculator } = require('./atmospheric.js');
const { SevenHolePrbInterpolator } = require('./sevenhole/seven-hole.js');

module.exports = {
  PrbInterpolator,
  MultiPrbInterpolator,
  ThreeHoleInterpolator,
  SevenHolePrbInterpolator,
  MODE_NEAREST,
  MODE_LINEAR,
  AtmosphericDataCalculator,
};
