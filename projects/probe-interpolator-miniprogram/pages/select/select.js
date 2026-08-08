Page({
  go(e) {
    const kind = e.currentTarget.dataset.kind;
    wx.navigateTo({ url: '/pages/' + kind + '/' + kind });
  },
  // 跳转使用说明页（不带 tab 参数，help 页 onLoad 默认回退 'three'）
  goHelp() {
    wx.navigateTo({ url: '/pages/help/help' });
  },
});
