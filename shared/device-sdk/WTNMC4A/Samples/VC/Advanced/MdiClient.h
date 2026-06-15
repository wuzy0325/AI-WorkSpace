#if !defined(AFX_MDICLIENT_H__07D11962_0399_11D3_BC0D_89BC9BB36074__INCLUDED_)
#define AFX_MDICLIENT_H__07D11962_0399_11D3_BC0D_89BC9BB36074__INCLUDED_



#if _MSC_VER >= 1000
#pragma once
#endif // _MSC_VER >= 1000
// MdiClient.h : header file
//
class CTabBar;

/////////////////////////////////////////////////////////////////////////////
// CMdiClient window
class CMdiClient : public CWnd
{
// Construction
public:
	CMdiClient();

// Attributes
public:
	CTabCtrlBarDoc* m_pWndTabs;
	COLORREF m_crBkColor;		
	CSize   m_sizeClient;   // 客户区大小
// Operations
public:
	void RemoveHandle(HWND hWnd);
	void AddHandle(HWND hWnd);

// Overrides
	// ClassWizard generated virtual function overrides
	//{{AFX_VIRTUAL(CMdiClient)
	//}}AFX_VIRTUAL

// Implementation
public:
	virtual ~CMdiClient();
	// Generated message map functions
protected:
	//{{AFX_MSG(CMdiClient)
	afx_msg BOOL OnEraseBkgnd(CDC* pDC);
	afx_msg void OnSize(UINT nType, int cx, int cy);

	//}}AFX_MSG
	afx_msg LRESULT OnMDICreate(WPARAM wParam, LPARAM lParam);
	afx_msg LRESULT OnMDIDestroy(WPARAM wParam, LPARAM lParam);
	DECLARE_MESSAGE_MAP()
};

#endif // !defined(AFX_MDICLIENT_H__07D11962_0399_11D3_BC0D_89BC9BB36074__INCLUDED_)
