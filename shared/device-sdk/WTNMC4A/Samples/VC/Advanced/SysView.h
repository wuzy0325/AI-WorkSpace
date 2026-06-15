// SysView.h : CSysView 类的接口
//



#pragma once
#include "SysDoc.h"

class CChildFrame;

class CSysView : public CScrollView
{
protected: // 仅从序列化创建
	CSysView();
	DECLARE_DYNCREATE(CSysView)

// 属性
public:
	enum SHOW_MODE
	{
		WAVE_MODE = 1,		// 完整波形显示模式
		SEGMENT_MODE = 2,   // 段显示模式
		ReLoad_MODE = 4    // 回读模式
	};

	UINT m_iShowMode;
	CSysDoc* GetDocument() const;
	void SetScrollSizes(int nMapMode, SIZE sizeTotal,
		const SIZE& sizePage = sizeDefault,
		const SIZE& sizeLine = sizeDefault);
protected:

	CSize m_oldSZ;  // 保存上次的显示区域
	CPoint m_ptLBDown;  // 左键位置
	CString m_strInfo;
	BOOL m_bShowInfo;
	

	CDC m_dcMem;
	CBitmap m_Bmp;  // 位图
	CBitmap* m_pOldBmp; // 位图指针
	CChildFrame* m_pParentFrm;
	CFont m_FontTxt;   // 文字字体

	PSHORT m_pDataBuf;
	PPOINT m_pPtBuf;

	CPen m_penPlay;  // 回访的化笔

	CRect m_rcClient;  // 客户去的区域
	CRect m_rcPolt;    // 波形显示区域
	CRect m_rcSpace;

// 操作
public:

// 重写
	public:
	
	virtual void OnDraw(CDC* pDC);  // 重写以绘制该视图
virtual BOOL PreCreateWindow(CREATESTRUCT& cs);
protected:
	virtual BOOL OnPreparePrinting(CPrintInfo* pInfo);
	virtual void OnBeginPrinting(CDC* pDC, CPrintInfo* pInfo);
	virtual void OnEndPrinting(CDC* pDC, CPrintInfo* pInfo);

// 实现
public:
	virtual ~CSysView();
#ifdef _DEBUG
	virtual void AssertValid() const;
	virtual void Dump(CDumpContext& dc) const;
#endif

protected:
	void PlotBkGnd(CDC *pDC); // 绘制背景
	void DrawWave(CDC* pDC, CRect rc);
// 生成的消息映射函数
protected:
	DECLARE_MESSAGE_MAP()
public:
	virtual void OnInitialUpdate();
	afx_msg BOOL OnEraseBkgnd(CDC* pDC);
	afx_msg void OnSize(UINT nType, int cx, int cy);

	afx_msg void OnHScroll(UINT nSBCode, UINT nPos, CScrollBar* pScrollBar);
	afx_msg void OnVScroll(UINT nSBCode, UINT nPos, CScrollBar* pScrollBar);
	BOOL ShowSegment(int iSegment);
	afx_msg void OnLButtonDown(UINT nFlags, CPoint point);
	afx_msg void OnLButtonDblClk(UINT nFlags, CPoint point);
	afx_msg void OnDestroy();
	afx_msg void OnContextMenu(CWnd* /*pWnd*/, CPoint /*point*/);
	afx_msg void OnReloadData();
	afx_msg void OnUpdateReloadData(CCmdUI *pCmdUI);
	afx_msg void OnShowAllWave();
	afx_msg void OnUpdateShowAllWave(CCmdUI *pCmdUI);

	afx_msg void OnMouseMove(UINT nFlags, CPoint point);

};

#ifndef _DEBUG  // SysView.cpp 的调试版本
inline CSysDoc* CSysView::GetDocument() const
   { return reinterpret_cast<CSysDoc*>(m_pDocument); }
#endif

