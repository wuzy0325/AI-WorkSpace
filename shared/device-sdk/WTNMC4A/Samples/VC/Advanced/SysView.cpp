// SysView.cpp : CSysView 类的实现
//

#include "stdafx.h"
#include "Sys.h"
#include "SysDoc.h"
#include "SysView.h"
#include ".\sysview.h"
#include "ChildFrm.h"
#include "MainFrm.h"

#ifdef _DEBUG
#define new DEBUG_NEW
#endif


// CSysView

IMPLEMENT_DYNCREATE(CSysView, CScrollView)

BEGIN_MESSAGE_MAP(CSysView, CScrollView)
	// 标准打印命令
	ON_COMMAND(ID_FILE_PRINT, CView::OnFilePrint)
	ON_COMMAND(ID_FILE_PRINT_DIRECT, CView::OnFilePrint)
	ON_COMMAND(ID_FILE_PRINT_PREVIEW, CView::OnFilePrintPreview)
	ON_WM_ERASEBKGND()
	ON_WM_SIZE()
	ON_WM_HSCROLL()
	ON_WM_VSCROLL()
	ON_WM_LBUTTONDOWN()
	ON_WM_LBUTTONDBLCLK()
	ON_WM_DESTROY()
	ON_WM_CONTEXTMENU()
	ON_COMMAND(IDM_RELOAD_DATA, OnReloadData)
	ON_UPDATE_COMMAND_UI(IDM_RELOAD_DATA, OnUpdateReloadData)
	ON_COMMAND(IDM_SHOW_ALL_WAVE, OnShowAllWave)
	ON_UPDATE_COMMAND_UI(IDM_SHOW_ALL_WAVE, OnUpdateShowAllWave)
	ON_WM_MOUSEMOVE()
END_MESSAGE_MAP()

// CSysView 构造/析构

CSysView::CSysView()
{
	// TODO: 在此处添加构造代码
	m_pOldBmp = NULL;
	m_iShowMode = WAVE_MODE;
	m_pDataBuf = NULL;
	m_pPtBuf = NULL;
	m_rcSpace.SetRect(50, 50, 20, 20);  // 四边空隙
	m_oldSZ.cx = 0;
	m_oldSZ.cx = 0;
	m_ptLBDown.x = 0;
	m_ptLBDown.y = 0;
	m_bShowInfo = FALSE;
}

CSysView::~CSysView()
{
	if (m_pOldBmp != NULL)
	{
		m_dcMem.SelectObject(m_pOldBmp);
		m_pOldBmp = NULL;
	}

	if (NULL != m_pDataBuf)
	{
		delete []m_pDataBuf;
		m_pDataBuf = NULL;
	}

	if (NULL != m_pPtBuf)
	{
		delete []m_pPtBuf;
		m_pPtBuf = NULL;
	}
}

BOOL CSysView::PreCreateWindow(CREATESTRUCT& cs)
{
	// TODO: 在此处通过修改 CREATESTRUCT cs 来修改窗口类或
	// 样式

	return CView::PreCreateWindow(cs);
}

