// ListSegmentInfo.cpp : 实现文件
//

#include "stdafx.h"

#include "Sys.h"
#include "MainFrm.h"
#include "ChildFrm.h"
#include "ListSegmentInfo.h"

#include "SysView.h"
#include ".\listsegmentinfo.h"

// CListSegmentInfo

IMPLEMENT_DYNAMIC(CListSegmentInfo, CListCtrl)
CListSegmentInfo::CListSegmentInfo()
{
	m_iCurColum = -1;
	m_iCurRow = -1;
	m_pParentFrm= NULL;
	m_iSelItem = -1;
	m_iLength = 1024;
	m_iLoopCount = 1;
	m_iWaveType = 0;
}

CListSegmentInfo::~CListSegmentInfo()
{
}

void CListSegmentInfo::DoDataExchange(CDataExchange* pDX)
{
	CWnd::DoDataExchange(pDX);
	DDX_Text(pDX, EDIT_ID, m_iNum);	 
}

BEGIN_MESSAGE_MAP(CListSegmentInfo, CListCtrl)
	ON_WM_CREATE()
	ON_WM_LBUTTONDOWN()	
	ON_EN_CHANGE(EDIT_ID, OnEnChanngeEdit)	
	ON_WM_VSCROLL()
	ON_WM_HSCROLL()
	ON_WM_MOUSEWHEEL()
	ON_BN_CLICKED(BTN_ID, OnBtnClickedWave)
	ON_WM_RBUTTONUP()
	ON_WM_RBUTTONDOWN()
	ON_COMMAND(IDM_RB_DEL, OnItemDel)
	ON_WM_KEYDOWN()
	ON_WM_LBUTTONUP()	
	ON_BN_KILLFOCUS(BTN_ID, OnBnKillfocusWave)
END_MESSAGE_MAP()

// CListSegmentInfo 消息处理程序
void CListSegmentInfo::PreSubclassWindow()
{
	// TODO: 在此添加专用代码和/或调用基类
	this->ModifyStyle(0, LVS_REPORT|LVS_SINGLESEL);
	this->SetExtendedStyle(LVS_EX_FULLROWSELECT|LVS_EX_GRIDLINES);

	// TODO:  在此添加您专用的创建代码
	CListCtrl::PreSubclassWindow();
}

int CListSegmentInfo::OnCreate(LPCREATESTRUCT lpCreateStruct)
{
	if (CListCtrl::OnCreate(lpCreateStruct) == -1)
		return -1;
	
	return 0;
}

BOOL CListSegmentInfo::PreCreateWindow(CREATESTRUCT& cs)
{
	// TODO: 在此添加专用代码和/或调用基类

	return CListCtrl::PreCreateWindow(cs);
}

BOOL CListSegmentInfo::OnCreateAggregates()
{
	// TODO: 在此添加专用代码和/或调用基类

	return TRUE;
}

BOOL CListSegmentInfo::Create(DWORD dwStyle, const RECT& rect, CWnd* pParentWnd, UINT nID)
{
	// TODO: 在此添加专用代码和/或调用基类
	return CListCtrl::Create(dwStyle, rect, pParentWnd, nID);

}

BOOL CListSegmentInfo::InitListView(void)
{
	if (this->GetSafeHwnd())
	{
		this->InsertColumn(0, "序号", LVCFMT_LEFT, 60);
		this->InsertColumn(1, "循环", LVCFMT_LEFT, 60);
		this->InsertColumn(2, "段长度", LVCFMT_LEFT, 60);
		this->InsertColumn(3, "波形", LVCFMT_LEFT, 60);

		m_ctrEdit.Create(WS_CHILD|ES_CENTER|ES_WANTRETURN|WS_TABSTOP, m_rcEdit, this, EDIT_ID);
		m_ctrEdit.SetWindowPos(&CWnd::wndBottom,
							0, 0, 0, 0,
							SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE );
		
		m_btnWave.Create("波形", WS_CHILD|WS_TABSTOP|BS_NOTIFY , m_rcBtn, this, BTN_ID);

		this->InsertItem(0, "添加");  // 添加第一行
		return TRUE;
	}
	else
		return FALSE;
	// TODO:  在此添加您专用的创建代码
}

