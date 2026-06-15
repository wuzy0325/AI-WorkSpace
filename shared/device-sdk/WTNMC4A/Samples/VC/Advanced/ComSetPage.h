#if !defined(AFX_COMSETPAGE_H__F2883D4D_A550_4FEF_951B_D0DECBC51A8A__INCLUDED_)
#define AFX_COMSETPAGE_H__F2883D4D_A550_4FEF_951B_D0DECBC51A8A__INCLUDED_

#if _MSC_VER > 1000
#pragma once
#endif // _MSC_VER > 1000
// ComSetPage.h : header file
//
#include "InterruptSetDlg.h"
/////////////////////////////////////////////////////////////////////////////
// CPageComSet dialog

class CPageComSet : public CPropertyPage
{
	DECLARE_DYNCREATE(CPageComSet)

// Construction
public:
	void ShowDVImpulseNumWindow(BOOL bShow );
	void EnableWindows(BOOL bEnable);
	void SetCurrentAxisNum(int nAxisNum);
	void InitCfg(int nAxisNum);
	CPageComSet();
	~CPageComSet();

// Dialog Data
	//{{AFX_DATA(CPageComSet)
	enum { IDD = IDD_Page_ComSet };
	CSpinButtonCtrl	m_Spin_Multiple;
	CEdit	m_Edit_Multiple;
	CComboBox	m_Combo_ImpulseModeIN;
	CComboBox	m_Combo_DirLogLever;
	CComboBox	m_Combo_PulseLogLever;
	CInterruptSetDlg  m_InterruptSetDlg;
	CEdit	m_Edit_AccOffset;
	CComboBox	m_Combo_ImpulseMode;
	CEdit	m_Edit_StartRate;
	CEdit	m_Edit_DriveRate;
	CEdit	m_Edit_DecAcc;
	CEdit	m_Edit_Acceleration;
	CComboBox	m_Combo_AxisNum;
	int m_nCurrentAxis;
	//}}AFX_DATA


// Overrides
	// ClassWizard generate virtual function overrides
	//{{AFX_VIRTUAL(CPageComSet)
	protected:
	virtual void DoDataExchange(CDataExchange* pDX);    // DDX/DDV support
	//}}AFX_VIRTUAL

// Implementation
protected:
	LRESULT SCurveDecTypeChange(WPARAM wParam, LPARAM lParam);
	// Generated message map functions
	//{{AFX_MSG(CPageComSet)
	virtual BOOL OnInitDialog();
	afx_msg void OnSelchangeComboAxis();
	afx_msg void OnChangeEDITStartRate();
	afx_msg void OnChangeEDITDriveRate();
	afx_msg void OnSelchangeCOMBOImpulseMode();
	afx_msg void OnChangeEDITAccOffset();
	afx_msg void OnBUTTONInterrruptSet();
	afx_msg void OnChangeEDITAcceleration();
	afx_msg void OnChangeEditDecacc();
	afx_msg void OnSelchangeCOMBOPulseLogLever();
	afx_msg void OnSelchangeCOMBODirLogLever();
	afx_msg void OnEditchangeCOMBOImpulseModeIN();
	afx_msg void OnChangeEDITMultiple();
	afx_msg void OnSelchangeCOMBOImpulseModeIN();
	//}}AFX_MSG
	DECLARE_MESSAGE_MAP()

};

//{{AFX_INSERT_LOCATION}}
// Microsoft Visual C++ will insert additional declarations immediately before the previous line.

#endif // !defined(AFX_COMSETPAGE_H__F2883D4D_A550_4FEF_951B_D0DECBC51A8A__INCLUDED_)