void CSysView::SetScrollSizes(int nMapMode, SIZE sizeTotal,
								 const SIZE& sizePage, const SIZE& sizeLine)
{
	switch(m_iShowMode)
	{
	case SEGMENT_MODE|ReLoad_MODE:
	case SEGMENT_MODE:
		sizeTotal.cx += (m_rcSpace.left + m_rcSpace.right);
		break;
	case WAVE_MODE:	  // 回读模式下 宽度为 1024 个像素的波形显示区	
	case WAVE_MODE|ReLoad_MODE:
		sizeTotal.cx = 1024 +(m_rcSpace.left + m_rcSpace.right);
		break;
	default:
		sizeTotal.cx += (m_rcSpace.left + m_rcSpace.right);
		break;		
	}
	
	CScrollView::SetScrollSizes(nMapMode, sizeTotal, sizePage, sizeLine);
}
void CSysView::PlotBkGnd(CDC *pDC)
{	
	pDC->FillSolidRect(m_rcClient, RGB(0, 0, 0));

	int nChHeight = m_rcPolt.Height() / 2;	
	//******************************** Draw Margin Line
	CPen MarginPen(PS_SOLID, 2, RGB(135, 220, 40));
	CPen* pOriginPen = pDC->SelectObject(&MarginPen);

	pDC->MoveTo(m_rcPolt.left, m_rcPolt.top);
	pDC->LineTo(m_rcPolt.right, m_rcPolt.top);
	pDC->LineTo(m_rcPolt.right, m_rcPolt.bottom);
	pDC->LineTo(m_rcPolt.left, m_rcPolt.bottom);
	pDC->LineTo(m_rcPolt.left, m_rcPolt.top -1);		

	CFont* pOriginFont = pDC->SelectObject(&m_FontTxt);
				
	int iOldBKMd = pDC->SetBkMode(TRANSPARENT);
	UINT nOldTA = pDC->SetTextAlign(TA_CENTER);

	CString strText;

	switch(m_iShowMode)
	{	
	case (WAVE_MODE):  // 完整的波模式
	case (SEGMENT_MODE):  // 段模式	
		{	
			//******************************** Draw Center Line
			CPen CLPen(PS_DOT, 1, RGB(128, 255, 255));
			pDC->SelectObject(&CLPen);
			pDC->MoveTo(m_rcPolt.left, m_rcPolt.CenterPoint().y);
			pDC->LineTo(m_rcPolt.right, m_rcPolt.CenterPoint().y);

			pDC->SetBkMode(TRANSPARENT);
			pDC->SetTextAlign(TA_CENTER);
			pDC->SetTextColor(RGB(125, 255, 255));		
			pDC->TextOut(m_rcPolt.left-20, m_rcPolt.CenterPoint().y, "用户数据");
		}

		break;
	case SEGMENT_MODE|ReLoad_MODE:	
	case WAVE_MODE|ReLoad_MODE:		
		{
			//************ Draw Splitter Line Between Channel if nChNum = 4
			pDC->MoveTo(m_rcPolt.left, m_rcPolt.top + nChHeight);
			pDC->LineTo(m_rcPolt.right, m_rcPolt.top + nChHeight);

			//******************************** Draw Center Line
			CPen CLPen(PS_DOT, 1, RGB(128, 255, 255));
			pDC->SelectObject(&CLPen);

			pDC->MoveTo(m_rcPolt.left, m_rcPolt.top + nChHeight / 2);
			pDC->LineTo(m_rcPolt.right, m_rcPolt.top + nChHeight / 2);
			pDC->MoveTo(m_rcPolt.left, m_rcPolt.top + nChHeight + nChHeight / 2);
			pDC->LineTo(m_rcPolt.right, m_rcPolt.top + nChHeight + nChHeight / 2);

			//******************* Draw Text In the Grid					
			pDC->SetBkMode(TRANSPARENT);
			pDC->SetTextAlign(TA_CENTER);
			pDC->SetTextColor(RGB(125, 255, 255));
			pDC->TextOut(m_rcPolt.left-20, m_rcPolt.top+m_rcPolt.Height()/4, "用户数据");
			pDC->TextOut(m_rcPolt.left-20, m_rcPolt.top+m_rcPolt.Height()*3/4, "回读数据");

		}
		break;
	default:
		ASSERT(FALSE); // 没有的显示模式
		break;
	}

	//##################################
	SCROLLINFO scrHInf;
	GetScrollInfo(SB_HORZ, &scrHInf);

	pDC->SelectStockObject(ANSI_VAR_FONT);
	pDC->SetTextColor(RGB(125, 255, 255));

	switch(m_iShowMode)
	{	
	case SEGMENT_MODE|ReLoad_MODE:	
	case (SEGMENT_MODE):  // 段模式	
		strText = L"波形";
		break;
	case (WAVE_MODE):  // 完整的波模式
	case WAVE_MODE|ReLoad_MODE:		
		strText = L"波形(缩影)";
		break;
	default:
		ASSERT(FALSE); // 没有的显示模式
		break;
	}
	pDC->SetTextAlign(TA_CENTER);
	pDC->TextOut(m_rcPolt.left+m_rcPolt.Width()/2, m_rcPolt.top - 40, strText);	

	CPen COPen(PS_SOLID, 0, RGB(125, 255, 255));
	pDC->SelectObject(&COPen);
	for (int i = 0; i <= m_rcPolt.Width(); i += 10)
	{
		pDC->MoveTo(m_rcPolt.left+i, m_rcPolt.top);
		if (i % 50)
			pDC->LineTo(m_rcPolt.left + i, m_rcPolt.top - 5);
		else
		{
			pDC->LineTo(m_rcPolt.left + i, m_rcPolt.top - 10);
			strText.Format("%d", scrHInf.nPos + i);
			pDC->TextOut(m_rcPolt.left + i, m_rcPolt.top - 25, strText);
		}
	}	

	// 恢复设备GDI对象
	pDC->SetBkMode(iOldBKMd);
	pDC->SetTextAlign(nOldTA);
	pDC->SelectObject(pOriginPen);
	pDC->SelectObject(pOriginFont);
}

