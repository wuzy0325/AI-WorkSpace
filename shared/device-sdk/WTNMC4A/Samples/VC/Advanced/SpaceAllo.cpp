// SpaceAllo.cpp : 实现文件
//

#include "stdafx.h"
#include "Sys.h"
#include "SpaceAllo.h"
#include ".\spaceallo.h"


// CSpaceAllo
COLORREF CSpaceAllo::m_clr[] =
{
	RGB(254, 153, 16),   // 
	RGB(100, 128, 255),  //
	RGB(0,   255, 0),    //
	RGB(255, 0,   255)		// 
};

IMPLEMENT_DYNAMIC(CSpaceAllo, CWnd)
CSpaceAllo::CSpaceAllo()
{
	CSpace rc;
	rc.m_clr = m_clr[0];
	rc.m_clrUsed = RGB(255, 0, 0);
	rc.m_rc.SetRect(0,0,0,0);
	rc.m_strTxt = "区域0";
	rc.m_iSpace = 100;
	m_arraySpace.Add(rc);

	m_nSpaceCount = 1;					// 区域个数
	m_iSelSpace = -1;                  // 当前位置  有下压效果
	m_nTotalSpace = 100;				// 总的可用空间

	m_pOldBmp = NULL;
	m_iMoveIndex = -1;
	m_bMoving = FALSE;

	m_hCurCurrent = NULL;
	m_hCursorSizeWe  = (HCURSOR)AfxGetApp()->LoadStandardCursor(IDC_SIZEWE);  // 水平拖动
	m_hcurDefault =  (HCURSOR)AfxGetApp()->LoadStandardCursor(IDC_ARROW);

	m_clrBK =  RGB(50, 50, 50);

	m_bEnableChangeSpace = TRUE;

}

CSpaceAllo::~CSpaceAllo()
{
	if (m_pOldBmp != NULL)
	{
		m_dcMem.SelectObject(m_pOldBmp);
		m_pOldBmp = NULL;
	}
}

CSpaceAllo::CSpace::CSpace()
{
	m_rc.SetRectEmpty();				
	m_rcUsed.SetRectEmpty();		// 占用的空间部分的巨型	
	
	m_iSpace = 0;       // 总的空间
	m_iUsedSpace = 0;   // 占用空间
}
CSpaceAllo::CSpace::~CSpace()
{

}


BOOL CSpaceAllo::SetSpaceCount(int iNewCount)
{
	ASSERT(iNewCount > 0);
	if (iNewCount > 0)
	{
		if (iNewCount > m_nSpaceCount)
		{
			// 增家新的通道			
			CSpace space;

			for(int Index = m_nSpaceCount; Index < iNewCount; Index++)
			{
				space.m_clr = m_clr[Index%4];
				space.m_clrUsed = RGB(255, 0, 0);
				space.m_strTxt.Format("通道%d", Index);
				space.m_iSpace = 10;
				m_arraySpace.Add(space);  // 添加到通道数组中				
			}
		}
		else if(iNewCount < m_nSpaceCount)
		{
			m_arraySpace.RemoveAt(iNewCount, m_nSpaceCount - iNewCount);  // 添加到通道数组中
		}

		m_nSpaceCount = iNewCount;
		int iSpace =  m_nTotalSpace/m_nSpaceCount;
		for(int Index = 0; Index < m_nSpaceCount; Index++)
		{
			m_arraySpace[Index].m_iSpace = iSpace;

		}
		this->CalculateRect();
		if (this->GetSafeHwnd())
		{
			this->Invalidate();
		}
		
		return TRUE;
	}
	else
		return FALSE;
}
BOOL CSpaceAllo::GetSpaceCount(int* iNewCount)
{
	*iNewCount = m_nSpaceCount;
	return TRUE;
}

