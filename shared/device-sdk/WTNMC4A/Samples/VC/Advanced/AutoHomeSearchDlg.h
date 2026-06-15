#if !defined(AFX_AUTOHOMESEARCHDLG_H__FC487D83_BCD2_4575_B035_2CD3AB63E497__INCLUDED_)
#define AFX_AUTOHOMESEARCHDLG_H__FC487D83_BCD2_4575_B035_2CD3AB63E497__INCLUDED_

#if _MSC_VER > 1000
#pragma once
#endif // _MSC_VER > 1000
// AutoHomeSearchDlg.h : header file
//

/////////////////////////////////////////////////////////////////////////////
// CAutoHomeSearchDlg dialog

class CAutoHomeSearchDlg : public CDialog
{
// Construction
public:
	CAutoHomeSearchDlg(CWnd* pParent = NULL);   // standard constructor

// Dialog Data
	//{{AFX_DATA(CAutoHomeSearchDlg)
	enum { IDD = IDD_Page_AutoHomeSearch };
	CEdit	m_Edit_LowSpeed;
	CEdit	m_Edit_HighSpeed;
	CButton	m_Button_LIMIT;
	CButton	m_Button_PCLR;
	CButton	m_Button_SAND;
	CEdit	m_Edit_OffsetPulseNum;
	CButton	m_Button_ST4E;
	CButton	m_Button_ST3E;
	CButton	m_Button_ST2E;
	CButton	m_Button_ST1E;
	//}}AFX_DATA
// Overrides
	// ClassWizard generated virtual function overrides
	//{{AFX_VIRTUAL(CAutoHomeSearchDlg)
	protected:
	virtual void DoDataExchange(CDataExchange* pDX);    // DDX/DDV support
	//}}AFX_VIRTUAL

// Implementation
protected:
	int m_nCurrentAxis;
	// Generated message map functions
	//{{AFX_MSG(CAutoHomeSearchDlg)
	afx_msg void OnBUTTONOnOK();
	afx_msg void OnCheckSt1e();
	virtual BOOL OnInitDialog();
	afx_msg void OnCheckSt2e();
	afx_msg void OnCheckSt3e();
	afx_msg void OnCheckSt4e();
	afx_msg void OnRadioSt1dp();
	afx_msg void OnRadioSt1dn();
	afx_msg void OnRadioSt2dp();
	afx_msg void OnRadioSt2dn();
	afx_msg void OnRadioSt3dp();
	afx_msg void OnRadioSt3dn();
	afx_msg void OnRadioSt4dp();
	afx_msg void OnRadioSt4dn();
	afx_msg void OnChangeEDITOffsetPulseNum();
	afx_msg void OnCheckSand();
	afx_msg void OnCheckPclr();
	afx_msg void OnCheckLimit();
	afx_msg void OnChangeEDITHighSpeed();
	afx_msg void OnChangeEDITLowSpeed();
	afx_msg void OnRadioIn0h();
	afx_msg void OnRadioIn0l();
	afx_msg void OnRadioIn1h();
	afx_msg void OnRadioIn1l();
	afx_msg void OnRadioIn2h();
	afx_msg void OnRadioIn2l();
	afx_msg void OnClose();
	//}}AFX_MSG
	DECLARE_MESSAGE_MAP()
};

//{{AFX_INSERT_LOCATION}}
// Microsoft Visual C++ will insert additional declarations immediately before the previous line.

#endif // !defined(AFX_AUTOHOMESEARCHDLG_H__FC487D83_BCD2_4575_B035_2CD3AB63E497__INCLUDED_)
