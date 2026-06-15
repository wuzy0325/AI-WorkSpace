// MdiClient.cpp : implementation file
//
/////////////////////////////////////////////////////////////////////////////
// This class does subclass the MDI-CLIENT window.
// Subclassing means that all messages are first routed to this class, then 
// to the original window (in this case the MDI-CLIENT).
// We need this to get notifications of the creation and deletion of the 
// MDI child frames (contain views).
/////////////////////////////////////////////////////////////////////////////
//
// Copyright © 1998 Written by Dieter Fauth 
//		mailto:fauthd@zvw.de 
//  
// This code may be used in compiled form in any way you desire. This    
// file may be redistributed unmodified by any means PROVIDING it is     
// not sold for profit without the authors written consent, and     
// providing that this notice and the authors name and all copyright     
// notices remains intact. If the source code in this file is used in     
// any  commercial application then a statement along the lines of     
// "Portions Copyright © 1999 Dieter Fauth" must be included in     
// the startup banner, "About" box or printed documentation. An email     
// letting me know that you are using it would be nice as well. That's     
// not much to ask considering the amount of work that went into this.    
//    
// This file is provided "as is" with no expressed or implied warranty.    
// The author accepts no liability for any damage/loss of business that    
// this product may cause.    
//  
// ==========================================================================  
// HISTORY:	  
// ==========================================================================  
//			1.00	08 May 1999	- Initial release.  
// ==========================================================================  
//  
/////////////////////////////////////////////////////////////////////////////

#include "stdafx.h"
#include "TabCtrlBarDoc.h"
#include "MdiClient.h"

#ifdef _DEBUG
#define new DEBUG_NEW
#undef THIS_FILE
static char THIS_FILE[] = __FILE__;
#endif

/////////////////////////////////////////////////////////////////////////////
// CMdiClient

CMdiClient::CMdiClient(): m_sizeClient(0, 0)
{
	m_crBkColor = GetSysColor(COLOR_DESKTOP);
	m_pWndTabs = NULL;
}


CMdiClient::~CMdiClient()
{
}

BEGIN_MESSAGE_MAP(CMdiClient, CWnd)
	//{{AFX_MSG_MAP(CMdiClient)
	ON_WM_ERASEBKGND()
	ON_WM_SIZE()
	ON_MESSAGE(WM_MDICREATE, OnMDICreate)
	ON_MESSAGE(WM_MDIDESTROY, OnMDIDestroy)
	//}}AFX_MSG_MAP
END_MESSAGE_MAP()

/////////////////////////////////////////////////////////////////////////////
void CMdiClient::AddHandle(HWND hWnd)
{
	ASSERT(m_pWndTabs != NULL);
	// Ìí¼ÓÁĞ±í
	m_pWndTabs->AddHandle(hWnd);
}

void CMdiClient::RemoveHandle(HWND hWnd)
{
	ASSERT(m_pWndTabs != NULL);
	m_pWndTabs->RemoveHandle(hWnd);
}

/////////////////////////////////////////////////////////////////////////////
// CMdiClient message handlers

LRESULT CMdiClient::OnMDICreate(WPARAM wParam, LPARAM lParam)
{
	HWND hWnd = (HWND) DefWindowProc(WM_MDICREATE,  wParam, lParam);
	AddHandle(hWnd);
	return (LRESULT) hWnd;
}

LRESULT CMdiClient::OnMDIDestroy(WPARAM wParam, LPARAM lParam)
{
	RemoveHandle((HWND) wParam);
	return DefWindowProc(WM_MDIDESTROY,  wParam, lParam);
}