BOOL CSpaceAllo::SetTotalSpace(int iSpace)
{
	ASSERT(iSpace > 0);
	if (iSpace > 0)
	{
		m_nTotalSpace = iSpace;
		this->CalculateRect();
		if (this->GetSafeHwnd())
		{
			this->Invalidate();
		}
		return TRUE;
	}
	else
	{
		return FALSE;
	}
	
}
BOOL CSpaceAllo::GetTotalSpace(int* iSpace)
{
	*iSpace = m_nTotalSpace;
	return TRUE;
}


// 考虑占用空间问题？？？？？？？？？
BOOL CSpaceAllo::SetSpace(int index, int iNewSpace)
{
	BOOL bRet = FALSE;
	ASSERT(m_nSpaceCount == m_arraySpace.GetSize());
	if (index > 0 && index < m_nSpaceCount - 1)
	{
		int iOldSpace0 = m_arraySpace[index].m_iSpace;
		int iOldSpace1 = m_arraySpace[index + 1].m_iSpace;

		int  iAdd = iNewSpace - m_arraySpace[index].m_iSpace;

		m_arraySpace[index].m_iSpace += iAdd;
		m_arraySpace[index + 1].m_iSpace -= iAdd;

		if (m_arraySpace[index].m_iSpace < m_arraySpace[index].m_iUsedSpace||
			m_arraySpace[index + 1].m_iSpace < m_arraySpace[index].m_iUsedSpace)
		{  // 分配后的空间不能小于 0 			
			m_arraySpace[index].m_iSpace = iOldSpace0;
			m_arraySpace[index + 1].m_iSpace = iOldSpace1;
		}
		else
		{
			bRet = TRUE;
		}	
	}
	else if (index == 0)
	{
		int iOldSpace0 = m_arraySpace[0].m_iSpace;
		int iOldSpace1 = m_arraySpace[1].m_iSpace;

		int  iAdd = iNewSpace - m_arraySpace[0].m_iSpace;

        m_arraySpace[0].m_iSpace += iAdd;
		m_arraySpace[1].m_iSpace -= iAdd;

		if (m_arraySpace[0].m_iSpace < m_arraySpace[0].m_iUsedSpace || 
			m_arraySpace[1].m_iSpace < m_arraySpace[1].m_iUsedSpace)
		{  // 分配后的空间不能小于 已占用的空间 		
			m_arraySpace[0].m_iSpace = iOldSpace0;
			m_arraySpace[1].m_iSpace=iOldSpace1;
		}
		else
		{
			bRet = TRUE;
		}		
	}
	else if (index == m_nSpaceCount - 1)
	{
		int iOldSpace0 = m_arraySpace[index].m_iSpace;
		int iOldSpace1 = m_arraySpace[index - 1].m_iSpace;

		int  iAdd = iNewSpace - m_arraySpace[index].m_iSpace;
		
		m_arraySpace[index].m_iSpace += iAdd;
		m_arraySpace[index - 1].m_iSpace -= iAdd;

		if (m_arraySpace[index].m_iSpace < m_arraySpace[index].m_iUsedSpace || 
			m_arraySpace[index - 1].m_iSpace < m_arraySpace[index - 1].m_iUsedSpace)
		{  // 分配后的空间不能小于 已占用的空间 		
			m_arraySpace[index].m_iSpace = iOldSpace0;
			m_arraySpace[index - 1].m_iSpace=iOldSpace1;
		}
		else
		{
			bRet = TRUE;
		}	
	} 	

	if (TRUE == bRet)
	{
		if (this->GetSafeHwnd())
		{
			CalculateRect();
			this->Invalidate();  // 更新显示
		}
	}
	else
		AfxMessageBox("空间分配错误", MB_OK|MB_ICONEXCLAMATION| MB_ICONWARNING);
	return bRet;
}

BOOL CSpaceAllo::GetSpace(int index, int* iSpace)
{
	ASSERT(m_nSpaceCount == m_arraySpace.GetSize());
	if (index >= 0 && index < m_nSpaceCount)
	{
		*iSpace = m_arraySpace[index].m_iSpace;

		return TRUE;
	}
	return FALSE;
}

