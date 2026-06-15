#pragma once

class CChildFrame;

// CListSegmentInfo

class CListSegmentInfo : public CListCtrl
{
	DECLARE_DYNAMIC(CListSegmentInfo)

public:
	CListSegmentInfo();
	virtual ~CListSegmentInfo();
	CChildFrame* m_pParentFrm;
	int m_iNum; // Edit控件对应的数字
public:
	inline void HideCtrl()
	{
		if (m_btnWave.GetSafeHwnd())
		{
			m_btnWave.ShowWindow(SW_HIDE);
		}
		if (m_ctrEdit.GetSafeHwnd())
		{
			m_ctrEdit.ShowWindow(SW_HIDE);
		}
	}

	inline int GetCurSelSegment() const
	{
		return m_iSelItem;
	}

protected:
	int m_iCurColum;
	int m_iCurRow;
	int m_iSelItem;
	int m_iLength;  // 当前数据段的数据长度
	int m_iLoopCount;  // 当前数据段的循环次数
	long m_iWaveType; // 当前波形
	enum
	{
		EDIT_ID = 0X0012,
		BTN_ID 
	};
	enum TIMER_ID
	{
		HIDE_CTR_ID = 100
	};
	
	DECLARE_MESSAGE_MAP()
	virtual void PreSubclassWindow();
	CEdit m_ctrEdit;
	CButton m_btnWave;
	CRect m_rcEdit;
	CRect m_rcBtn;
public:
	afx_msg int OnCreate(LPCREATESTRUCT lpCreateStruct);
protected:
	virtual BOOL PreCreateWindow(CREATESTRUCT& cs);
	virtual void DoDataExchange(CDataExchange* pDX);	// DDX/DDV 支持
public:
	virtual BOOL OnCreateAggregates();
	virtual BOOL Create(DWORD dwStyle, const RECT& rect, CWnd* pParentWnd, UINT nID);
	BOOL InitListView(void);
	afx_msg void OnLButtonDown(UINT nFlags, CPoint point);
	afx_msg void OnEnChanngeEdit();
	afx_msg void OnVScroll(UINT nSBCode, UINT nPos, CScrollBar* pScrollBar);
	afx_msg void OnHScroll(UINT nSBCode, UINT nPos, CScrollBar* pScrollBar);
	afx_msg BOOL OnMouseWheel(UINT nFlags, short zDelta, CPoint pt);
	afx_msg void OnBtnClickedWave();
	afx_msg void OnRButtonUp(UINT nFlags, CPoint point);
	afx_msg void OnRButtonDown(UINT nFlags, CPoint point);
	afx_msg void OnItemDel();
	afx_msg void OnKeyDown(UINT nChar, UINT nRepCnt, UINT nFlags);
	afx_msg void OnLButtonUp(UINT nFlags, CPoint point);
	afx_msg void OnBnKillfocusWave();
};


