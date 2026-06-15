// SynchronPageSheet.cpp : implementation file
//

#include "stdafx.h"
#include "sys.h"
#include "SynchronPageSheet.h"
#include "ADView.h"
#ifdef _DEBUG
#define new DEBUG_NEW
#undef THIS_FILE
static char THIS_FILE[] = __FILE__;
#endif
extern CADView* gl_pADView;
/////////////////////////////////////////////////////////////////////////////
// CSynchronPageSheet dialog


CSynchronPageSheet::CSynchronPageSheet(CWnd* pParent /*=NULL*/)
	: CDialog(CSynchronPageSheet::IDD, pParent)
{
	//{{AFX_DATA_INIT(CSynchronPageSheet)
		// NOTE: the ClassWizard will add member initialization here
	//}}AFX_DATA_INIT
}


void CSynchronPageSheet::DoDataExchange(CDataExchange* pDX)
{
	CDialog::DoDataExchange(pDX);
	//{{AFX_DATA_MAP(CSynchronPageSheet)
		// NOTE: the ClassWizard will add DDX and DDV calls here
	DDX_Control(pDX, IDC_TAB_Synchron, m_TabSynchron);
	//}}AFX_DATA_MAP
}


BEGIN_MESSAGE_MAP(CSynchronPageSheet, CDialog)
	//{{AFX_MSG_MAP(CSynchronPageSheet)
	ON_NOTIFY(TCN_SELCHANGE, IDC_TAB_Synchron, OnSelchangeTABSynchron)
	//}}AFX_MSG_MAP
END_MESSAGE_MAP()

/////////////////////////////////////////////////////////////////////////////
// CSynchronPageSheet message handlers

BOOL CSynchronPageSheet::OnInitDialog() 
{
	CDialog::OnInitDialog();
	
	// TODO: Add extra initialization here
	CRect rect;
	m_TabSynchron.InsertItem(0, L"X轴同步设置");
	m_TabSynchron.InsertItem(1, L"Y轴同步设置");
	m_TabSynchron.InsertItem(2, L"Z轴同步设置");
	m_TabSynchron.InsertItem(3, L"U轴同步设置");
	m_PageSynchronX.Create(IDD_Page_SynchronX, this); // X轴
	m_PageSynchronY.Create(IDD_Page_SynchronX, this); // Y轴
	m_PageSynchronZ.Create(IDD_Page_SynchronX, this); // Z轴
	m_PageSynchronU.Create(IDD_Page_SynchronX, this); // U轴
	m_PageSynchronX.SetAxisNum(WTNMC4A_XAXIS); // 设置X轴号
	m_PageSynchronY.SetAxisNum(WTNMC4A_YAXIS); // 设置Y轴号
	m_PageSynchronZ.SetAxisNum(WTNMC4A_ZAXIS); // 设置Z轴号
	m_PageSynchronU.SetAxisNum(WTNMC4A_UAXIS); // 设置U轴号
	m_TabSynchron.GetWindowRect(rect);
	ScreenToClient(rect);
	rect.DeflateRect(2, 4);
	rect.top += 16;
	m_PageSynchronX.MoveWindow(rect);
	m_PageSynchronY.MoveWindow(rect);
	m_PageSynchronZ.MoveWindow(rect);
	m_PageSynchronU.MoveWindow(rect);
	m_PageSynchronX.ShowWindow(SW_SHOW);
	return TRUE;  // return TRUE unless you set the focus to a control
	              // EXCEPTION: OCX Property Pages should return FALSE
}

void CSynchronPageSheet::OnSelchangeTABSynchron(NMHDR* pNMHDR, LRESULT* pResult) 
{
	// TODO: Add your control notification handler code here
	switch(m_TabSynchron.GetCurSel())
	{
	case 0:
		m_PageSynchronX.ShowWindow(SW_SHOW);
		m_PageSynchronY.ShowWindow(SW_HIDE);
		m_PageSynchronZ.ShowWindow(SW_HIDE);
		m_PageSynchronU.ShowWindow(SW_HIDE);
		gl_pADView->m_PageComSet.SetCurrentAxisNum(WTNMC4A_XAXIS);
		break;
	case 1:
		m_PageSynchronX.ShowWindow(SW_HIDE);
		m_PageSynchronY.ShowWindow(SW_SHOW);
		m_PageSynchronZ.ShowWindow(SW_HIDE);
		m_PageSynchronU.ShowWindow(SW_HIDE);
		gl_pADView->m_PageComSet.SetCurrentAxisNum(WTNMC4A_YAXIS);
		break;
	case 2:
		m_PageSynchronX.ShowWindow(SW_HIDE);
		m_PageSynchronY.ShowWindow(SW_HIDE);
		m_PageSynchronZ.ShowWindow(SW_SHOW);
		m_PageSynchronU.ShowWindow(SW_HIDE);
		gl_pADView->m_PageComSet.SetCurrentAxisNum(WTNMC4A_ZAXIS);
		break;
	case 3:
		m_PageSynchronX.ShowWindow(SW_HIDE);
		m_PageSynchronY.ShowWindow(SW_HIDE);
		m_PageSynchronZ.ShowWindow(SW_HIDE);
		m_PageSynchronU.ShowWindow(SW_SHOW);
		gl_pADView->m_PageComSet.SetCurrentAxisNum(WTNMC4A_UAXIS);
		break;
	default:
		break;
	}	
	*pResult = 0;
}
