#if !defined(AFX_PAGELINE_H__3D91F7D4_0D70_4D20_8596_C41AC4C3A0E2__INCLUDED_)
#define AFX_PAGELINE_H__3D91F7D4_0D70_4D20_8596_C41AC4C3A0E2__INCLUDED_

#if _MSC_VER > 1000
#pragma once
#endif // _MSC_VER > 1000
// PageLine.h : header file
//
#include "PageSynchronSet.h"
#include "AutoHomeSearchDlg.h"
/////////////////////////////////////////////////////////////////////////////
// CPageLine dialog

class CPageLine : public CPropertyPage
{
	DECLARE_DYNCREATE(CPageLine)

// Construction
public:
	void SetFunction(int nFunction);
	void SetCurrentAxisNum(int nCurrentAxis);
	void ShowSynchronSetWnd(BOOL bShow);
	void EnableSynchronWindows(BOOL bEnable);
	

	CPageLine();
	~CPageLine();
	void InitCfg(int nCurrentAxis);
	void EnableWindows(BOOL bEnable);


// Dialog Data
	//{{AFX_DATA(CPageLine)
	enum { IDD = IDD_Page_Line };
	CButton	m_Button_HomeSearchSet;
	CButton	m_Button_OutStartAll;
	CButton	m_Button_InstStopAll;
	CButton	m_Button_DecStopAll;
	CButton	m_Button_StartLineAll;
	CPageSynchronSet m_PageSynchronSet;
	CAutoHomeSearchDlg m_HomeSearchDlg;
	CStatic	m_Static_DVImpulseNum;
	CEdit	m_Edit_DVImpulseNum;
	CEdit	m_Edit_HandDecNum;
	CComboBox	m_Combo_DecType;
	CStatic	m_Static_AccIncRate;
	CStatic	m_Static_DecelerationK;
	CEdit	m_Edit_DecelerationK;
	CEdit		m_Edit_AccIncRate;
	CComboBox	m_Combo_ImpulseType;
	CComboBox	m_Combo_LineMode;
	CButton	m_Button_OutStartX;
	CButton	m_Button_OutStartY;
	CButton	m_Button_OutStartZ;
	CButton	m_Button_OutStartU;
	CButton	m_Button_StartLineX;
	CButton	m_Button_StartLineY;
	CButton	m_Button_StartLineZ;
	CButton	m_Button_StartLineU;
	CButton	m_Button_InstStopX;
	CButton	m_Button_InstStopY;
	CButton	m_Button_InstStopZ;
	CButton	m_Button_InstStopU;
	CButton	m_Button_DecStopX;
	CButton	m_Button_DecStopY;
	CButton	m_Button_DecStopZ;
	CButton	m_Button_DecStopU;
	//}}AFX_DATA
protected:

	int m_nCurrentAxis;
	int m_nFunction; // 选择运动方式(线性|同步|原点搜寻)
// Overrides
	// ClassWizard generate virtual function overrides
	//{{AFX_VIRTUAL(CPageLine)
	protected:
	virtual void DoDataExchange(CDataExchange* pDX);    // DDX/DDV support
	//}}AFX_VIRTUAL

// Implementation
protected:
	void StartFuncMovement(int nAxisNum);

	// Generated message map functions
	//{{AFX_MSG(CPageLine)
	afx_msg void OnSelchangeComboType();
	afx_msg void OnSelchangeComboAxis();
	afx_msg void OnRADIOForward();
	afx_msg void OnRADIOReverse();
	afx_msg void OnSelchangeCOMBOLineMode();
	afx_msg void OnSelchangeCOMBOImpulseType();
	virtual BOOL OnInitDialog();
	afx_msg void OnKillfocusEditDecacck();
	afx_msg void OnKillfocusEDITAccIncRate();
	afx_msg void OnKillfocusEDITAccOffset();
	afx_msg void OnSelchangeCOMBOLineDecType();
	afx_msg void OnChangeEDITSCurveHandDecNum();
	afx_msg void OnChangeEDITDVImpulseNum();
	afx_msg void OnStartLineX();
	afx_msg void OnStartLineY();
	afx_msg void OnStartLineZ();
	afx_msg void OnStartLineU();
	afx_msg void OnOutStartX();
	afx_msg void OnOutStartY();
	afx_msg void OnOutStartZ();
	afx_msg void OnOutStartU();
	afx_msg void OnSTOPDecStopX();
	afx_msg void OnSTOPDecStopY();
	afx_msg void OnSTOPDecStopZ();
	afx_msg void OnSTOPDecStopU();
	afx_msg void OnSTOPInstStopX();
	afx_msg void OnSTOPInstStopY();
	afx_msg void OnSTOPInstStopZ();
	afx_msg void OnSTOPInstStopU();
	afx_msg void OnStartLineall();
	afx_msg void OnOutStartAll();
	afx_msg void OnSTOPDecStopAll();
	afx_msg void OnSTOPInstStopAll();
	afx_msg void OnBUTTONHomeSearchSet();
	afx_msg void OnButton1();
	//}}AFX_MSG
	DECLARE_MESSAGE_MAP()

};

//{{AFX_INSERT_LOCATION}}
// Microsoft Visual C++ will insert additional declarations immediately before the previous line.

#endif // !defined(AFX_PAGELINE_H__3D91F7D4_0D70_4D20_8596_C41AC4C3A0E2__INCLUDED_)