BOOL CSpaceAllo::SetTxt(int index, CString strTxt)
{
	if (index >= 0 && index < m_nSpaceCount)
	{
		m_arraySpace[index].m_strTxt = strTxt;           // 更新显示文本
		this->InvalidateRect(m_arraySpace[index].m_rc);  // 更新重新显示
		return TRUE;
	} 
	else
	{
		ASSERT(FALSE);
		return FALSE;
	}	
}

BOOL CSpaceAllo::GetTxt(int index, CString& strTxt)
{
	if (index >= 0 && index < m_nSpaceCount)
	{
		strTxt = m_arraySpace[index].m_strTxt;
		return TRUE;
	} 
	else
	{
		ASSERT(FALSE);
		return FALSE;
	}	
}

BEGIN_MESSAGE_MAP(CSpaceAllo, CWnd)
	ON_WM_PAINT()
	ON_WM_SIZE()
	ON_WM_LBUTTONDBLCLK()
	ON_WM_LBUTTONDOWN()
	ON_WM_LBUTTONUP()
	ON_WM_MOUSEMOVE()
	ON_WM_SETCURSOR()	
END_MESSAGE_MAP()

BOOL CSpaceAllo::Create(LPCTSTR lpszWindowName, DWORD dwStyle, const RECT& rect, CWnd* pParentWnd, UINT nID)
{
	BOOL result ;
	static CString className = AfxRegisterWndClass(CS_HREDRAW | CS_VREDRAW | CS_DBLCLKS) ;

	result = CWnd::CreateEx( WS_EX_STATICEDGE, 
		className, NULL, dwStyle, 
		rect.left, rect.top, rect.right-rect.left, rect.bottom-rect.top,
		pParentWnd->GetSafeHwnd(), (HMENU)nID);
	return result;
}

// CSpaceAllo 消息处理程序

void CSpaceAllo::OnDraw(CDC* pDC)
{	// 3d边框
	CRect rc(m_rcClient);	
	rc.DeflateRect(1, 1);
	pDC->FillSolidRect(rc, m_clrBK);

	int oldBkMode = pDC->SetBkMode(TRANSPARENT);
	for (int index = 0; index < m_nSpaceCount; index++)
	{	
		rc = m_arraySpace[index].m_rcUsed;		
		pDC->FillSolidRect(rc, m_arraySpace[index].m_clrUsed);

		rc = m_arraySpace[index].m_rc;		
		rc.left = m_arraySpace[index].m_rcUsed.right;	
		pDC->FillSolidRect(rc, m_arraySpace[index].m_clr);		

		// 输出文本
		pDC->DrawText(m_arraySpace[index].m_strTxt, m_arraySpace[index].m_rc,  DT_SINGLELINE|DT_VCENTER|DT_CENTER);

	}

	pDC->SetBkMode(oldBkMode);
	// 给选中通道绘制3d边框
	if (m_iSelSpace >= 0 && m_nSpaceCount)
	{
		rc = m_arraySpace[m_iSelSpace].m_rc;		
		pDC->Draw3dRect(rc, RGB(57,101,82), RGB(239,243,247));
		rc.DeflateRect(2, 2);
		pDC->DrawFocusRect(rc);
		rc.DeflateRect(1, 1);
		pDC->DrawFocusRect(rc);
		
	}

	if (TRUE == m_bMoving)
	{
		pDC->MoveTo(m_Ptold.x, m_rcClient.top);
		pDC->LineTo(m_Ptold.x, m_rcClient.bottom);
	}
}