// 鼠标左键 下按消息
void CListSegmentInfo::OnLButtonDown(UINT nFlags, CPoint point)
{	
	LVHITTESTINFO lvhti;	
	lvhti.pt = point;
	this->SubItemHitTest(&lvhti);  // 得到下按信息	

	if (lvhti.flags & LVHT_ONITEM )
	{
		m_iCurColum = lvhti.iSubItem;
		m_iCurRow = lvhti.iItem;
		int iOldSelItem = m_iSelItem;  // 保存旧的选中项
		m_iSelItem = lvhti.iItem;  // 选中的项 段的序号

		CChannel* pChannel = &(m_pParentFrm->m_Channel);
		ASSERT(pChannel != NULL);

		if (m_iCurRow == this->GetItemCount() - 1)  
		{	// 安下最后一项 是添加
			// 请求空间分配			
			if(TRUE == m_pParentFrm->AllocSpace(m_iLength + SEGMENT_INFO_SIZE))
			{
				int iItemCount = this->GetItemCount();
				this->InsertItem(iItemCount, "添加");  // 添加第一行

				CString strIndex;
				strIndex.Format("%d", iItemCount - 1);	
				this->SetItemText(iItemCount - 1, 0, strIndex);

				// 数据点循环次数
				strIndex.Format("%d", m_iLoopCount);
				this->SetItemText(iItemCount - 1, 1, strIndex);
				// 数据点长度
				strIndex.Format("%d", m_iLength);
				this->SetItemText(iItemCount - 1, 2, strIndex);

				// 为添加一格数据段
				CChannel::CSegment* pTmpSegment = new CChannel::CSegment;
				pTmpSegment->m_iLoopCount = m_iLoopCount;
				pTmpSegment->m_WaveInfo.iSizeWords = m_iLength;
				pTmpSegment->m_WaveInfo.pDataBuff = new long[m_iLength];


				if (iOldSelItem != -1 && iOldSelItem < pChannel->m_arraySegment.GetSize())
				{					
					CChannel::CSegment* pLastSelSegment = pChannel->m_arraySegment[iOldSelItem];
					pTmpSegment->m_WaveInfo.enumWaveType = pLastSelSegment->m_WaveInfo.enumWaveType;
					pTmpSegment->m_WaveInfo.iDataBits = pLastSelSegment->m_WaveInfo.iDataBits;
					pTmpSegment->m_WaveInfo.iCycleCount = pLastSelSegment->m_WaveInfo.iCycleCount;
					pTmpSegment->m_WaveInfo.fAmplitude = pLastSelSegment->m_WaveInfo.fAmplitude;
					pTmpSegment->m_WaveInfo.fOffset = pLastSelSegment->m_WaveInfo.fOffset;
					pTmpSegment->m_WaveInfo.fPhase = pLastSelSegment->m_WaveInfo.fPhase;
					pTmpSegment->m_WaveInfo.fDuty = pLastSelSegment->m_WaveInfo.fDuty;
					
					memcpy(pTmpSegment->m_WaveInfo.pDataBuff, pLastSelSegment->m_WaveInfo.pDataBuff, 
										sizeof(long) * pLastSelSegment->m_WaveInfo.iSizeWords);

					switch(pTmpSegment->m_WaveInfo.enumWaveType)
					{
						// 	Sine = 0,		// 正弦
						// 	Triangle = 1,	// 三角
						// 	Square,			// 方波
						// 	SawTooth, 		// 锯齿波
						// 	WhiteNoise,		// 白噪声
						// 	DC,				// 直线波
						// 	USER			// 用户自定义
					case Sine:
						this->SetItemText(m_iSelItem, 3, "正弦波");
						break;
					case Triangle:
						this->SetItemText(m_iSelItem, 3, "三角波");
						break;
					case Square:
						this->SetItemText(m_iSelItem, 3, "方波");
					    break;
					case SawTooth:
						this->SetItemText(m_iSelItem, 3, "锯齿波");
					    break;
					case WhiteNoise:
						this->SetItemText(m_iSelItem, 3, "白燥波");
						break;
					case DC:
						this->SetItemText(m_iSelItem, 3, "DC");
						break;					
					default:
						this->SetItemText(m_iSelItem, 3, "用户定义");
					    break;
					}					
				}
				else
				{
					pTmpSegment->m_WaveInfo.enumWaveType = Sine;  // 正弦波					
					pTmpSegment->m_WaveInfo.iDataBits = 12;
					pTmpSegment->m_WaveInfo.iCycleCount = 1;
					pTmpSegment->m_WaveInfo.fAmplitude = 1.0f;
					pTmpSegment->m_WaveInfo.fOffset = 0.0f;
					pTmpSegment->m_WaveInfo.fPhase = 0.0f;
					
					this->SetItemText(m_iSelItem, 3, "正弦波");								
					CreateSine(pTmpSegment->m_WaveInfo.pDataBuff, pTmpSegment->m_WaveInfo.iSizeWords,
								pTmpSegment->m_WaveInfo.iCycleCount, pTmpSegment->m_WaveInfo.fAmplitude,
								pTmpSegment->m_WaveInfo.fOffset, pTmpSegment->m_WaveInfo.fPhase, pTmpSegment->m_WaveInfo.iDataBits );
			}				
				pChannel->m_arraySegment.Add(pTmpSegment);	
				m_pParentFrm->m_pChCfgView->GetDocument()->SetModifiedFlag();
				//	m_pParentFrm->m_pChCfgView->GetDlgItem(IDC_BUTTON_Begin)->EnableWindow(FALSE);
				m_pParentFrm->m_pChCfgView->m_bInitDevice = FALSE;

				m_pParentFrm->UpdataSegmentViewScrSizes();
				m_pParentFrm->m_pSegmentView->ShowSegment(lvhti.iItem);  // 显示制定段
				Scroll(CSize(0, 30));  // 向下滚动

				// 显示波形按钮
				m_iCurColum = 3;
				this->GetSubItemRect(lvhti.iItem, m_iCurColum, LVIR_LABEL, m_rcBtn);
				m_btnWave.MoveWindow(m_rcBtn, TRUE);
				m_btnWave.ShowWindow(SW_SHOW);
				m_ctrEdit.ShowWindow(SW_HIDE);
			}		
			else
			{
				// 提示空间不足 要求分配空间
				m_btnWave.ShowWindow(SW_HIDE);
				m_ctrEdit.ShowWindow(SW_HIDE);
				m_iSelItem = iOldSelItem;  // 恢复就的选项

				CString strWar;
				strWar.LoadString(IDS_NO_RAM);				

				this->MessageBox(strWar, "警告", MB_OK|MB_ICONEXCLAMATION| MB_ICONWARNING);				
			}
		}
		else
		{  // 选中已有一项				
			m_pParentFrm->m_pSegmentView->ShowSegment(m_iSelItem);  // 显示选中段
			switch(m_iCurColum)
			{
			case 1:  // 段的循环次数
				{
					m_btnWave.ShowWindow(SW_HIDE);	

					CString strLoopCount = this->GetItemText(m_iCurRow, m_iCurColum);
					m_iNum = ::strtol(strLoopCount, NULL, 10);  // 转化为数字				
					// 设置Edit 的值
					this->UpdateData(FALSE);	

					this->GetSubItemRect(m_iCurRow, m_iCurColum, LVIR_LABEL, m_rcEdit);
					m_ctrEdit.MoveWindow(m_rcEdit);				 
					m_ctrEdit.ShowWindow(SW_SHOW);
				}
				break;

			case 2:  // 段的长度
				{
					m_btnWave.ShowWindow(SW_HIDE);			

					CString strLoopCount = this->GetItemText(m_iCurRow, m_iCurColum);
					m_iNum = ::strtol(strLoopCount, NULL, 10);

					this->UpdateData(FALSE);

					this->GetSubItemRect(m_iCurRow, m_iCurColum, LVIR_LABEL, m_rcEdit);				
					m_ctrEdit.MoveWindow(m_rcEdit);				 
					m_ctrEdit.ShowWindow(SW_SHOW);
				}
				break;	

			default:
				m_ctrEdit.ShowWindow(SW_HIDE);   // 隐藏Edit控件

				this->GetSubItemRect(m_iCurRow, 3, LVIR_LABEL, m_rcBtn);
				m_btnWave.MoveWindow(m_rcBtn, TRUE);
				m_btnWave.ShowWindow(SW_SHOW);	// 显示按钮						
				break;
			}				
		}

		this->Invalidate();		
	}
	else
	{  // 点中其他位置时 隐藏 edit btn ctrl
		m_btnWave.ShowWindow(SW_HIDE);
		m_ctrEdit.ShowWindow(SW_HIDE);
	}

	CListCtrl::OnLButtonDown(nFlags, point);	

	// 使窗体获得焦点	
	if (m_ctrEdit.IsWindowVisible())
	{
		m_ctrEdit.SetFocus();     // edit 得到焦点
		m_ctrEdit.SetSel(MAKELONG(0, -1));		// 全部选中文本
	}	
}

