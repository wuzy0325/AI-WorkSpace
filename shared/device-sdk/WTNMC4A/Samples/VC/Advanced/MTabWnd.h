#if !defined(AFX_MTABWND_H__E23B47A5_FE45_11D2_BC0D_006008CCD137__INCLUDED_)
#define AFX_MTABWND_H__E23B47A5_FE45_11D2_BC0D_006008CCD137__INCLUDED_

#if _MSC_VER >= 1000
#pragma once
#endif // _MSC_VER >= 1000
// MTabWnd.h : header file
//

#include "TabCtrlBarDoc.h"
#include "MdiClient.h"

/////////////////////////////////////////////////////////////////////////////
// CMDITabFrameWnd frame
class CMDITabFrameWnd : public CMDIFrameWnd
{
	DECLARE_DYNCREATE(CMDITabFrameWnd)
protected:
	CMDITabFrameWnd();           // protected constructor used by dynamic creation

// Attributes
public:
	CTabCtrlBarDoc  m_wndTabs;
	CMdiClient m_wndMdiClient;
// Operations
public:
	BOOL CreateTabs(void);
// Overrides
	// ClassWizard generated virtual function overrides
	//{{AFX_VIRTUAL(CMDITabFrameWnd)
	protected:
	//}}AFX_VIRTUAL
protected:
	virtual ~CMDITabFrameWnd();
	afx_msg void OnInitMenu(CMenu* pMenu);
	// Generated message map functions
	//{{AFX_MSG(CMDITabFrameWnd)
	afx_msg void OnWindowNextPane();
	afx_msg void OnWindowPrevPane();
	//}}AFX_MSG
	DECLARE_MESSAGE_MAP()
};

#endif // !defined(AFX_MTABWND_H__E23B47A5_FE45_11D2_BC0D_006008CCD137__INCLUDED_)
