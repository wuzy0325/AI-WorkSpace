// ADView.h : interface of the CADView class
//
/////////////////////////////////////////////////////////////////////////////

#if !defined(AFX_ADVIEW_H__890AFD7C_4E65_4B40_B646_D1E42C19FBC2__INCLUDED_)
#define AFX_ADVIEW_H__890AFD7C_4E65_4B40_B646_D1E42C19FBC2__INCLUDED_
	// Added by ClassView
#if _MSC_VER > 1000
#pragma once
#endif // _MSC_VER > 1000

#include "PageDIO.h"
#include "PageHardLimit.h"
#include "PageInterpolation.h"
#include "PageLine.h"
#include "PageSoftLimit.h"
#include "ComSetPage.h"
#include "PageInterApp.h"
#include "PageSynchron.h"
#include "SynchronPageSheet.h"
#include "PageSynchronSet.h"
class CADView : public CFormView
{
protected: // create from serialization only
	CADView();
	DECLARE_DYNCREATE(CADView)
public:
	CBitmap            m_bmRate;
	CDC                m_memDCRate;
	COLORREF m_Color[4];                         
	CPageLine          m_PageLine;               // 线性运动页
	CPageInterpolation m_PageInterpolation;      // 插补运动页
	CPageInterApp      m_PageInterApp;           // 插补应用页
	CPageDIO           m_PageDIO;                // 开关量测试页
	CPageHardLimit     m_PageHardLimit;			 // 硬件限位页
	CPageSoftLimit     m_PageSoftLimit;          // 软件限位页
	CPageComSet        m_PageComSet;             // 公用设置页
	CPageSynchronSet   m_PageSynchronSet;        // 同步运动页
	BOOL               m_bSynchronEnable;        // 是否使用同步
	ULONG					m_lPulseModeIN[4];	 
	WTNMC4A_PARA_DataList   m_DataList[4];       // 公用参数 
	WTNMC4A_PARA_LCData     m_LCData[4];         // 直线和S曲线参数
	OTHERPARA               m_OtherPara[4];      // 硬件限位和软件限位的参数
	WTNMC4A_PARA_ExpMode    m_FilterPara[4];     // 滤波器设置参数
	WTNMC4A_PARA_LineData   m_LineData;          // 直线插补和固定线速度直线插补参数
	WTNMC4A_PARA_CircleData m_CircleData;        // 正反方向圆弧插补参数
	WTNMC4A_PARA_InterpolationAxis m_InterpAxis; // 插补轴(nAxis1, nAxis2, nAxis3)
	WTNMC4A_PARA_SynchronActionOwnAxis   m_SynchronActionOwnAxis[4];   // 设置当前轴同步参数
	WTNMC4A_PARA_SynchronActionOtherAxis m_SynchronActionOtherAxis[4]; // 设置与当前轴同步的参数
	int m_nLPEP[4];                 // 同步运动时选择逻辑计数器/实位计数器与COMP+/COMP-计数器比较
	int m_nSynchronCOMPPValue[4];   // COMP+的值
	int m_nSynchronCOMPNValue[4];   // COMP-的值
	WTNMC4A_PARA_AutoHomeSearch  m_HomeSearchPara[4]; // 原点搜寻的参数
	int m_nHomeLowSpeed;            // 低速原点搜寻速度
	int m_nInData[4][3];            // WR寄存器的有效电平(默认为高电平有效)
	int m_nFunction;                // 功能号(直线插补方式|圆弧插补)
	ULONG m_nRV[4];                   // 当前速度
	ULONG m_nLPV[4];                // 逻辑计数器计数
//	BOOL m_bHomeSearchEnable;            
	BOOL m_bAllAxisRun;             // 四轴同时启动的标志
	WTNMC4A_PARA_DO m_ParaDO[4];    // 四个轴的开关量输出
	WTNMC4A_PARA_RR3  m_ParaRR3;    // 开关量输入
	WTNMC4A_PARA_RR4  m_ParaRR4;    // 开关量输入
	LONG    m_StatusGeneralOut[4];  // 开关量输出选择(通用输出|状态输出)
	WTNMC4A_PARA_RR0 m_RR0;
	WTNMC4A_PARA_RR1 m_RR1[4];
	WTNMC4A_PARA_RR2 m_RR2[4];
	WTNMC4A_PARA_RR3 m_RR3;
	WTNMC4A_PARA_RR4 m_RR4;
	WTNMC4A_PARA_RR5 m_RR5;
	WTNMC4A_PARA_Interrupt m_Interrupt[4];  // 中断位参数
	UINT m_nCurrentAxis;					// 参数的当前索引
	HANDLE m_hDevice;						// 设备句柄
	BOOL m_bInstStop;						// 是否立即停止
	BOOL m_bSLimit[4];						// 是否设置软件限位
	BOOL m_bHLimit[4];						// 是否设置硬件限位
	BOOL m_bAlarm[4];						// 是否设置报警信号有效
	BOOL m_bStopNum[4][4];						// 是否设置停止信号有效
	BOOL m_bInPos[4];						// 是否设置到位信号有效
	int m_pLineBuffer[4][8192];			    // X、Y轴绘制速度曲线图的点
	LONG* m_pXPulse;                        // X轴脉冲数
	LONG* m_pYPulse;		                // Y轴脉冲数
	int m_nCount[4];                        // 绘制速度曲线图的点的个数
	int m_nIntCount;                        // 插补时绘制位移图的点的个数
	int m_nStartBitMode;                    // 位插补方式(心形或六边形)
	BOOL m_bAxisRun[4];                     // 四个轴的启停状态
	USHORT m_nStopNum;						// 外部停止信号
	BOOL   m_nStopNumSts[4];				// 外部停止信号设置状态
	//{{AFX_DATA(CADView)
	enum { IDD = IDD_SYS_FORM };
	CTabCtrl	m_TabFunction; // 功能切换属性页
	CTabCtrl	m_TabComSet;   // 公用设置属性页
	CTabCtrl	m_TabLimitSet; // 软|硬件限位属性页
	CButton	m_Static_FuncSet;
	CBitmapButton	m_ZZZTY;
	CBitmapButton	m_ZZZTX;
	CBitmapButton	m_ZYXWY;
	CBitmapButton	m_ZYXWX;
	CBitmapButton	m_ZXDDY;
	CBitmapButton	m_ZXDDX;
	CBitmapButton	m_ZRXWY;
	CBitmapButton	m_ZRXWX;
	CBitmapButton	m_JSZTY;
	CBitmapButton	m_JSZTX;
	CBitmapButton	m_JJZDY;
	CBitmapButton	m_JJZDX;
	CBitmapButton	m_JDSZTY;
	CBitmapButton	m_JDSZTX;
	CBitmapButton	m_FYXWY;
	CBitmapButton	m_FYXWX;
	CBitmapButton	m_FXDDY;
	CBitmapButton	m_FXDDX;
	CBitmapButton	m_FRXWY;
	CBitmapButton	m_FRXWX;
	CBitmapButton	m_CSZTY;
	CBitmapButton	m_CSZTX;
	CBitmapButton	m_DWZTY;
	CBitmapButton	m_DWZTX;
	CBitmapButton	m_BJXHY;
	CBitmapButton	m_BJXHX;
	CBitmapButton	m_BJXHZ;
	CBitmapButton	m_BJXHU;
	CBitmapButton	m_ZZZTZ;
	CBitmapButton	m_ZZZTU;
	CBitmapButton	m_ZYXWZ;
	CBitmapButton	m_ZYXWU;
	CBitmapButton	m_ZXDDZ;
	CBitmapButton	m_ZXDDU;
	CBitmapButton	m_ZRXWZ;
	CBitmapButton	m_ZRXWU;
	CBitmapButton	m_JSZTZ;
	CBitmapButton	m_JSZTU;
	CBitmapButton	m_JJZDZ;
	CBitmapButton	m_JJZDU;
	CBitmapButton	m_JDSZTZ;
	CBitmapButton	m_JDSZTU;
	CBitmapButton	m_FYXWZ;
	CBitmapButton	m_FYXWU;
	CBitmapButton	m_FXDDZ;
	CBitmapButton	m_FXDDU;
	CBitmapButton	m_FRXWZ;
	CBitmapButton	m_FRXWU;
	CBitmapButton	m_DWZTZ;
	CBitmapButton	m_DWZTU;
	CBitmapButton	m_CSZTZ;
	CBitmapButton	m_CSZTU;
	//}}AFX_DATA

// Attributes
public:
	//HY
	BOOL m_bDrawSingleStep;
	
// Operations
public:

// Overrides
	// ClassWizard generated virtual function overrides
	//{{AFX_VIRTUAL(CADView)
	public:
	protected:
	virtual void DoDataExchange(CDataExchange* pDX);    // DDX/DDV support
	virtual void OnInitialUpdate(); // called first time after construct
	//}}AFX_VIRTUAL

// Implementation
public:
	void ResetDevice(HANDLE hDevice);
	void SetInterrruptBit(int nAxis);
	void StartSingleStepInterpMovement();
	void StartSynchronMovement(int nStartAxis);
	void StartAutoHomeSearch(int nAxisNum);
	void SetHomeSearchWnd();
	void GetSwitchDI();
	void SetSwitchOut(int nAxisNum);
	void SetSynchronPara(int nAxisNum);
	void StartINTBitInterpMovement();
	void StartBitInterpMovement();
	void StartSequenceMovement();
	void StartCircleInterpMovement(int nDirection);
	void StartLineInterpMovement(int nAxisCount);
	void OutStart(int nAixsNum);
	void EnableWindows(BOOL bEnable);
	void DecStop(int nAxisNum);
	void RefreshStatusU();
	void RefreshStatusZ();
	void ImmediateStop(int nAxisNum);
	void StartLineMovement(int nAxisNum);
	void SavePara(PWTNMC4A_PARA_DataList pDataList2, 
						PWTNMC4A_PARA_LCData   pLCData2,
						PWTNMC4A_PARA_LineData pLineData,
						PWTNMC4A_PARA_CircleData pCircleData,
						POTHERPARA pOtherPara2);
	void LoadPara(PWTNMC4A_PARA_DataList pDataList2,
						PWTNMC4A_PARA_LCData   pLCData2,
						PWTNMC4A_PARA_LineData pLineData,
						PWTNMC4A_PARA_CircleData pCircleData,
						POTHERPARA pOtherPara2);
	LRESULT OnDrawBit(WPARAM wPara, LPARAM lPara);
	LRESULT OnDrawSequence(WPARAM wPara, LPARAM lPara);
	void ReadLineData(int Axis);
	void ReadInterpolationData();
	void RefreshStatusX();
	void RefreshStatusY();

