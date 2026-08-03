Page({
  go(e) {
    const kind = e.currentTarget.dataset.kind;
    wx.navigateTo({ url: '/pages/' + kind + '/' + kind });
  },
});
