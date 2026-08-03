App({
  globalData: {
    // 跨页共享：已加载的校准文件（按探针类型隔离）
    calibration: {
      three: [],
      five: [],
      seven: [],
    },
    // 跨页共享：结果展示单位偏好（输入固定国际单位 Pa/°C/deg，不影响计算与 CSV 导入）
    units: {
      pressure: 'Pa',   // Pa | kPa | MPa
      velocity: 'm/s',  // m/s | km/h
      angle: 'deg',     // deg | rad
      temp: '°C',       // °C | K | °F
    },
  },
});
