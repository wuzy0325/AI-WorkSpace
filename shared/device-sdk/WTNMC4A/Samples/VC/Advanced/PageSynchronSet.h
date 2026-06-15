#if !defined(AFX_PAGESYNCHRONSET_H__69563F08_7277_484A_ADF7_EC0CE2DBB31C__INCLUDED_)
#define AFX_PAGESYNCHRONSET_H__69563F08_7277_484A_ADF7_EC0CE2DBB31C__INCLUDED_

#if _MSC_VER > 1000
#pragma once
#endif // _MSC_VER > 1000
// PageSynchronSet.h : header file
//
#include "SynchronPageSheet.h"
/////////////////////////////////////////////////////////////////////////////
// CPageSynchronSet dialog

class CPageSynchronSet : public CPropertyPage
{
	DECLARE_DYNCREATE(CPageSynchronSet)

// Construction
public:
	void SetCurrentAxisNum(int nCurrentAxis);
	CPageSynchronSet();
	~CPageSynchronSet();

// Dialog Data
	//{{AFX_DATA(CPageSynchronSet)
	enum { IDD = IDD_Page_SynchronSet };
	CEdit	m_Edit_COMPN;
	CEdit	m_Edit_COMPP;
	int     m_nCurrentAxis;
	//}}AFX_DATA

	CSynchronPageSheet m_SynchronSheet;
// Overrides
	// ClassWizard generate virtual function overrides
	//{{AFX_VIRTUAL(CPageSynchronSet)
	protected:
	virtual void DoDataExchange(CDataExchange* pDX);    // DDX/DDV support
	//}}AFX_VIRTUAL

// Implementation
protected:
	// Generated message map functions
	//{{AFX_MSG(CPageSynchronSet)
	afx_msg void OnBUTTONSynchronSet();
	virtual BOOL OnInitDialog();
	afx_msg void OnChangeEditCompp();
	afx_msg void OnChangeEditCompn();
	afx_msg void OnBUTTONStartSynchronActionX();
	afx_msg void OnBUTTONStartSynchronActionY();
	afx_msg void OnBUTTONStartSynchronActionU();
	afx_msg void OnBUTTONStartSynchronActionZ();
	//}}AFX_MSG
	DECLARE_MESSAGE_MAP()

};

//{{AFX_INSERT_LOCATION}}
// Microsoft Visual C++ will insert additional declarations immediately before the previous line.

#endif // !defined(AFX_PAGESYNCHRONSET_H__69563F08_7277_484A_ADF7_EC0CE2DBB31C__INCLUDED_)
