// ADDoc.cpp : implementation of the CADDoc class
//

#include "stdafx.h"
#include "Sys.h"

#include "ADDoc.h"

#ifdef _DEBUG
#define new DEBUG_NEW
#undef THIS_FILE
static char THIS_FILE[] = __FILE__;
#endif

/////////////////////////////////////////////////////////////////////////////
// CADDoc

IMPLEMENT_DYNCREATE(CADDoc, CDocument)

BEGIN_MESSAGE_MAP(CADDoc, CDocument)
	//{{AFX_MSG_MAP(CADDoc)
	//}}AFX_MSG_MAP
END_MESSAGE_MAP()

/////////////////////////////////////////////////////////////////////////////
// CADDoc construction/destruction

CADDoc::CADDoc()
{
	// TODO: add one-time construction code here

}

CADDoc::~CADDoc()
{
}

BOOL CADDoc::OnNewDocument()
{
	if (!CDocument::OnNewDocument())
		return FALSE;
	// TODO: add reinitialization code here
	// (SDI documents will reuse this document)

	return TRUE;
}



/////////////////////////////////////////////////////////////////////////////
// CADDoc serialization

void CADDoc::Serialize(CArchive& ar)
{
	if (ar.IsStoring())
	{
		// TODO: add storing code here
	}
	else
	{
		// TODO: add loading code here
	}
}

/////////////////////////////////////////////////////////////////////////////
// CADDoc diagnostics

#ifdef _DEBUG
void CADDoc::AssertValid() const
{
	CDocument::AssertValid();
}

void CADDoc::Dump(CDumpContext& dc) const
{
	CDocument::Dump(dc);
}
#endif //_DEBUG

/////////////////////////////////////////////////////////////////////////////
// CADDoc commands



 