void CListSegmentInfo::OnEnChanngeEdit()
{
	int iOldNum = m_iNum;
	if (FALSE == this->UpdateData(TRUE))
	{
		this->UpdateData(FALSE);
		return;
	}
	// 察看是否有效
	ASSERT(m_iSelItem < m_pParentFrm->m_Channel.m_arraySegment.GetSize());
	CChannel::CSegment* pSegment = m_pParentFrm->m_Channel.m_arraySegment[m_iSelItem];
	if (m_iNum > 0)
	{ // 合理
		switch(m_iCurColum)
		{
		case 1:  // 循环次数发生变化
			m_iLoopCount = m_iNum;  // 保存循环次数
			pSegment->m_iLoopCount = m_iLoopCount;
			m_pParentFrm->m_pChCfgView->GetDocument()->SetModifiedFlag();
			//	m_pParentFrm->m_pChCfgView->GetDlgItem(IDC_BUTTON_Begin)->EnableWindow(FALSE);
			m_pParentFrm->m_pChCfgView->m_bInitDevice = FALSE;
			break;
		case 2:  // 数据长度改变了
			if(m_iNum != pSegment->m_WaveInfo.iSizeWords)
			{   // 数据增加
				m_pParentFrm->m_pChCfgView->GetDocument()->SetModifiedFlag();
				//	m_pParentFrm->m_pChCfgView->GetDlgItem(IDC_BUTTON_Begin)->EnableWindow(FALSE);
				m_pParentFrm->m_pChCfgView->m_bInitDevice = FALSE;
				if (m_iNum > pSegment->m_WaveInfo.iSizeWords)
				{
					if(TRUE == m_pParentFrm->AllocSpace(m_iNum - pSegment->m_WaveInfo.iSizeWords))
					{

						PLONG pData = new long[m_iNum];
						memset(pData, 0, m_iNum * sizeof(long));

						memcpy(pData, pSegment->m_WaveInfo.pDataBuff,sizeof(LONG) * pSegment->m_WaveInfo.iSizeWords);	
						m_iLength = m_iNum;
						pSegment->m_WaveInfo.iSizeWords = m_iNum;
						delete [] pSegment->m_WaveInfo.pDataBuff;
						pSegment->m_WaveInfo.pDataBuff = pData;

						m_pParentFrm->UpdataSegmentViewScrSizes();
						m_pParentFrm->m_pSegmentView->ShowSegment(m_iCurRow);  // 显示制定段

					}
					else
					{
						m_iNum = iOldNum;
						UpdateData(FALSE);
						CString strWar;
						strWar.LoadString(IDS_NO_RAM);
						this->MessageBox(strWar, "警告", 
											MB_OK|MB_ICONEXCLAMATION| MB_ICONWARNING);				
					}
				} 
				else
				{
					// 数据减小
					PLONG pData = new long[m_iNum];
					memset(pData, 0, m_iNum * sizeof(long));
					int iMin = (m_iNum < pSegment->m_WaveInfo.iSizeWords)? m_iNum: pSegment->m_WaveInfo.iSizeWords;
					memcpy(pData, pSegment->m_WaveInfo.pDataBuff,sizeof(LONG) * iMin);				

					m_iLength = m_iNum;
					pSegment->m_WaveInfo.iSizeWords = m_iNum;
					delete [] pSegment->m_WaveInfo.pDataBuff;
					pSegment->m_WaveInfo.pDataBuff = pData;
					m_pParentFrm->UpdataSegmentViewScrSizes();
					m_pParentFrm->m_pSegmentView->ShowSegment(m_iCurRow);  // 显示制定段
				}                
			}			
			break;
		default:
			ASSERT(FALSE);
			break;
		}

		CString strTxt;
		GetDlgItemText(EDIT_ID, strTxt);
		this->SetItemText(m_iCurRow, m_iCurColum, strTxt);	
	} 
	else
	{ // 不合理
		m_iNum = iOldNum;
		this->UpdateData(FALSE); // 回填数据
	}		
}