void  CSpaceAllo::CalculateRect()   // 计算各个区域的大小
{
	ASSERT(m_nSpaceCount == m_arraySpace.GetSize());
	if ( m_rcClient.Width() > 0)
	{
		int ileft = 0;	
		int iTotalSpace = 0;
		for (int index = 0; index < m_nSpaceCount; index++)
		{	
			iTotalSpace += m_arraySpace[index].m_iSpace;
			m_arraySpace[index].m_rc.top = m_rcClient.top;
			m_arraySpace[index].m_rc.bottom = m_rcClient.bottom;

			m_arraySpace[index].m_rc.left = ileft;
			m_arraySpace[index].m_rc.right = iTotalSpace * m_rcClient.Width()/m_nTotalSpace;  

			// 计算占用的空间矩形
			m_arraySpace[index].m_rcUsed = m_arraySpace[index].m_rc;
			m_arraySpace[index].m_rcUsed.right = m_arraySpace[index].m_rc.left +
									(m_arraySpace[index].m_rc.Width() * m_arraySpace[index].m_iUsedSpace)/m_arraySpace[index].m_iSpace;
			
				
			ileft = m_arraySpace[index].m_rc.right;
		}
	}

}

void CSpaceAllo::OnPaint()
{
	CPaintDC dc(this); // device context for painting

	if (NULL == m_dcMem.m_hDC)
	{
		m_dcMem.CreateCompatibleDC(&dc);
	}

	if (m_pOldBmp == NULL)
	{
		m_bmp.CreateCompatibleBitmap(&dc, m_rcClient.Width(), m_rcClient.Height());
		m_pOldBmp = m_dcMem.SelectObject(&m_bmp);
	}

	// 绘制间隔并绘制各个区域的 当前选中区域
	OnDraw(&m_dcMem);
	dc.BitBlt(m_rcClient.left, m_rcClient.top, m_rcClient.Width(), m_rcClient.Height(),
		&m_dcMem, 0, 0, SRCCOPY);

	
}

void CSpaceAllo::OnSize(UINT nType, int cx, int cy)
{
	CWnd::OnSize(nType, cx, cy);
	this->GetClientRect(m_rcClient);
	if (NULL != m_pOldBmp)
	{
		m_dcMem.SelectObject(m_pOldBmp);
		m_pOldBmp = NULL;
		m_bmp.DeleteObject();
	}

	// 更新各个区域的大小
	this->CalculateRect();
}

void CSpaceAllo::OnLButtonDblClk(UINT nFlags, CPoint point)
{
	// TODO: 在此添加消息处理程序代码和/或调用默认值

	CWnd::OnLButtonDblClk(nFlags, point);
}

void CSpaceAllo::OnLButtonDown(UINT nFlags, CPoint point)
{	
	if (m_bEnableChangeSpace)
	{
		for (int index = 0; index < m_nSpaceCount - 1; index++)
		{	
			if (point.x - 4 < m_arraySpace[index].m_rc.right &&
				point.x + 4 > m_arraySpace[index].m_rc.right)
			{
				m_iMoveIndex = index;		
				this->SetCapture();
				m_PtBegin = point;
				m_bMoving = TRUE;			
				break;
			}
		}	

	}

	if (m_iSelSpace >=0 && m_iSelSpace < m_nSpaceCount)
	{
		this->InvalidateRect(m_arraySpace[m_iSelSpace].m_rc);
	}

	if (FALSE ==m_bMoving)
	{
		for (int index = 0; index < m_nSpaceCount; index++)
		{
			if (m_arraySpace[index].m_rc.PtInRect(point))
			{
				// 通知父窗体当前段改变
				this->GetOwner()->SendMessage(UM_CHANNEL_CHANGE, index, m_iSelSpace);

				m_iSelSpace = index;
				this->InvalidateRect(m_arraySpace[index].m_rc);			
				break;
			}			
		}
	}		
	
	CWnd::OnLButtonDown(nFlags, point);
}