void CSysView::DrawWave(CDC* pDC, CRect rect)
{	
	SCROLLINFO scrInf;
	GetScrollInfo(SB_VERT, &scrInf);

	SCROLLINFO scrHInf;
	GetScrollInfo(SB_HORZ, &scrHInf);

	pDC->SaveDC();
	pDC->IntersectClipRect(m_rcPolt);

	rect.OffsetRect(-scrHInf.nPos, 0);
	switch(m_iShowMode)
	{
	case WAVE_MODE|ReLoad_MODE:		
		{	
			// 绘制用户填充数据
			CRect rcTop(rect);

			rcTop.bottom = rect.CenterPoint().y;
			rcTop.DeflateRect(0, 3);
			rcTop.right = rcTop.left + scrHInf.nMax - (m_rcSpace.right + m_rcSpace.left) + 1;	

			if (m_penPlay.m_hObject)
			{
				CPen* pOldPen = pDC->SelectObject(&m_penPlay);
				m_pParentFrm->m_Channel.DrawAllWave(pDC, rcTop);
				pDC->SelectObject(pOldPen);
			}	
			
			// 显示回读数据	
			if ( INVALID_HANDLE_VALUE != theApp.m_hDevice)
			{
				CRect rcBottom(rect);
				rcBottom.top = rect.CenterPoint().y;
				rcBottom.DeflateRect(0, 3);		

				// 从 scrHInf.nPos 开始的 scrHInf.nPage	
				long iRetWord(0);
				long ilsbCount = 1 << 12;  // 12位精度

				int iVisabePtCount = scrHInf.nPage - (m_rcSpace.left + m_rcSpace.right);

				if (iVisabePtCount < m_rcPolt.Width())
				{
					iVisabePtCount = scrHInf.nMax - (m_rcSpace.left + m_rcSpace.right);
				}

				for(int index = 0; index < iVisabePtCount; index++)
				{
					m_pPtBuf[index].x =  index + m_rcSpace.left;

					// 抽取点
					PCI8603_ReadDeviceBulkDA(theApp.m_hDevice, m_pDataBuf, scrHInf.nPos + index, 1, 
						&iRetWord, m_pParentFrm->m_Channel.m_iIndex);
					if (1 == iRetWord)
					{	
						m_pPtBuf[index].y = rcBottom.bottom - m_pDataBuf[0] * rcBottom.Height()/ilsbCount;
					}
					else
						break;
				}

				if (m_penPlay.m_hObject)
				{
					CPen* pOldPen = pDC->SelectObject(&m_penPlay);
					pDC->MoveTo(m_pPtBuf[0]);
					pDC->Polyline(m_pPtBuf, index - 1);
					pDC->SelectObject(pOldPen);
				}  
			}			
		}
		break;

	case (WAVE_MODE):  // 完整的波模式			
		rect.DeflateRect(0, 3);
		rect.right = rect.left + scrHInf.nMax - (m_rcSpace.right + m_rcSpace.left) + 1;					

		if (m_penPlay.m_hObject)
		{
			CPen* pOldPen = pDC->SelectObject(&m_penPlay);
			m_pParentFrm->m_Channel.DrawAllWave(pDC, rect);
			pDC->SelectObject(pOldPen);
		}				
		break;

	case SEGMENT_MODE|ReLoad_MODE:
		{	
			// 绘制用户填充的数据
			CRect rcTop(rect);
			rcTop.bottom = rect.CenterPoint().y;
			rcTop.DeflateRect(0, 3);

			m_pParentFrm->m_Channel.DrawWave(pDC, rcTop);	

			// 绘制从设备中读取的数据
			if ( INVALID_HANDLE_VALUE != theApp.m_hDevice)
			{
				CRect rcBottom(rect);
				rcBottom.top = rect.CenterPoint().y;	
				rcBottom.DeflateRect(0, 3);	

				// 从 scrHInf.nPos 开始的 scrHInf.nPage
				LONG iReadWord(rect.Width()), iRetWord(0);

				// 防止绘制多余数据
				if (iReadWord > scrHInf.nMax - (m_rcSpace.left + m_rcSpace.right))
				{
					iReadWord = scrHInf.nMax - (m_rcSpace.left + m_rcSpace.right);
				}
				//ASSERT(scrHInf.nPage <= rc.Width()); // 一页 小于宽度
				PCI8603_ReadDeviceBulkDA(theApp.m_hDevice, m_pDataBuf, scrHInf.nPos, iReadWord, 
					&iRetWord, m_pParentFrm->m_Channel.m_iIndex);

				if (iRetWord == iReadWord)
				{				
					long ilsbCount = 1 << 12;

					// 数据处理
					for(int index = 0; index < iReadWord; index++)
					{
						m_pPtBuf[index].x =  index + m_rcSpace.left;

						m_pPtBuf[index].y = rcBottom.bottom - m_pDataBuf[index] * rcBottom.Height()/ilsbCount;
					}

					if (m_penPlay.m_hObject)
					{
						CPen* pOldPen = pDC->SelectObject(&m_penPlay);
						pDC->MoveTo(m_pPtBuf[0]);
						pDC->Polyline(m_pPtBuf, iRetWord);
						pDC->SelectObject(pOldPen);
					}                
				}		
			}
					
		}
		break;	

	case (SEGMENT_MODE):  // 段模式		
		rect.DeflateRect(0, 3);
		m_pParentFrm->m_Channel.DrawWave(pDC, rect);	
		break;
	default:
		ASSERT(FALSE); // 没有的显示模式
		break;
	}

	pDC->RestoreDC(-1); // 恢复剪切区
}