	LRESULT OnDrawRate(WPARAM wPara, LPARAM lPara);
	LRESULT OnDrawLineInterpolation(WPARAM wPara, LPARAM lPara);
	LRESULT OnDrawCircle(WPARAM wPara, LPARAM lPara);

	void LineArrow(CDC* pDC, CPoint P1, CPoint P2, double theta, int length);
	CString GetAxisString(int nAxisNum);
	virtual ~CADView();

#ifdef _DEBUG
	virtual void AssertValid() const;
	virtual void Dump(CDumpContext& dc) const;
#endif

protected:

// Generated message map functions
protected:
	void ClearZeroStatus(int nAxisNum);
	//{{AFX_MSG(CADView)
	afx_msg void OnClose();
	afx_msg void OnRadioFact();
	afx_msg void OnTimer(UINT_PTR nIDEvent);
	afx_msg void OnClearCounter();
	afx_msg void OnSetSlimit();
	afx_msg void OnSetHLimit();
	afx_msg void OnSetStopNum();
	afx_msg void OnClearInPos();
	afx_msg void OnPaint();
	afx_msg void OnSelchangeTabFuncton(NMHDR* pNMHDR, LRESULT* pResult);
	afx_msg void OnSelchangeTabLimitSet(NMHDR* pNMHDR, LRESULT* pResult);
	afx_msg int OnCreate(LPCREATESTRUCT lpCreateStruct);
	afx_msg void OnBUTTONReset();
	afx_msg void OnSelchangeTABComSet(NMHDR* pNMHDR, LRESULT* pResult);
	//}}AFX_MSG
	DECLARE_MESSAGE_MAP()
};

#ifndef _DEBUG  // debug version in ADView.cpp

#endif

/////////////////////////////////////////////////////////////////////////////

//{{AFX_INSERT_LOCATION}}
// Microsoft Visual C++ will insert additional declarations immediately before the previous line.

#endif // !defined(AFX_ADVIEW_H__890AFD7C_4E65_4B40_B646_D1E42C19FBC2__INCLUDED_)