void CSpaceAllo::OnLButtonUp(UINT nFlags, CPoint point)
{
	// TODO: 在此添加消息处理程序代码和/或调用默认值
	if (m_bMoving == TRUE)
	{
		int iModeIndex = m_iMoveIndex;
		ReleaseCapture();
		m_iMoveIndex = -1;
		m_bMoving = FALSE;

		m_PtEnd = point;
		// 修正防止出现分配负值空间
		 
		if (m_PtEnd.x < m_arraySpace[iModeIndex].m_rc.left + 2)
		{
			m_PtEnd.x = m_arraySpace[iModeIndex].m_rc.left + 2;			
		}

		if (m_PtEnd.x > m_arraySpace[iModeIndex + 1].m_rc.right - 2)
		{
			m_PtEnd.x = m_arraySpace[iModeIndex + 1].m_rc.right - 2;			
		}

		int iOffSet = m_nTotalSpace*(m_PtBegin.x - m_PtEnd.x)/m_rcClient.Width();
		m_arraySpace[iModeIndex].m_iSpace -= iOffSet;
		ASSERT(m_arraySpace[iModeIndex].m_iSpace > 0);
		m_arraySpace[iModeIndex + 1].m_iSpace += iOffSet;
		ASSERT(m_arraySpace[iModeIndex + 1].m_iSpace > 0);

#ifdef _DEBUG
		int iTotalSpace = 0;
		for (int index = 0; index < m_nSpaceCount; index++)
		{
			iTotalSpace += m_arraySpace[index].m_iSpace;
		}
		ASSERT(m_nTotalSpace == iTotalSpace);
#endif
		CalculateRect();
		/*CRect rcInvali(m_rcClient);
		rcInvali.left = m_PtEnd.x >= m_PtBegin.x ? m_PtBegin.x:m_PtEnd.x;
		rcInvali.right = m_PtEnd.x >= m_PtBegin.x ? m_PtEnd.x: m_PtBegin.x;*/
		this->InvalidateRect(m_arraySpace[iModeIndex].m_rc);
		this->InvalidateRect(m_arraySpace[iModeIndex + 1].m_rc);

		// 通知父窗体通道的内存占用量发生变化
		this->GetOwner()->SendMessage(UM_SPACE_CHANGE, iModeIndex);
	}

	CWnd::OnLButtonUp(nFlags, point);
}

void CSpaceAllo::OnMouseMove(UINT nFlags, CPoint point)
{		
	m_hCurCurrent = NULL;	
	if (TRUE == m_bEnableChangeSpace)
	{
		for (int index = 0; index < m_nSpaceCount - 1; index++)
		{	
			if (point.x - 4 < m_arraySpace[index].m_rc.right &&
				point.x + 4 > m_arraySpace[index].m_rc.right)
			{				
				m_hCurCurrent = m_hCursorSizeWe;
				break;
			}
		}
	}
	
	if (TRUE == m_bMoving)
	{ // 调整矩形大小
		ASSERT(m_iMoveIndex >= 0 && m_iMoveIndex < m_nSpaceCount - 1);
		CRect rc(m_rcClient);

		// 更新旧的区域
		rc.left = m_Ptold.x - 1;
		rc.right = m_Ptold.x + 1;
		InvalidateRect(rc);

		// 更新新的区域
		m_Ptold = point;
		rc.left = m_Ptold.x - 1;
		rc.right = m_Ptold.x + 1;
		InvalidateRect(rc);		

		// 限制移动防止 在相邻的区域中 
		// ????? 没有考虑内存的使用情况
		if (point.x < m_arraySpace[m_iMoveIndex].m_rcUsed.right + 2)
		{
			point.x = m_arraySpace[m_iMoveIndex].m_rcUsed.right + 2;
			CPoint pt = point;
			this->ClientToScreen(&pt);
			::SetCursorPos(pt.x, pt.y);
		}

		if (point.x > m_arraySpace[m_iMoveIndex + 1].m_rc.right - 2 - m_arraySpace[m_iMoveIndex + 1].m_rcUsed.Width())
		{
			point.x = m_arraySpace[m_iMoveIndex + 1].m_rc.right - 2- m_arraySpace[m_iMoveIndex + 1].m_rcUsed.Width();
			CPoint pt = point;
			this->ClientToScreen(&pt);
			::SetCursorPos(pt.x, pt.y);
		}		
	}
	CWnd::OnMouseMove(nFlags, point);
}

