#if !defined(AFX_FILTERSETDLG_H__1D13FFD0_8634_47FB_BC3F_718C3E56ADAE__INCLUDED_)
#define AFX_FILTERSETDLG_H__1D13FFD0_8634_47FB_BC3F_718C3E56ADAE__INCLUDED_

#if _MSC_VER > 1000
#pragma once
#endif // _MSC_VER > 1000
// FilterSetDlg.h : header file
//

/////////////////////////////////////////////////////////////////////////////
// CFilterSetDlg dialog

class CFilterSetDlg : public CDialog
{
// Construction
public:
	CFilterSetDlg(CWnd* pParent = NULL);   // standard constructor

// Dialog Data
	//{{AFX_DATA(CFilterSetDlg)
	enum { IDD = IDD_DIALOG_FilterSet };
	CStatic	m_Static_SignalDelay;
	CComboBox	m_Combo_TimeConst;
	CButton	m_Button_FE4;
	CButton	m_Button_FE3;
	CButton	m_Button_FE2;
	CButton	m_Button_FE1;
	CButton	m_Button_FE0;
	//}}AFX_DATA
	int m_nCurrentAxis;

// Overrides
	// ClassWizard generated virtual function overrides
	//{{AFX_VIRTUAL(CFilterSetDlg)
	protected:
	virtual void DoDataExchange(CDataExchange* pDX);    // DDX/DDV support
	//}}AFX_VIRTUAL

// Implementation
protected:
	CString GetSignalDelay(int nIndex);

	// Generated message map functions
	//{{AFX_MSG(CFilterSetDlg)
	virtual BOOL OnInitDialog();
	afx_msg void OnSelchangeCOMBOTimeConst();
	afx_msg void OnCheckFe0();
	afx_msg void OnCheckFe1();
	afx_msg void OnCheckFe2();
	afx_msg void OnCheckFe3();
	afx_msg void OnCheckFe4();
	afx_msg void OnClose();
	virtual void OnOK();
	//}}AFX_MSG
	DECLARE_MESSAGE_MAP()
};

//{{AFX_INSERT_LOCATION}}
// Microsoft Visual C++ will insert additional declarations immediately before the previous line.

#endif // !defined(AFX_FILTERSETDLG_H__1D13FFD0_8634_47FB_BC3F_718C3E56ADAE__INCLUDED_)