// 没有添加滑轮事件处理函数
void CListSegmentInfo::OnVScroll(UINT nSBCode, UINT nPos, CScrollBar* pScrollBar)
{
	m_ctrEdit.ShowWindow(SW_HIDE);
	m_btnWave.ShowWindow(SW_HIDE);

	CListCtrl::OnVScroll(nSBCode, nPos, pScrollBar);	
}

void CListSegmentInfo::OnHScroll(UINT nSBCode, UINT nPos, CScrollBar* pScrollBar)
{
	// TODO: 在此添加消息处理程序代码和/或调用默认值
	m_ctrEdit.ShowWindow(SW_HIDE);
	m_btnWave.ShowWindow(SW_HIDE);
	CListCtrl::OnHScroll(nSBCode, nPos, pScrollBar);	
}

// 只更新可见区域
BOOL CListSegmentInfo::OnMouseWheel(UINT nFlags, short zDelta, CPoint pt)
{
	m_ctrEdit.ShowWindow(SW_HIDE);
	m_btnWave.ShowWindow(SW_HIDE);

	BOOL bRet =  CListCtrl::OnMouseWheel(nFlags, zDelta, pt);	
	return bRet;
}


void CListSegmentInfo::OnBnKillfocusWave()
{
}

void CListSegmentInfo::OnBtnClickedWave()
{
	ASSERT(m_iCurRow >= 0 && m_pParentFrm->m_Channel.m_arraySegment.GetSize());

	CChannel::CSegment* pSegment = m_pParentFrm->m_Channel.m_arraySegment[m_iCurRow];


	// 设置一个可用空间
	WAVE_INFO* pWave = 	CreateWaveDlg(&(pSegment->m_WaveInfo), this);
	if (NULL != pWave)
	{
		pSegment->m_WaveInfo.iSizeWords = 0;
		delete [] pSegment->m_WaveInfo.pDataBuff;

		pSegment->m_WaveInfo = *pWave;

		pSegment->m_WaveInfo.pDataBuff = NULL;
		pSegment->m_WaveInfo.pDataBuff = new LONG[pWave->iSizeWords];
		memcpy(pSegment->m_WaveInfo.pDataBuff, pWave->pDataBuff,  sizeof(LONG) * pWave->iSizeWords);

		pSegment->m_WaveInfo.iSizeWords =  pWave->iSizeWords;
	}
	ReleaseWave(pWave);

	m_iWaveType = pSegment->m_WaveInfo.enumWaveType; // 更新波形信息
	// 更新长度
	m_iLength = pSegment->m_WaveInfo.iSizeWords;
	CString str;
	str.Format("%d", m_iLength);
	this->SetItemText(m_iCurRow, 2, str);
	switch(m_iWaveType)
	{
	case Sine:
		this->SetItemText(m_iCurRow, 3, "正弦波");
		break;
	case Triangle:
		this->SetItemText(m_iCurRow, 3, "三角波");
		break;
	case 2:
		this->SetItemText(m_iCurRow, 3, "方波");
		break;
	case 3:
		this->SetItemText(m_iCurRow, 3, "锯齿波");
		break;
	case 4:
		this->SetItemText(m_iCurRow, 3, "白燥波");
		break;
	case DC:
		this->SetItemText(m_iCurRow, 3, "DC");
		break;					
	default:
		this->SetItemText(m_iCurRow, 3, "用户定义");
		break;
	}		
	m_pParentFrm->m_pChCfgView->GetDocument()->SetModifiedFlag();
	//	m_pParentFrm->m_pChCfgView->GetDlgItem(IDC_BUTTON_Begin)->EnableWindow(FALSE);
	m_pParentFrm->m_pChCfgView->m_bInitDevice = FALSE;
	m_pParentFrm->UpdataSegmentViewScrSizes();
}