// CSysView 绘制
void CSysView::OnDraw(CDC* pDC)
{
	CSysDoc* pDoc = GetDocument();
	ASSERT_VALID(pDoc);
	if (!pDoc)
		return;
	
	if(m_dcMem.m_hDC == NULL)
		m_dcMem.CreateCompatibleDC(pDC);
	if(m_Bmp.m_hObject == NULL)
	{
		m_Bmp.CreateCompatibleBitmap(pDC, m_rcClient.Width(), m_rcClient.Height());
		m_pOldBmp = m_dcMem.SelectObject(&m_Bmp);
	}	

	this->PlotBkGnd(&m_dcMem);
	this->DrawWave(&m_dcMem, m_rcPolt);

	if (m_iShowMode == SEGMENT_MODE)
	{		
		if (m_rcPolt.PtInRect(m_ptLBDown) && m_bShowInfo)
		{

			int iOldBkMode = m_dcMem.SetBkMode(TRANSPARENT);
			m_dcMem.SetTextColor(RGB(0, 200 , 0));
			
			UINT OldTA = m_dcMem.SetTextAlign(TA_BOTTOM|TA_CENTER);		
			m_dcMem.TextOut(m_ptLBDown.x, m_ptLBDown.y, m_strInfo);	
			m_dcMem.SetTextAlign(OldTA);
			
			CPen pen(PS_DOT, 1, RGB(255, 255, 128));
			CPen* pOldPen = m_dcMem.SelectObject(&pen);
			m_dcMem.MoveTo( m_ptLBDown.x, m_rcPolt.top);
			m_dcMem.LineTo( m_ptLBDown.x, m_rcPolt.bottom);
			m_dcMem.SelectObject(pOldPen);

			m_dcMem.SetBkMode(iOldBkMode);	
			m_bShowInfo = FALSE;
		}
	}

	CRect rcClient(m_rcClient);
	pDC->DPtoLP(&rcClient);
	pDC->BitBlt(rcClient.left, rcClient.top, rcClient.Width(), rcClient.Height()
		,&m_dcMem, 0, 0, SRCCOPY);		
}
// CSysView 打印

BOOL CSysView::OnPreparePrinting(CPrintInfo* pInfo)
{
	// 默认准备
	return DoPreparePrinting(pInfo);
}

void CSysView::OnBeginPrinting(CDC* /*pDC*/, CPrintInfo* /*pInfo*/)
{
	// TODO: 打印前添加额外的初始化
}

