// ADFrm.cpp : implementation of the CADFrame class
//

#include "stdafx.h"
#include "Sys.h"

#include "ADFrm.h"
#include "ADView.h"
#ifdef _DEBUG
#define new DEBUG_NEW
#undef THIS_FILE
static char THIS_FILE[] = __FILE__;
#endif

/////////////////////////////////////////////////////////////////////////////
// CADFrame

IMPLEMENT_DYNCREATE(CADFrame, CMDIChildWnd)

BEGIN_MESSAGE_MAP(CADFrame, CMDIChildWnd)
	//{{AFX_MSG_MAP(CADFrame)
	ON_WM_CLOSE()
	ON_WM_DESTROY()
	//}}AFX_MSG_MAP
END_MESSAGE_MAP()

/////////////////////////////////////////////////////////////////////////////
// CADFrame construction/destruction

CADFrame::CADFrame()
{
	// TODO: add member initialization code here
}

CADFrame::~CADFrame()
{
}

BOOL CADFrame::PreCreateWindow(CREATESTRUCT& cs)
{
	// TODO: Modify the Window class or styles here by modifying
	//  the CREATESTRUCT cs
	if( !CMDIChildWnd::PreCreateWindow(cs) )
		return FALSE;
	return TRUE;
}



/////////////////////////////////////////////////////////////////////////////
// CADFrame diagnostics

#ifdef _DEBUG
void CADFrame::AssertValid() const
{
	CMDIChildWnd::AssertValid();
}

void CADFrame::Dump(CDumpContext& dc) const
{
	CMDIChildWnd::Dump(dc);
}

#endif //_DEBUG

/////////////////////////////////////////////////////////////////////////////
// CADFrame message handlers

void CADFrame::ActivateFrame(int nCmdShow) 
{
	// TODO: Add your specialized code here and/or call the base class
	nCmdShow = SW_SHOWMAXIMIZED;
	CMDIChildWnd::ActivateFrame(nCmdShow);
}

void CADFrame::OnClose() 
{
	// TODO: Add your message handler code here and/or call default

	CMDIChildWnd::OnClose();
}

void CADFrame::OnDestroy() 
{
	CADView* pView = (CADView*)GetActiveView();
	pView->SavePara(pView->m_DataList,       // ±£´æ²ÎÊý
					 pView->m_LCData,
					 &pView->m_LineData,
					 &pView->m_CircleData,
					 pView->m_OtherPara);
	
	CMDIChildWnd::OnDestroy();
	
	// TODO: Add your message handler code here
	
}

//DEL void CADFrame::OnUpdateStartLine(CCmdUI* pCmdUI) 
//DEL {
//DEL 	// TODO: Add your command update UI handler code here
//DEL 	CADView* pView = (CADView*)GetActiveView();
//DEL 	pCmdUI->Enable(!pView->m_bDeviceRun);
//DEL }

//DEL void CADFrame::OnStartLine() 
//DEL {
//DEL 	// TODO: Add your control notification handler code here
//DEL   	CADView* pView = (CADView*)GetActiveView();
//DEL 	pView->OnStartLine();
//DEL }

//DEL void CADFrame::OnStartLine() 
//DEL {
//DEL 	// TODO: Add your command handler code here
//DEL 
//DEL }
