#if !defined(AFX_INTERRUPTSETDLG_H__E12370F2_7EB1_432C_9671_DB4086851DBC__INCLUDED_)
#define AFX_INTERRUPTSETDLG_H__E12370F2_7EB1_432C_9671_DB4086851DBC__INCLUDED_

#if _MSC_VER > 1000
#pragma once
#endif // _MSC_VER > 1000
// InterruptSetDlg.h : header file
//

/////////////////////////////////////////////////////////////////////////////
// CInterruptSetDlg dialog

class CInterruptSetDlg : public CDialog
{
// Construction
public:
	CInterruptSetDlg(CWnd* pParent = NULL);   // standard constructor

// Dialog Data
	//{{AFX_DATA(CInterruptSetDlg)
	enum { IDD = IDD_DlALOG_InterruptSet };
	CButton	m_Button_PULSE;
	CButton	m_Button_PSCP;
	CButton	m_Button_PSCM;
	CButton	m_Button_PBCP;
	CButton	m_Button_PBCM;
	CButton	m_Button_DEND;
	CButton	m_Button_CSTA;
	CButton	m_Button_CIINT;
	CButton	m_Button_CDEC;
	CButton	m_Button_BPINT;
	CEdit	m_Edit_COMP;
	CEdit	m_Edit_COMN;
	//}}AFX_DATA


// Overrides
	// ClassWizard generated virtual function overrides
	//{{AFX_VIRTUAL(CInterruptSetDlg)
	protected:
	virtual void DoDataExchange(CDataExchange* pDX);    // DDX/DDV support
	//}}AFX_VIRTUAL

// Implementation
protected:

	// Generated message map functions
	//{{AFX_MSG(CInterruptSetDlg)
	afx_msg void OnCheckIntPbcm();
	afx_msg void OnCheckIntPscm();
	afx_msg void OnCheckIntPscp();
	afx_msg void OnCheckIntPbcp();
	afx_msg void OnCheckIntPulse();
	afx_msg void OnCheckIntCdec();
	afx_msg void OnCheckIntCsta();
	afx_msg void OnCheckIntDend();
	afx_msg void OnCheckIntCiint();
	afx_msg void OnCheckIntBpint();
	afx_msg void OnChangeEditIntComp();
	afx_msg void OnChangeEditIntComn();
	afx_msg void OnButton1();
	virtual BOOL OnInitDialog();
	//}}AFX_MSG
	DECLARE_MESSAGE_MAP()
};

//{{AFX_INSERT_LOCATION}}
// Microsoft Visual C++ will insert additional declarations immediately before the previous line.

#endif // !defined(AFX_INTERRUPTSETDLG_H__E12370F2_7EB1_432C_9671_DB4086851DBC__INCLUDED_)
