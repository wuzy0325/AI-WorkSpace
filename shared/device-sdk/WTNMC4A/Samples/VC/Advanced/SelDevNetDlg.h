#if !defined(AFX_SELDEVNETDLG_H__92A497AE_9EB7_45F9_968F_F21AB532C6A5__INCLUDED_)
#define AFX_SELDEVNETDLG_H__92A497AE_9EB7_45F9_968F_F21AB532C6A5__INCLUDED_

#if _MSC_VER > 1000
#pragma once
#endif // _MSC_VER > 1000
// SelDevNetDlg.h : header file
//

/////////////////////////////////////////////////////////////////////////////
// CSelDevNetDlg dialog

class CSelDevNetDlg : public CDialog
{
// Construction
public:
	CSelDevNetDlg(CWnd* pParent = NULL);   // standard constructor

// Dialog Data
	//{{AFX_DATA(CSelDevNetDlg)
	enum { IDD = IDD_DLG_SEL_DEV_NET };
	CIPAddressCtrl	m_IPAddr;
	//}}AFX_DATA


// Overrides
	// ClassWizard generated virtual function overrides
	//{{AFX_VIRTUAL(CSelDevNetDlg)
	protected:
	virtual void DoDataExchange(CDataExchange* pDX);    // DDX/DDV support
	//}}AFX_VIRTUAL

// Implementation
protected:

	// Generated message map functions
	//{{AFX_MSG(CSelDevNetDlg)
	afx_msg void OnBUTTONLink();
	virtual BOOL OnInitDialog();
	//}}AFX_MSG
	DECLARE_MESSAGE_MAP()
};

//{{AFX_INSERT_LOCATION}}
// Microsoft Visual C++ will insert additional declarations immediately before the previous line.

#endif // !defined(AFX_SELDEVNETDLG_H__92A497AE_9EB7_45F9_968F_F21AB532C6A5__INCLUDED_)
