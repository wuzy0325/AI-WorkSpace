// ListStateView.cpp : 实现文件
//

#include "stdafx.h"
#include "Sys.h"
#include "ListStateView.h"
#include ".\liststateview.h"
#include "ChildFrm.h"
#include "SysView.h"

// CListStateView

IMPLEMENT_DYNCREATE(CListStateView, CListView)

CListStateView::CListStateView():
m_nTimerId(800)
{
	memset(&m_DAStatus, 0, sizeof(PCI8603_STATUS_DA));
	m_iElapse = 500;
	m_pParentFrm = NULL;
	m_nReadTimes = 0;
}

CListStateView::~CListStateView()
{

}

BEGIN_MESSAGE_MAP(CListStateView, CListView)
	ON_WM_TIMER()
END_MESSAGE_MAP()

void CListStateView::StartUpataDAStatus()      // 开始更新DA的状态
{
	GetListCtrl().DeleteAllItems();	
	m_nReadTimes = 0;
    this->SetTimer(m_nTimerId, m_iElapse, NULL);
}

void CListStateView::StopUpataDAStatus()       // 停止更新DA的状态
{
	this->KillTimer(m_nTimerId);	
	m_pParentFrm->m_pChCfgView->m_ListCtrlSegmentInfo.SetItemState(-1, ~LVIS_DROPHILITED, LVIS_DROPHILITED);
}

// CListStateView 诊断

#ifdef _DEBUG
void CListStateView::AssertValid() const
{
	CListView::AssertValid();
}

void CListStateView::Dump(CDumpContext& dc) const
{
	CListView::Dump(dc);
}
#endif //_DEBUG


// CListStateView 消息处理程序

void CListStateView::OnInitialUpdate()
{
	CListView::OnInitialUpdate();	
	// TODO: 在此添加专用代码和/或调用基类
	m_pParentFrm = STATIC_DOWNCAST(CChildFrame, GetParentFrame());	


	GetListCtrl().ModifyStyle(0, LVS_REPORT);
	DWORD ExStyle = GetListCtrl().GetExtendedStyle()| LVS_EX_GRIDLINES|LVS_EX_FULLROWSELECT|LVS_EX_HEADERDRAGDROP ; 

	this->GetListCtrl().SetExtendedStyle(ExStyle);

	this->GetListCtrl().InsertColumn(0, "序号", LVCFMT_CENTER, 50);
	this->GetListCtrl().InsertColumn(1, "总循环次数", LVCFMT_CENTER, 100);
	this->GetListCtrl().InsertColumn(2, "段号", LVCFMT_CENTER, 50);
	this->GetListCtrl().InsertColumn(3, "段循环次数", LVCFMT_CENTER, 100);	
	this->GetListCtrl().InsertColumn(4, "段地址", LVCFMT_CENTER, 100);
	this->GetListCtrl().InsertColumn(5, "使能标志", LVCFMT_CENTER, 100);
	this->GetListCtrl().InsertColumn(6, "转换标志", LVCFMT_CENTER, 100);
	this->GetListCtrl().InsertColumn(7, "触发标志", LVCFMT_CENTER, 100);
	this->GetListCtrl().InsertColumn(8, "时间", LVCFMT_CENTER, 100);
}

void CListStateView::OnTimer(UINT nIDEvent)
{	
	if (m_nTimerId == nIDEvent)
	{   // 更新DA状态列表
		ASSERT(theApp.m_hDevice);
		ASSERT_KINDOF(CChildFrame,  m_pParentFrm);
		PCI8603_GetDevStatusDA(theApp.m_hDevice, &m_DAStatus, 
						m_pParentFrm->m_Channel.m_iIndex);

		GetListCtrl().SetRedraw(FALSE);
		//// 添加信息相
		int iItemCount = this->GetListCtrl().GetItemCount();
		CString strTmp;
		strTmp.Format("%d", iItemCount + m_nReadTimes);
		
		this->GetListCtrl().InsertItem(iItemCount, strTmp);

		strTmp.Format("%d", m_DAStatus.nCurSegNum);         // 可读段号
		// 在波形里显示当前段
		m_pParentFrm->m_pSegmentView->ShowSegment(m_DAStatus.nCurSegNum);
		m_pParentFrm->m_pChCfgView->m_ListCtrlSegmentInfo.SetItemState(-1, ~LVIS_DROPHILITED, LVIS_DROPHILITED);		
		m_pParentFrm->m_pChCfgView->m_ListCtrlSegmentInfo.SetItemState(m_DAStatus.nCurSegNum, LVIS_DROPHILITED, LVIS_DROPHILITED);
		m_pParentFrm->m_pChCfgView->m_ListCtrlSegmentInfo.EnsureVisible(m_DAStatus.nCurSegNum, FALSE);

		strTmp.Format("%d", m_DAStatus.nCurLoopCount);		// 总的循环次数
		this->GetListCtrl().SetItemText(iItemCount, 1, strTmp);

		strTmp.Format("%d", m_DAStatus.nCurSegNum);		// 总的循环次数
		this->GetListCtrl().SetItemText(iItemCount, 2, strTmp);
	
		strTmp.Format("%d", m_DAStatus.nCurSegLoopCount);	// 段的循环次数
		this->GetListCtrl().SetItemText(iItemCount, 3, strTmp);	
	
		strTmp.Format("%d", m_DAStatus.nCurSegAddr );		// 段地址
		this->GetListCtrl().SetItemText(iItemCount, 4, strTmp);

		strTmp.Format("%d", m_DAStatus.bEnable);		// 使能标志
		this->GetListCtrl().SetItemText(iItemCount, 5, strTmp);


		strTmp.Format("%d", m_DAStatus.bConverting );		// DA转换标志
		this->GetListCtrl().SetItemText(iItemCount, 6, strTmp);

		strTmp.Format("%d", m_DAStatus.bTrigFlag );			// 触发标志
		this->GetListCtrl().SetItemText(iItemCount, 7, strTmp);

		CTime tm;
		tm = CTime::GetCurrentTime();
		this->GetListCtrl().SetItemText(iItemCount, 8, tm.Format("%H:%M:%S") );	
		
		GetListCtrl().SetItemState(-1, ~LVIS_DROPHILITED, LVIS_DROPHILITED);	
		
		GetListCtrl().SetItemState(iItemCount, LVIS_DROPHILITED, LVIS_DROPHILITED);

		if (iItemCount > 1024)
		{
			m_nReadTimes++;
			GetListCtrl().DeleteItem(0);
		}		
		GetListCtrl().EnsureVisible(iItemCount,  FALSE);  // 使该项显示
		GetListCtrl().SetRedraw(TRUE);	  // 更新窗体	

	}
	else
		CListView::OnTimer(nIDEvent);
}