//////////////////////////////////////////////////////////////////////////
// µ±¸Ä±ä´°¿Ú´óĞ¡»ò±»ÒÆ¶¯»òÊ×´Î´´½¨Ê±£¬´Ëº¯Êı±»µ÷ÓÃ£¬Ëü´´½¨´°¿ÚµÄ±³¾°
BOOL CMdiClient::OnEraseBkgnd(CDC* pDC) 
{
	// Èç¹û±³¾°Ç°É«²»Îª0, m_crBkColorÔÚ¹¹Ôìº¯ÊıÖĞ³õÊ¼»¯
	if (m_crBkColor != 0)
	{
		CBrush NewBrush(m_crBkColor); 
		pDC->SetBrushOrg(0, 0);// ÉèÖÃË¢×ÓµÄÆğÊ¼Ô­µã
		CBrush* pOldBrush = (CBrush*)(pDC->SelectObject(&NewBrush)); // Ñ¡ÔñĞÂË¢×Ó£¬ÇÒ½«Ô­Ë¢×Ó×ª»»³ÉCBrushÀà 
		CRect rect;
		GetClientRect(&rect);// È¡µÃÖ÷´°¿Ú¿Í»§Çø¾ØĞÎ
		pDC->PatBlt(rect.left, rect.top, rect.Width(), rect.Height(), PATCOPY);// ÒÔ¿½±´·½Ê½»æÖÆ´°¿Ú±³¾°
		pDC->SelectObject(pOldBrush);
		NewBrush.DeleteObject();// É¾³ıĞÂË¢×Ó¶ÔÏó
	}

	CString strLogo = "°¢¶ûÌ©¿Æ¼¼";

	TEXTMETRIC tm;
	
	// ½«µÚÒ»¸ö32Î»²ÎÊıÓëµÚ¶ş¸ö32Î»²ÎÊıÏà³Ë£¬ÔÙ³ıÒÔµÚÈı¸ö32Î»Êı£¬×îºóÈ¡ÉÌ
	int fontSize = -MulDiv(18, pDC->GetDeviceCaps(LOGPIXELSY), 72);
	
	// ´´½¨×ÖÌå	
	CFont fontLogo;
	fontLogo.CreateFont(fontSize, 0, 0, 0, FW_BOLD, FALSE, FALSE, FALSE,
		ANSI_CHARSET, OUT_DEFAULT_PRECIS, CLIP_DEFAULT_PRECIS, DEFAULT_QUALITY, 
		FIXED_PITCH | FF_ROMAN, _T("Times New Roman"));

	pDC->SetBkMode(OPAQUE);// ÉèÖÃ±³¾°Ä£Ê½Îª²»Í¸Ã÷·½Ê½£¨OPAQUE£©

	CFont* oldFont = pDC->SelectObject(&fontLogo);// Ñ¡ÔñĞÂ×ÖÌå
	CRect st(0, 0, 0, 0);
	CSize sz = pDC->GetTextExtent(strLogo, strLogo.GetLength());// È¡µÃ¸ø¶¨×Ö·û´®strLogoµÄ¾ØĞÎÇøÓò
	pDC->GetTextMetrics(&tm);// ¸ù¾İµ±Ç°×ÖÌå£¬»ñµÃËùÑ¡×ÖÌåµÄ¶ÈÁ¿

	// Calculate the box size by subtracting the text width and height from the
	// window size.  Also subtract 20% of the average character size to keep the
	// logo from printing into the borders...

	// È¡µÃ¿Í»§Çø
    CRect rcDataBox;
	GetClientRect(&rcDataBox);
	
	rcDataBox.left = rcDataBox.right  - sz.cx - tm.tmAveCharWidth;
	rcDataBox.top  = rcDataBox.bottom - sz.cy - st.bottom - tm.tmHeight;
	
	CRect rcSave = rcDataBox;		
	
	pDC->SetBkMode(TRANSPARENT); // ÉèÖÃ±³¾°Ä£Ê½ÎªÍ¸Ã÷·½Ê½£¨TRANSPARENT£©
	rcSave = rcDataBox;
	
	// shift logo box right, and print black...
	rcDataBox.left   += tm.tmAveCharWidth / 5;
	
	COLORREF oldColor = pDC->SetTextColor(RGB(0, 0, 0));// ÉèÖÃÎÄ×ÖÉ«£¨ºÚÉ«£©
	
	pDC->DrawText(strLogo, strLogo.GetLength(), &rcDataBox, 
		DT_VCENTER | DT_SINGLELINE | DT_CENTER);// »æÖÆÁ¢Ìå×ÖµÄµ×²ãÎÄ×Ö
	
	rcDataBox = rcSave;
	rcDataBox.left -= tm.tmAveCharWidth /4;// Ïò×óÆ«ÒÆÎÄÌåÎ»ÖÃÎªµ±Ç°×ÖÌå¿í¶ÈµÄ25%
	
	pDC->SetTextColor(RGB(255, 255, 255));// ÉèÖÃÁ¢Ìå×ÖµÄÖĞ²ã×ÖÌåÉ«Îª°×É«
	
	pDC->DrawText(strLogo, strLogo.GetLength(), &rcDataBox, 
		DT_VCENTER | DT_SINGLELINE | DT_CENTER);// »æÖÆÖĞ²ã×Ö
	
	rcDataBox = rcSave;
	
	pDC->SetTextColor(GetSysColor(COLOR_BTNFACE));// ÉèÖÃÎÄ×ÖÎªÏµÍ³É«
	// ÔÚÔ­Î»ÖÃ»æÖÆÉÏ²ãÎÄ×Ö
	pDC->DrawText(strLogo, strLogo.GetLength(), &rcDataBox, 
		DT_VCENTER | DT_SINGLELINE | DT_CENTER);
	
	// restore the original properties and release resources...
	pDC->SelectObject(oldFont);
	pDC->SetTextColor(oldColor);   
	pDC->SetBkMode(OPAQUE);	
	fontLogo.DeleteObject();
    // ÊÍ·Å¹«¹²DCºÍ´°¿ÚDC£¬Ê¹ÆäËûÓ¦ÓÃ³ÌĞò¿ÉÒÔÊ¹ÓÃ£¬¶ÔÀàDCºÍË½ÓĞDCÎŞÓ°Ïì	
	ReleaseDC(pDC);
	
    return TRUE;	
	//return CMDIFrameWnd::OnEraseBkgnd(pDC);
}

void CMdiClient::OnSize(UINT nType, int cx, int cy) 
{
	// µ±¿Í»§ÇøÓò±ä»¯Ê±£¬±»µ÷ÓÃ¡£¼´µ±×ÓÖ¡´°¿Ú±»×î´ó»¯Ê±£¬Ò²»á²úÉú´ËÏûÏ¢
	CWnd::OnSize(nType, cx, cy);
	
	// Èç¹ûÓ¦ÓÃ³ÌĞòµ±Æô¶¯£¬Ôò±£´æÕâ¸ö´óĞ¡²ÎÊı£¬¼´¿É·µ»Ø
    if ((m_sizeClient.cx == 0) && (m_sizeClient.cy == 0))
	{
        m_sizeClient.cx = cx;
        m_sizeClient.cy = cy;
		
        return;
	}	

    // Èç¹û¿Í»§´°¿Ú´óĞ¡Î´·¢Éú±ä»¯£¬Ôò·µ»Ø
    if ((m_sizeClient.cx == cx) && (m_sizeClient.cy == cy))
    { 
        return;
    }	

	// ±£´æĞÂÖµ
    m_sizeClient.cx = cx;
    m_sizeClient.cy = cy;
	// Ç¿ÖÆÖØ»æ
    RedrawWindow(NULL, NULL, 
        RDW_INVALIDATE | RDW_ERASE | RDW_ERASENOW | RDW_ALLCHILDREN);    
	
    return;                
}