BOOL CSpaceAllo::OnSetCursor(CWnd* pWnd, UINT nHitTest, UINT message)
{		
	BOOL bRet =  CWnd::OnSetCursor(pWnd, nHitTest, message);

	if(m_hCurCurrent != NULL)
	{
		::SetCursor(m_hCurCurrent);
	}
	else 
	{	
		::SetClassLong(GetSafeHwnd(), GCL_HCURSOR, LONG(m_hcurDefault));
	}

	return bRet;	
}


BOOL CSpaceAllo::GetCurChannel(int* iChannel)
{
	*iChannel = m_iSelSpace;
    return TRUE;
}

BOOL CSpaceAllo::SetCurChannel(int iChannel)
{
	if (iChannel >=0 && iChannel < m_nSpaceCount)
	{
		m_iSelSpace = iChannel;
		CRect rcInvad = m_arraySpace[iChannel].m_rc;
		this->InvalidateRect(rcInvad);
		return TRUE;
	} 
	else
	{
		return FALSE;
	}	
}

BOOL CSpaceAllo::SetUsedSpace(int iChannel, int iUsedSpace)	//  设置指定段使用的空间
{
	ASSERT(iChannel >=0 && iChannel < m_nSpaceCount);	

	BOOL bRet = FALSE;
	if (iChannel >=0 && iChannel < m_nSpaceCount)
	{
		if (iUsedSpace <= m_arraySpace[iChannel].m_iSpace
			&& iUsedSpace >= 0)
		{
			m_arraySpace[iChannel].m_iUsedSpace = iUsedSpace;
			// 计算占用的空间矩形
			m_arraySpace[iChannel].m_rcUsed.right = m_arraySpace[iChannel].m_rc.left +
				(m_arraySpace[iChannel].m_rc.Width() * m_arraySpace[iChannel].m_iUsedSpace)/m_arraySpace[iChannel].m_iSpace;


			this->InvalidateRect(m_arraySpace[iChannel].m_rc);
			bRet = TRUE;
		}		
	} 
	return bRet;	
}

BOOL CSpaceAllo::GetUsedSpace(int iChannel, int* piSpace)	//  得到指定段使用的空间
{
	ASSERT(iChannel >=0 && iChannel < m_nSpaceCount);
	ASSERT(NULL != piSpace);

	BOOL bRet = FALSE;
	if (iChannel >=0 && iChannel < m_nSpaceCount)
	{		
		*piSpace = m_arraySpace[iChannel].m_iUsedSpace;		
		bRet = TRUE;		
	} 
	return bRet;
}

BOOL CSpaceAllo::GetSpaceFree(int iChannel, int* piNewSpace)	//  得到指定段可用的空间
{
	ASSERT(piNewSpace != NULL);
	BOOL bRet = FALSE;
	if (iChannel >=0 && iChannel < m_nSpaceCount)
	{	
		if (piNewSpace)
		{
			*piNewSpace = m_arraySpace[iChannel].m_iSpace - m_arraySpace[iChannel].m_iUsedSpace;
			ASSERT(*piNewSpace >=0 );
		}
		bRet = TRUE;				
	} 
	return bRet;
}

BOOL CSpaceAllo::AllocSpace(int iChannel, int iNewSpace)		//  增加空间分配
{
	ASSERT(iChannel >=0 && iChannel < m_nSpaceCount);

	BOOL bRet = FALSE;
	if (iChannel >=0 && iChannel < m_nSpaceCount)
	{		
		bRet = SetUsedSpace(iChannel, 
			(iNewSpace + m_arraySpace[iChannel].m_iUsedSpace));		
	}
	return bRet;
}

void CSpaceAllo::EnableChangeSpace(BOOL bEnable)
{
	m_bEnableChangeSpace = bEnable;
}

