#if !defined(AFX_PAGEINTERPOLATION_H__CACC032D_89B8_4881_A19A_44CBEA6BC1C7__INCLUDED_)
#define AFX_PAGEINTERPOLATION_H__CACC032D_89B8_4881_A19A_44CBEA6BC1C7__INCLUDED_

#if _MSC_VER > 1000
#pragma once
#endif // _MSC_VER > 1000
// PageInterpolation.h : header file
//

/////////////////////////////////////////////////////////////////////////////
// CPageInterpolation dialog

class CPageInterpolation : public CPropertyPage
{
	DECLARE_DYNCREATE(CPageInterpolation)

// Construction
public:
	CPageInterpolation();
	~CPageInterpolation();

// Dialog Data
	//{{AFX_DATA(CPageInterpolation)
	enum { IDD = IDD_Page_Interpolation };
	CButton	m_Button_StartSingleStepInterp;
	CButton	m_Button_SingleSetpInterp;
	CButton	m_Button_InstStop;
	CButton	m_Button_DecStop;
	CButton	m_Button_StartInterpolation;
	CEdit	m_Edit_HandDecNum;
	CStatic	m_Static_HandDecNum;
	CStatic	m_Static_AccK;
	CButton	m_Check_Constand;
	CEdit	m_Edit_AccK;
	CStatic	m_Static_ThirdPulseNum;
	CEdit	m_Edit_ThirdAxisPulseNum;
	CStatic	m_Static_ShortAxisCenter;
	CStatic	m_Static_LongAxisCenter;
	CEdit	m_Edit_ShortAxisCenter;
	CEdit	m_Edit_LongAxisCenter;
	CEdit	m_Edit_ShortAxisPulseNUm;
	CEdit	m_Edit_LongAxisPulseNum;
	CStatic	m_Static_ThirdAxis;
	CComboBox	m_Combo_ThirdAxisNum;
	CComboBox	m_Combo_SecondAxisNum;
	CComboBox	m_Combo_FirstAxisNum;
	CComboBox	m_Combo_DecType;
	CButton	    m_Check_LineCurve;
	CComboBox	m_Combo_InterMode;
	int		m_FirstAxisNum;
	//}}AFX_DATA
protected:
	BOOL m_bAxisNum[4];     // 是否已经选择了该轴
	BOOL m_bSingleStep;    // 单步插补
// Overrides
	// ClassWizard generate virtual function overrides
	//{{AFX_VIRTUAL(CPageInterpolation)
	protected:
	virtual void DoDataExchange(CDataExchange* pDX);    // DDX/DDV support
	//}}AFX_VIRTUAL
public:
	void EnableWindows(BOOL bEnable);

// Implementation
protected:
	void InitCfg();
	// Generated message map functions
	//{{AFX_MSG(CPageInterpolation)
	afx_msg void OnSelchangeCOMBOInterMode();
	afx_msg void OnAkordecnum();
	virtual BOOL OnInitDialog();
	afx_msg void OnSTARTInterpolation();
	afx_msg void OnCHECKLineCurve();
	afx_msg void OnSelchangeCOMBOFirstAxisNum();
	afx_msg void OnSelchangeCOMBOSecondAxisNum();
	afx_msg void OnSelchangeCOMBOThirdAxisNum();
	afx_msg void OnChangeLongAxisPulseNum();
	afx_msg void OnChangeShortAxisPulseNum();
	afx_msg void OnChangeEDITXCenter();
	afx_msg void OnChangeEDITYCenter();
	afx_msg void OnChangeThirdAxisPulseNum();
	afx_msg void OnSTOPDecStop();
	afx_msg void OnSTOPInstStop();
	afx_msg void OnCHECKConstand();
	afx_msg void OnSelchangeDectype();
	afx_msg void OnChangeEditAccK();
	afx_msg void OnChangeEDITHandDecNum();
	afx_msg void OnCHECKSingleStepInterp();
	afx_msg void OnRADIOSingleStepInterpCom();
	afx_msg void OnRADIOSingleStepInterpExt();
	afx_msg void OnBUTTONStartSingleStepInterp();
	//}}AFX_MSG
	DECLARE_MESSAGE_MAP()

};

//{{AFX_INSERT_LOCATION}}
// Microsoft Visual C++ will insert additional declarations immediately before the previous line.

#endif // !defined(AFX_PAGEINTERPOLATION_H__CACC032D_89B8_4881_A19A_44CBEA6BC1C7__INCLUDED_)
