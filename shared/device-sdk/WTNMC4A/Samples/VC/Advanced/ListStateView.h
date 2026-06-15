#pragma once


// CListStateView 视图

class CListStateView : public CListView
{
	DECLARE_DYNCREATE(CListStateView)

protected:
	CListStateView();           // 动态创建所使用的受保护的构造函数
	virtual ~CListStateView();

protected:
	DECLARE_MESSAGE_MAP()
	const UINT m_nTimerId;
	CChildFrame* m_pParentFrm;
public:
	virtual void OnInitialUpdate();
	PCI8603_STATUS_DA m_DAStatus;   // 保存AD的状态
	void StartUpataDAStatus();      // 开始更新DA的状态
	void StopUpataDAStatus();       // 停止更新DA的状态
	UINT m_iElapse;                 // 定时器间隔
	INT  m_nReadTimes;
	

public:
#ifdef _DEBUG
	virtual void AssertValid() const;
	virtual void Dump(CDumpContext& dc) const;
#endif
	afx_msg void OnTimer(UINT nIDEvent);
};


