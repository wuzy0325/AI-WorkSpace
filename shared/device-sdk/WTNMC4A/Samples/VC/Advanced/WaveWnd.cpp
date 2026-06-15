// Wave.cpp : implementation file
//

#include "stdafx.h"
#ifdef _DEBUG
#define new DEBUG_NEW
#undef THIS_FILE
static char THIS_FILE[] = __FILE__;
#endif

/////////////////////////////////////////////////////////////////////////////
// CWave

CWaveWnd::CWaveWnd()
{

}

CWaveWnd::~CWaveWnd()
{
}


BEGIN_MESSAGE_MAP(CWaveWnd, CWnd)
	//{{AFX_MSG_MAP(CWaveWnd)
	ON_WM_PAINT()
	//}}AFX_MSG_MAP
END_MESSAGE_MAP()


/////////////////////////////////////////////////////////////////////////////

BOOL CWaveWnd::Create(DWORD dwStyle, const RECT& rect, CWnd* pParentWnd, UINT nID) 
{
  static CString className = AfxRegisterWndClass(CS_HREDRAW | CS_VREDRAW);
  return CWnd::CreateEx(WS_EX_CLIENTEDGE | WS_EX_STATICEDGE, 
                          className, NULL, dwStyle, 
                          rect.left, rect.top, rect.right-rect.left, rect.bottom-rect.top,
                          pParentWnd->GetSafeHwnd(), (HMENU)nID);
}

void CWaveWnd::OnPaint() 
{
	CPaintDC dc(this); // device context for painting
	int nIndex = 0, nPictureWidth, nPictureHeight;
	CRect rect;	
	CBrush brush;
	CPen Pen, *oldPen;
	GetClientRect(&rect);
	Pen.CreatePen (PS_SOLID, 1, RGB(255, 255, 0));
	brush.CreateSolidBrush(RGB(0, 0, 0));	
	dc.FillRect(rect, &brush);	
	nPictureWidth = rect.Width(); // 图片屏幕宽度像素数
	nPictureHeight = rect.Height();

	if (!m_bConstant)
	{
		// 输出正弦波
		oldPen = dc.SelectObject(&Pen);
		dc.MoveTo(0, (int)(nPictureHeight*(4096-(WORD)m_pWaveBuffer[nIndex])/4096.0));
		for (nIndex=0; nIndex<nPictureWidth; nIndex++)
		{
			dc.LineTo(nIndex, (int)(nPictureHeight*(4096-(WORD)m_pWaveBuffer[nIndex])/4096.0));
		}

		dc.SelectObject(oldPen);
	}
	else
	{	
		// 输出恒定值
		// DA_LSB_COUNT是编辑框控件的范围,滑动条控件中的滑块位置值转换为屏幕像素位置
		oldPen = dc.SelectObject(&Pen);
		int num = (int)(nPictureHeight * (4096-(WORD)m_pDigitBuffer[0]) / 4096.0);
		dc.MoveTo(0, num);
		dc.LineTo(nPictureWidth, num);
	}
}
