#if !defined(AFX_PAGEHARDLIMIT_H__4405F5EF_56E0_4736_B4F8_6E1FFD82A127__INCLUDED_)
#define AFX_PAGEHARDLIMIT_H__4405F5EF_56E0_4736_B4F8_6E1FFD82A127__INCLUDED_

#if _MSC_VER > 1000
#pragma once
#endif // _MSC_VER > 1000
// PageHardLimit.h : header file
//
#include "FilterSetDlg.h"
/////////////////////////////////////////////////////////////////////////////
// CPageHardLimit dialog

class CPageHardLimit : public CPropertyPage
{
	DECLARE_DYNCREATE(CPageHardLimit)

// Construction
public:
	void SetCurrentAxisNum(int nCurrentAxis);
	void InitCfg(int nCurrentAxis);
	CPageHardLimit();
	~CPageHardLimit();
	int m_nStopNum;
	BOOL   m_nStopNumSts[4];				// 外部停止信号设置状态

// Dialog Data
	//{{AFX_DATA(CPageHardLimit)
	enum { IDD = IDD_Page_HardLimit };
	CButton	m_Button_ClearInPos;
	CButton	m_Button_SetInPos;
	CButton	m_Button_ClearAlarm;
	CButton	m_Button_Alarm;
	CButton	m_Button_SetHLimit;
	CButton	m_Button_ClearStopNum;
	CButton	m_Button_SetStopNum;
	CButton	m_Button_FilterSet;
	CButton	m_Button_FilterEnable;
	CComboBox	m_Combo_StopNum;
	CComboBox	m_Combo_StopType;
	//}}AFX_DATA

	CFilterSetDlg m_FilterSetDlg; 
// Overrides
	// ClassWizard generate virtual function overrides
	//{{AFX_VIRTUAL(CPageHardLimit)
	protected:
	virtual void DoDataExchange(CDataExchange* pDX);    // DDX/DDV support
	//}}AFX_VIRTUAL

// Implementation
protected:
	// Generated message map functions
	//{{AFX_MSG(CPageHardLimit)
	afx_msg void OnSelchangeCOMBOStopType();
	afx_msg void OnSetHLimit();
	afx_msg void OnSetALARM();
	afx_msg void OnClearALARM();
	afx_msg void OnSetStopNum();
	afx_msg void OnClearStopNum();
	afx_msg void OnSetInPos();
	afx_msg void OnClearInPos();
	afx_msg void OnSelchangeCOMBOStopNum();
	virtual BOOL OnInitDialog();
	afx_msg void OnCHECKFilterEnable();
	afx_msg void OnBUTTONFilterSet();
	//}}AFX_MSG
	DECLARE_MESSAGE_MAP()

};

//{{AFX_INSERT_LOCATION}}
// Microsoft Visual C++ will insert additional declarations immediately before the previous line.

#endif // !defined(AFX_PAGEHARDLIMIT_H__4405F5EF_56E0_4736_B4F8_6E1FFD82A127__INCLUDED_)