void CSysView::OnEndPrinting(CDC* /*pDC*/, CPrintInfo* /*pInfo*/)
{
	// TODO: 打印后添加清除过程
}


// CSysView 诊断

#ifdef _DEBUG
void CSysView::AssertValid() const
{
	CView::AssertValid();
}

void CSysView::Dump(CDumpContext& dc) const
{
	CView::Dump(dc);
}

CSysDoc* CSysView::GetDocument() const // 非调试版本是内联的
{
	ASSERT(m_pDocument->IsKindOf(RUNTIME_CLASS(CSysDoc)));
	return (CSysDoc*)m_pDocument;
}
#endif //_DEBUG


// CSysView 消息处理程序

void CSysView::OnInitialUpdate()
{
	CScrollView::OnInitialUpdate();	

	m_pParentFrm = STATIC_DOWNCAST(CChildFrame, GetParentFrame());

	CSize sizeTotal;
	// TODO: 计算此视图的合计大小
	sizeTotal.cx = 1000;
	sizeTotal.cy = 200;
	SetScrollSizes(MM_TEXT, sizeTotal);  // 设置滚动大小
	

	if (m_penPlay.m_hObject != NULL)
	{
		m_penPlay.DeleteObject();
	}

	m_penPlay.CreatePen(PS_SOLID, 1, RGB(255, 0, 0));	

	if (m_FontTxt.m_hObject != NULL)
	{
		m_FontTxt.DeleteObject();
	}

	m_FontTxt.CreateFont(12, 0, 900, 900, FW_NORMAL, FALSE, FALSE, FALSE,
		ANSI_CHARSET, OUT_CHARACTER_PRECIS, CLIP_DEFAULT_PRECIS,
		DEFAULT_QUALITY, DEFAULT_PITCH, "宋体");
		
}

BOOL CSysView::OnEraseBkgnd(CDC* pDC)
{
	return TRUE;	
}

void CSysView::OnSize(UINT nType, int cx, int cy)
{
	CScrollView::OnSize(nType, cx, cy);

	GetClientRect(&m_rcClient);
	m_rcPolt.left = m_rcClient.left + m_rcSpace.left;
	m_rcPolt.right = m_rcClient.right - m_rcSpace.right;
	m_rcPolt.top = m_rcClient.top + m_rcSpace.top;
	m_rcPolt.bottom = m_rcClient.bottom - m_rcSpace.bottom;
	//m_rcPolt.DeflateRect(m_rcSpace);
	if(m_pOldBmp != NULL)
	{
		m_dcMem.SelectObject(m_pOldBmp);
		m_Bmp.DeleteObject();
		m_pOldBmp = NULL;
	}

	if (NULL != m_pDataBuf)
	{
		delete []m_pDataBuf;		
		m_pDataBuf = NULL;
	}

	if (NULL != m_pPtBuf)
	{
		delete []m_pPtBuf;
		m_pPtBuf = NULL;
	}

	if (cx > 0)
	{
		m_pDataBuf = new SHORT[cx];
		m_pPtBuf = new POINT[cx];
	}	
}

void CSysView::OnHScroll(UINT nSBCode, UINT nPos, CScrollBar* pScrollBar)
{
	this->Invalidate();

	CScrollView::OnHScroll(nSBCode, nPos, pScrollBar);
}

void CSysView::OnVScroll(UINT nSBCode, UINT nPos, CScrollBar* pScrollBar)
{
	this->Invalidate();
	CScrollView::OnVScroll(nSBCode, nPos, pScrollBar);
}

// 制定段 在中间显示？？？？？
BOOL CSysView::ShowSegment(int iSegment)
{
	// 移动到制定段
	ASSERT(iSegment >= 0 && iSegment < m_pParentFrm->m_Channel.GetSigmentCount());
	int iBengin(0), iEnd(0);
	if (m_pParentFrm->m_Channel.GetSegmentPos(iSegment, iBengin, iEnd))
	{
		SCROLLINFO vScrInfo;
		this->GetScrollInfo(SB_HORZ, &vScrInfo);
		this->SetScrollPos(SB_HORZ, (iBengin + iEnd)/2 - vScrInfo.nPage/2);        
		this->Invalidate();
		return TRUE;
	}
	else
		return FALSE;
}

