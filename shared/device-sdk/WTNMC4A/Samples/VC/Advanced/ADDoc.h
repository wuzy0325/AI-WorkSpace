// ADDoc.h : interface of the CADDoc class
//
/////////////////////////////////////////////////////////////////////////////

#if !defined(AFX_ADDOC_H__4EED58EC_488C_498F_8476_F93E02E8EF0C__INCLUDED_)
#define AFX_ADDOC_H__4EED58EC_488C_498F_8476_F93E02E8EF0C__INCLUDED_

#if _MSC_VER > 1000
#pragma once
#endif // _MSC_VER > 1000

#include "ADView.h"
class CADDoc : public CDocument
{
protected: // create from serialization only
	CADDoc();
	DECLARE_DYNCREATE(CADDoc)

// Attributes
public:
	CADView* m_pView;
// Operations
public:

// Overrides
	// ClassWizard generated virtual function overrides
	//{{AFX_VIRTUAL(CADDoc)
	public:
	virtual BOOL OnNewDocument();
	virtual void Serialize(CArchive& ar);
	//}}AFX_VIRTUAL

// Implementation
public:
	virtual ~CADDoc();
#ifdef _DEBUG
	virtual void AssertValid() const;
	virtual void Dump(CDumpContext& dc) const;
#endif

protected:

// Generated message map functions
protected:
	//{{AFX_MSG(CADDoc)
	//}}AFX_MSG
	DECLARE_MESSAGE_MAP()
};

/////////////////////////////////////////////////////////////////////////////

//{{AFX_INSERT_LOCATION}}
// Microsoft Visual C++ will insert additional declarations immediately before the previous line.

#endif // !defined(AFX_ADDOC_H__4EED58EC_488C_498F_8476_F93E02E8EF0C__INCLUDED_)
