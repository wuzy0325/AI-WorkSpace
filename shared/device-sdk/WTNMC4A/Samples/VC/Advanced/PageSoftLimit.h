#if !defined(AFX_PAGESOFTLIMIT_H__8FAD6824_61AE_41DE_82D2_D7DA05883CA3__INCLUDED_)
#define AFX_PAGESOFTLIMIT_H__8FAD6824_61AE_41DE_82D2_D7DA05883CA3__INCLUDED_

#if _MSC_VER > 1000
#pragma once
#endif // _MSC_VER > 1000
// PageSoftLimit.h : header file
//

/////////////////////////////////////////////////////////////////////////////
// CPageSoftLimit dialog

class CPageSoftLimit : public CPropertyPage
{
	DECLARE_DYNCREATE(CPageSoftLimit)

// Construction
public:
	bool m_bSetSoftLimit;
	void SetCurrentAxisNum(int nCurrentAxis);
	CPageSoftLimit();
	~CPageSoftLimit();

// Dialog Data
	//{{AFX_DATA(CPageSoftLimit)
	enum { IDD = IDD_Page_SoftLimit };
	CButton	m_Button_Slimit;
	CButton	m_Button_ClearLimit;
	CEdit	m_Edit_UpperLimit;
	CEdit	m_Edit_LowerLimit;
	//}}AFX_DATA

	int m_nCurrentAxis;
// Overrides
	// ClassWizard generate virtual function overrides
	//{{AFX_VIRTUAL(CPageSoftLimit)
	protected:
	virtual void DoDataExchange(CDataExchange* pDX);    // DDX/DDV support
	//}}AFX_VIRTUAL

// Implementation
protected:
	// Generated message map functions
	//{{AFX_MSG(CPageSoftLimit)
	virtual BOOL OnInitDialog();
	afx_msg void OnSetSlimit();
	afx_msg void OnClearlimit();
	afx_msg void OnRadioLogic();
	afx_msg void OnRadioFact();
	afx_msg void OnChangeEditUpderlimit();
	afx_msg void OnChangeEditLowerlimit();
	//}}AFX_MSG
	DECLARE_MESSAGE_MAP()

};

//{{AFX_INSERT_LOCATION}}
// Microsoft Visual C++ will insert additional declarations immediately before the previous line.

#endif // !defined(AFX_PAGESOFTLIMIT_H__8FAD6824_61AE_41DE_82D2_D7DA05883CA3__INCLUDED_)