void CSysView::OnLButtonDown(UINT nFlags, CPoint point)
{
	if (m_iShowMode == SEGMENT_MODE)
	{			
		CRect rcInvad;
		rcInvad.top = m_rcPolt.top;
		rcInvad.bottom = m_rcPolt.bottom;
		rcInvad.left = m_ptLBDown.x - 1;
		rcInvad.right = m_ptLBDown.x + 1;
		
		InvalidateRect(rcInvad);
		rcInvad.top = m_ptLBDown.y - m_oldSZ.cy;
		rcInvad.bottom = m_ptLBDown.y;
		rcInvad.left = m_ptLBDown.x - m_oldSZ.cx/2;
		rcInvad.right = rcInvad.left + m_oldSZ.cx;
		InvalidateRect(rcInvad);	
		
		m_ptLBDown = point;	

		if (m_rcPolt.PtInRect(m_ptLBDown))
		{				
			m_bShowInfo = TRUE;			
			SCROLLINFO scrInf;
			
			GetScrollInfo(SB_VERT, &scrInf);
			
			SCROLLINFO scrHInf;
			GetScrollInfo(SB_HORZ, &scrHInf);
			// 设置状态栏的信息
			int iPos = m_ptLBDown.x + scrHInf.nPos - m_rcSpace.left;
			LONG lsb(0);
			m_pParentFrm->m_Channel.GetData(iPos, &lsb);			
			
			m_strInfo.Format("POS = %d, LSB = %d", iPos, lsb);	
			m_oldSZ = m_dcMem.GetTextExtent(m_strInfo);
			rcInvad.top = m_rcPolt.top;
			rcInvad.bottom = m_rcPolt.bottom;
			rcInvad.left = m_ptLBDown.x - 1;
			rcInvad.right = m_ptLBDown.x + 1;
			
			InvalidateRect(rcInvad);
			rcInvad.top = m_ptLBDown.y - m_oldSZ.cy;
			rcInvad.bottom = m_ptLBDown.y;
			rcInvad.left = m_ptLBDown.x - m_oldSZ.cx/2;
			rcInvad.right = rcInvad.left + m_oldSZ.cx;
			InvalidateRect(rcInvad);
		}
	}	

	CScrollView::OnLButtonDown(nFlags, point);
}

void CSysView::OnLButtonDblClk(UINT nFlags, CPoint point)
{
	// TODO: 在此添加消息处理程序代码和/或调用默认值
	// 配置选中段

	CScrollView::OnLButtonDblClk(nFlags, point);
}

void CSysView::OnDestroy()
{
	CScrollView::OnDestroy();
}

void CSysView::OnContextMenu(CWnd* pWnd, CPoint point)
{
	CMenu menuRBtn;  // 右键菜单

	VERIFY(menuRBtn.LoadMenu(IDR_MENU_WAVE_RB));  //　载入菜单资源

	if(menuRBtn.m_hMenu != NULL)
	{
		CMenu* pPopup = menuRBtn.GetSubMenu(0);
		ASSERT(pPopup != NULL);	

		if (WAVE_MODE == (m_iShowMode & WAVE_MODE))
		{
			pPopup->CheckMenuItem(IDM_SHOW_ALL_WAVE,  MF_CHECKED | MF_BYCOMMAND);
		}
		else
			pPopup->CheckMenuItem(IDM_SHOW_ALL_WAVE,  MF_UNCHECKED | MF_BYCOMMAND);

		if (ReLoad_MODE == (m_iShowMode & ReLoad_MODE))
			pPopup->CheckMenuItem(IDM_RELOAD_DATA,	MF_CHECKED|MF_BYCOMMAND);
		else
			pPopup->CheckMenuItem(IDM_RELOAD_DATA, MF_UNCHECKED | MF_BYCOMMAND);

		pPopup->TrackPopupMenu(TPM_LEFTALIGN |TPM_RIGHTBUTTON, point.x, point.y, this);
	}
}

void CSysView::OnReloadData()
{
	if (m_iShowMode & ReLoad_MODE)  // 波形模式
	{
		m_iShowMode ^= (ReLoad_MODE);  // 去除完波并  ^ 异或  ~ 求反		
	} 
	else
	{		
		m_iShowMode |= ReLoad_MODE;  
	}
	this->Invalidate();
}