void CListSegmentInfo::OnRButtonUp(UINT nFlags, CPoint point)
{
	// TODO: 在此添加消息处理程序代码和/或调用默认值
	// 添加删除菜单
	

	CListCtrl::OnRButtonUp(nFlags, point);
}

void CListSegmentInfo::OnRButtonDown(UINT nFlags, CPoint point)
{
	// TODO: 在此添加消息处理程序代码和/或调用默认值
	CListCtrl::OnRButtonDown(nFlags, point);

	LVHITTESTINFO lvhti;	
	lvhti.pt = point;
	this->SubItemHitTest(&lvhti);
	if (lvhti.flags & LVHT_ONITEM )
	{
		m_iSelItem = lvhti.iItem;  // 判断合理性 不能使最后一个  最后一个是添加 ????
		CMenu menuRBtn;  // 右键菜单
	
		VERIFY(menuRBtn.LoadMenu(IDR_MENU_RB));  //　载入菜单资源
	
		if(menuRBtn.m_hMenu != NULL)
		{
			CMenu* pPopup = menuRBtn.GetSubMenu(0);
			ASSERT(pPopup != NULL);

			CPoint pt = point;
			this->ClientToScreen(&pt);

			pPopup->TrackPopupMenu(TPM_LEFTALIGN |TPM_RIGHTBUTTON, pt.x, pt.y, this);
		}
	}
}

