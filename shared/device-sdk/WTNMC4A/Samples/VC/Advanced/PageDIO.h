#if !defined(AFX_PAGEDIO_H__72CDBA7E_5F77_486F_ACE2_1736CAAC04E2__INCLUDED_)
#define AFX_PAGEDIO_H__72CDBA7E_5F77_486F_ACE2_1736CAAC04E2__INCLUDED_

#if _MSC_VER > 1000
#pragma once
#endif // _MSC_VER > 1000
// PageDIO.h : header file
//

/////////////////////////////////////////////////////////////////////////////
// CPageDIO dialog

class CPageDIO : public CPropertyPage
{
	DECLARE_DYNCREATE(CPageDIO)

// Construction
public:
	void RefreshButton(PWTNMC4A_PARA_RR3 RR3, PWTNMC4A_PARA_RR4 RR4);
	CPageDIO();
	~CPageDIO();
// Dialog Data
	//{{AFX_DATA(CPageDIO)
	enum { IDD = IDD_Page_DIO };
	//}}AFX_DATA
protected:
	BOOL m_bDOX[8];
	BOOL m_bDIX[9];
	BOOL m_bDOY[8];
	BOOL m_bDIY[9];
	BOOL m_bDOZ[8];
	BOOL m_bDIZ[9];
	BOOL m_bDOU[8];
	BOOL m_bDIU[9];

CButton  m_ButtonDOX[8];
// Overrides
	// ClassWizard generate virtual function overrides
	//{{AFX_VIRTUAL(CPageDIO)
	protected:
	virtual void DoDataExchange(CDataExchange* pDX);    // DDX/DDV support
	//}}AFX_VIRTUAL

// Implementation
protected:
	void SetButtonText(CButton* pButton, BOOL bFlag);
	CButton* GetButtonDOX(int nIndex);
	CButton* GetButtonDIX(int nIndex);
	CButton* GetButtonDOY(int nIndex);
	CButton* GetButtonDIY(int nIndex);
	CButton* GetButtonDOZ(int nIndex);
	CButton* GetButtonDIZ(int nIndex);
	CButton* GetButtonDOU(int nIndex);
	CButton* GetButtonDIU(int nIndex);
	// Generated message map functions
	//{{AFX_MSG(CPageDIO)
	virtual BOOL OnInitDialog();
	afx_msg void OnStartDio();
	afx_msg void OnStopDio();
	afx_msg void OnDox0();
	afx_msg void OnDox1();
	afx_msg void OnDox2();
	afx_msg void OnDox3();
	afx_msg void OnDox4();
	afx_msg void OnDox5();
	afx_msg void OnDox6();
	afx_msg void OnDox7();
	afx_msg void OnDoy0();
	afx_msg void OnDoy1();
	afx_msg void OnDoy2();
	afx_msg void OnDoy3();
	afx_msg void OnDoy4();
	afx_msg void OnDoy5();
	afx_msg void OnDoy6();
	afx_msg void OnDoy7();
	afx_msg void OnDoz0();
	afx_msg void OnDou1();
	afx_msg void OnDou2();
	afx_msg void OnDou3();
	afx_msg void OnDou4();
	afx_msg void OnDou5();
	afx_msg void OnDou6();
	afx_msg void OnDou7();
	afx_msg void OnDoz1();
	afx_msg void OnDoz2();
	afx_msg void OnDoz3();
	afx_msg void OnDoz4();
	afx_msg void OnDoz5();
	afx_msg void OnDoz6();
	afx_msg void OnDoz7();
	afx_msg void OnDou0();
	afx_msg void OnRADIOComOutX();
	afx_msg void OnRADIOComOutY();
	afx_msg void OnRADIOStatOutX();
	afx_msg void OnRADIOStatOutY();
	afx_msg void OnRADIOComOutZ();
	afx_msg void OnRADIOStatOutZ();
	afx_msg void OnRADIOComOutU();
	afx_msg void OnRADIOStatOutU();
	//}}AFX_MSG
	DECLARE_MESSAGE_MAP()

};

//{{AFX_INSERT_LOCATION}}
// Microsoft Visual C++ will insert additional declarations immediately before the previous line.

#endif // !defined(AFX_PAGEDIO_H__72CDBA7E_5F77_486F_ACE2_1736CAAC04E2__INCLUDED_)