void CSysView::OnUpdateReloadData(CCmdUI *pCmdUI)
{
	// TODO: 在此添加命令更新用户界面处理程序代码	
}

void CSysView::OnShowAllWave()
{
	if (m_iShowMode & WAVE_MODE)  // 波形模式
	{
		m_iShowMode ^= WAVE_MODE;  // 去除完波并
		m_iShowMode |= SEGMENT_MODE;  // 添加段显示模式 标志位		
		CSize sizeTotal;	
		sizeTotal.cx = m_pParentFrm->m_Channel.GetDateSize();
		sizeTotal.cy = 200;				
		this->SetScrollSizes(MM_TEXT, sizeTotal);  // 设置滚动大小
		
	} 
	else
	{
		m_iShowMode ^= SEGMENT_MODE;
		m_iShowMode |= WAVE_MODE;  
		CSize sizeTotal;	
		sizeTotal.cx = 0;
		sizeTotal.cy = 200;				
		this->SetScrollSizes(MM_TEXT, sizeTotal);  // 设置滚动大小
	}
	this->Invalidate();
}

void CSysView::OnUpdateShowAllWave(CCmdUI *pCmdUI)
{
//	pCmdUI->SetCheck(this->m_iShowMode == WAVE_MODE);
}

void CSysView::OnMouseMove(UINT nFlags, CPoint point)
{
	if (m_iShowMode == SEGMENT_MODE)
	{		
		if (m_rcPolt.PtInRect(point))
		{
			SCROLLINFO scrInf;
			GetScrollInfo(SB_VERT, &scrInf);
			
			SCROLLINFO scrHInf;
			GetScrollInfo(SB_HORZ, &scrHInf);
			// 设置状态栏的信息
			int iPos = point.x + scrHInf.nPos - m_rcSpace.left;
			LONG lsb(0);
			m_pParentFrm->m_Channel.GetData(iPos, &lsb);
			
			CString strInfo;
			strInfo.Format("POS %d, LSB %d", iPos, lsb);
			
			
			// 设置状态栏信息 m_pParentFrm
			CStatusBarCtrl& sb = 
				((CChildFrame*)m_pParentFrm)->m_wndStatusBar.GetStatusBarCtrl();			

				//((CMainFrame*)theApp.m_pMainWnd)->m_wndStatusBar.GetStatusBarCtrl();			
			sb.SetText(strInfo, 3, SBT_NOBORDERS);		
			
			if (nFlags & MK_LBUTTON)  // 左键按下移动鼠标
			{
				CRect rcInvad;	
				// 擦除显示的信息
				rcInvad.top = m_rcPolt.top;
				rcInvad.bottom = m_rcPolt.bottom;
				rcInvad.left = m_ptLBDown.x - 1;
				rcInvad.right = m_ptLBDown.x + 1;
				
				InvalidateRect(rcInvad);
				rcInvad.top = m_ptLBDown.y - m_oldSZ.cy;
				rcInvad.bottom = m_ptLBDown.y;
				rcInvad.left = m_ptLBDown.x - m_oldSZ.cx/2;
				rcInvad.right = rcInvad.left + m_oldSZ.cx;
				InvalidateRect(rcInvad);
				
				m_ptLBDown = point;	  // 更新鼠标左键按下位置	
				
				m_bShowInfo = TRUE;		
				
				// 更新显示信息
				m_strInfo.Format("POS = %d, LSB = %d", iPos, lsb);	
				m_oldSZ = m_dcMem.GetTextExtent(m_strInfo);


				rcInvad.top = m_rcPolt.top;
				rcInvad.bottom = m_rcPolt.bottom;
				rcInvad.left = m_ptLBDown.x - 1;
				rcInvad.right = m_ptLBDown.x + 1;
				
				InvalidateRect(rcInvad);
				rcInvad.top = m_ptLBDown.y - m_oldSZ.cy;
				rcInvad.bottom = m_ptLBDown.y;
				rcInvad.left = m_ptLBDown.x - m_oldSZ.cx/2;
				rcInvad.right = rcInvad.left + m_oldSZ.cx;
				InvalidateRect(rcInvad);				
			}
		
		}
		else
		{
			m_bShowInfo = FALSE;
		}		
	}
	else
	{
		// 把当前位置转换为码值 和数据点索引

	}
	CScrollView::OnMouseMove(nFlags, point);
}