// 删除选中了段数据
void CListSegmentInfo::OnItemDel()
{
	ASSERT(m_iSelItem != -1);	
	ASSERT(m_iSelItem != (this->GetItemCount() - 1)); // 不能删除最后一条添加
	this->DeleteItem(m_iSelItem);

	// 调用析构函数
	delete m_pParentFrm->m_Channel.m_arraySegment[m_iSelItem];

	m_pParentFrm->m_Channel.m_arraySegment.RemoveAt(m_iSelItem);

	//m_pParentFrm->m_pChCfgView->GetDlgItem(IDC_BUTTON_Begin)->EnableWindow(FALSE);
	m_pParentFrm->m_pChCfgView->m_bInitDevice = FALSE;
	m_pParentFrm->m_pChCfgView->GetDocument()->SetModifiedFlag();	
	m_pParentFrm->UpdataSegmentViewScrSizes();	

	CString str;
	INT_PTR iSegmentCount = m_pParentFrm->m_Channel.m_arraySegment.GetSize();
	for (int index = m_iSelItem; index < iSegmentCount; index++)
	{
		str.Format("%d", index);
		this->SetItemText(index, 0, str);
	}
	m_iSelItem = -1;
	m_btnWave.ShowWindow(SW_HIDE);  // 隐藏按钮
}

void CListSegmentInfo::OnKeyDown(UINT nChar, UINT nRepCnt, UINT nFlags)
{
	switch(nChar)
	{
	case VK_DELETE:
		{	
			int iOldSelItem = m_iSelItem;
			if(m_iSelItem != -1)
			{
				OnItemDel();
				m_iSelItem = iOldSelItem;
				if(m_iSelItem == m_pParentFrm->m_Channel.GetSigmentCount())
					m_iSelItem -= 1;			
			}

			// 选中
			if(m_iSelItem != -1)
			{
				this->SetItemState(m_iSelItem, LVIS_SELECTED, LVIS_SELECTED);
			}
		}
		break;
	default:
	    break;
	}

	CListCtrl::OnKeyDown(nChar, nRepCnt, nFlags);
}

void CListSegmentInfo::OnLButtonUp(UINT nFlags, CPoint point)
{	
	CListCtrl::OnLButtonUp(nFlags, point);	
}

