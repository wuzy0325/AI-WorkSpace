#pragma once
#include "stdafx.h"


// 自定义消息
enum UserWndMessages
{	
	UM_SPACE_CHANGE = (WM_USER + 0x100 + 1),  // 通道的内存空间变化 (WPARAM 通道空间的首通道)
	UM_CHANNEL_CHANGE  // 当前通道变化	(WPARAM 新的当前通道	LPARAM 旧的当前通道)
};

// CSpaceAllo

class CSpaceAllo : public CWnd
{
	DECLARE_DYNAMIC(CSpaceAllo)

	static COLORREF m_clr[]; // 默认区域颜色
public:
	
	COLORREF m_clrBK;
	CSpaceAllo();
	virtual ~CSpaceAllo();
	virtual BOOL Create(LPCTSTR lpszWindowName, DWORD dwStyle, const RECT& rect, CWnd* pParentWnd, UINT nID);

public:
	BOOL SetSpaceCount(int iNewCount); // 
	BOOL GetSpaceCount(int* iNewCount); // 

	BOOL SetTotalSpace(int iSpace);
	BOOL GetTotalSpace(int* iSpace);

	BOOL SetSpace(int iChannel, int iNewSpace);			// 
	BOOL GetSpace(int iChannel, int* iNewSpace);		// 

	BOOL SetUsedSpace(int iChannel, int iNewSpace);		//  设置指定段使用的空间
	BOOL GetUsedSpace(int iChannel, int* piNewSpace);	//  得到指定段使用的空间
	BOOL GetSpaceFree(int iChannel, int* piFreeSpace);  //	得到可用空间
	BOOL AllocSpace(int iChannel, int iNewSpace);		//  增加空间分配

	BOOL GetCurChannel(int* iChannel);
	BOOL SetCurChannel(int iChannel);
	BOOL SetTxt(int iChannel, CString strTxt);			// 
	BOOL GetTxt(int iChannel, CString& strTxt);			// 

	BOOL SetBkClr(int iChannel, COLORREF clr);			// 
	BOOL GetBkClr(int iChannel, COLORREF& clr);			// 

	BOOL SetUsedClr(int iChannel, COLORREF clr);		// 
	BOOL GetUsedClr(int iChannel, COLORREF& clr);		// 

	void  EnableChangeSpace(BOOL bEnable = TRUE);

protected:
	int m_nSpaceCount;					// 区域个数
	int m_iSelSpace;                  // 当前位置  有下压效果
	int m_nTotalSpace;				// 总的可用空间
	int m_iMoveIndex; 	
	BOOL m_bMoving;
	BOOL m_bEnableChangeSpace;

	CPoint m_Ptold;
	CPoint m_PtBegin;
	CPoint m_PtEnd;

	HCURSOR m_hCurCurrent;
	HCURSOR m_hCursorSizeWe;
	HCURSOR m_hcurDefault;

	CRect m_rcClient;

	// 内存绘制有关的
	CDC      m_dcMem;
	CBitmap  m_bmp;
	CBitmap* m_pOldBmp;
	class CSpace
	{
	public:
		CSpace();
		virtual ~CSpace();
		CRect m_rc;				
		CRect m_rcUsed;		// 占用的空间部分的巨型
		COLORREF m_clr;
		COLORREF m_clrUsed;  // 占用部分的背景
		CString m_strTxt;
		int	m_iSpace;       // 总的空间
		int	m_iUsedSpace;   // 占用空间
	};
	CArray<CSpace, CSpace&> m_arraySpace;	// 每个区域占据的空间

	virtual void OnDraw(CDC* pDC);
	void  CalculateRect();   // 计算各个区域的大小

protected:
	DECLARE_MESSAGE_MAP()
public:
	afx_msg void OnPaint();
	afx_msg void OnSize(UINT nType, int cx, int cy);
	afx_msg void OnLButtonDblClk(UINT nFlags, CPoint point);
	afx_msg void OnLButtonDown(UINT nFlags, CPoint point);
	afx_msg void OnLButtonUp(UINT nFlags, CPoint point);
	afx_msg void OnMouseMove(UINT nFlags, CPoint point);
	afx_msg BOOL OnSetCursor(CWnd* pWnd, UINT nHitTest, UINT message);
};


