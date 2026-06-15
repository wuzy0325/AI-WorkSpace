// ADFrm.h : interface of the CADFrame class
//
/////////////////////////////////////////////////////////////////////////////

#if !defined(AFX_ADFRM_H__D99D7F2C_3AEC_4CEC_81A5_F0231D95D059__INCLUDED_)
#define AFX_ADFRM_H__D99D7F2C_3AEC_4CEC_81A5_F0231D95D059__INCLUDED_

#if _MSC_VER > 1000
#pragma once
#endif // _MSC_VER > 1000


class CADFrame : public CMDIChildWnd
{
	DECLARE_DYNCREATE(CADFrame)
public:
	CADFrame();

// Attributes
public:

// Operations
public:

// Overrides
	// ClassWizard generated virtual function overrides
	//{{AFX_VIRTUAL(CADFrame)
	public:
	virtual BOOL PreCreateWindow(CREATESTRUCT& cs);
	virtual void ActivateFrame(int nCmdShow = -1);
	//}}AFX_VIRTUAL

// Implementation
public:
	virtual ~CADFrame();
#ifdef _DEBUG
	virtual void AssertValid() const;
	virtual void Dump(CDumpContext& dc) const;
#endif

// Generated message map functions
protected:
	//{{AFX_MSG(CADFrame)
	afx_msg void OnClose();
	afx_msg void OnDestroy();
	//}}AFX_MSG
	DECLARE_MESSAGE_MAP()
};

/////////////////////////////////////////////////////////////////////////////

//{{AFX_INSERT_LOCATION}}
// Microsoft Visual C++ will insert additional declarations immediately before the previous line.

#endif // !defined(AFX_ADFRM_H__D99D7F2C_3AEC_4CEC_81A5_F0231D95D059__INCLUDED_)
