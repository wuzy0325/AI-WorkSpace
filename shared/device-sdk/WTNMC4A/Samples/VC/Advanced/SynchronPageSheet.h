#if !defined(AFX_SYNCHRONPAGESHEET_H__E0EF7098_8965_4128_8C3E_6A1133545C49__INCLUDED_)
#define AFX_SYNCHRONPAGESHEET_H__E0EF7098_8965_4128_8C3E_6A1133545C49__INCLUDED_

#if _MSC_VER > 1000
#pragma once
#endif // _MSC_VER > 1000
// SynchronPageSheet.h : header file
//
#include "PageSynchron.h"
/////////////////////////////////////////////////////////////////////////////
// CSynchronPageSheet dialog

class CSynchronPageSheet : public CDialog
{
// Construction
public:
	CSynchronPageSheet(CWnd* pParent = NULL);   // standard constructor

// Dialog Data
	//{{AFX_DATA(CSynchronPageSheet)
	enum { IDD = IDD_Page_SynchronSheet };
		// NOTE: the ClassWizard will add data members here
	//}}AFX_DATA
	

// Overrides
	// ClassWizard generated virtual function overrides
	//{{AFX_VIRTUAL(CSynchronPageSheet)
	protected:
	virtual void DoDataExchange(CDataExchange* pDX);    // DDX/DDV support
	//}}AFX_VIRTUAL

// Implementation
protected:
	CTabCtrl	m_TabSynchron;
	CPageSynchron     m_PageSynchronX;          // 同步轴X设置页
	CPageSynchron     m_PageSynchronY;          // 同步轴Y设置页
	CPageSynchron     m_PageSynchronZ;          // 同步轴Z设置页
	CPageSynchron     m_PageSynchronU;          // 同步轴U设置页

	// Generated message map functions
	//{{AFX_MSG(CSynchronPageSheet)
	virtual BOOL OnInitDialog();
	afx_msg void OnSelchangeTABSynchron(NMHDR* pNMHDR, LRESULT* pResult);
	//}}AFX_MSG
	DECLARE_MESSAGE_MAP()
};

//{{AFX_INSERT_LOCATION}}
// Microsoft Visual C++ will insert additional declarations immediately before the previous line.

#endif // !defined(AFX_SYNCHRONPAGESHEET_H__E0EF7098_8965_4128_8C3E_6A1133545C49__INCLUDED_)
