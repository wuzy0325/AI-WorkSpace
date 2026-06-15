#if !defined(AFX_DEVNETCFGDLG_H__687D2093_C1DA_46E0_9472_F2F95DF1EABD__INCLUDED_)
#define AFX_DEVNETCFGDLG_H__687D2093_C1DA_46E0_9472_F2F95DF1EABD__INCLUDED_

#if _MSC_VER > 1000
#pragma once
#endif // _MSC_VER > 1000
// DevNetCfgDlg.h : header file
//

/////////////////////////////////////////////////////////////////////////////
// CDevNetCfgDlg dialog

class CDevNetCfgDlg : public CDialog
{
// Construction
public:
	CDevNetCfgDlg(CWnd* pParent = NULL);   // standard constructor

// Dialog Data
	//{{AFX_DATA(CDevNetCfgDlg)
	enum { IDD = IDD_DLG_DEV_NET_CFG };
		// NOTE: the ClassWizard will add data members here
	//}}AFX_DATA


// Overrides
	// ClassWizard generated virtual function overrides
	//{{AFX_VIRTUAL(CDevNetCfgDlg)
	protected:
	virtual void DoDataExchange(CDataExchange* pDX);    // DDX/DDV support
	//}}AFX_VIRTUAL

// Implementation
protected:

	// Generated message map functions
	//{{AFX_MSG(CDevNetCfgDlg)
	afx_msg void OnBUTTONModify();
	virtual BOOL OnInitDialog();
	//}}AFX_MSG
	DECLARE_MESSAGE_MAP()
};

//{{AFX_INSERT_LOCATION}}
// Microsoft Visual C++ will insert additional declarations immediately before the previous line.

#endif // !defined(AFX_DEVNETCFGDLG_H__687D2093_C1DA_46E0_9472_F2F95DF1EABD__INCLUDED_)
