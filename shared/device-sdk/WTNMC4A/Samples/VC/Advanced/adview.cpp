// ADView.cpp : implementation of the CADView class
//

#include "stdafx.h"
#include <math.h>
#include <malloc.h>
#include "Sys.h"

#include "ADDoc.h"
#include "ADView.h"

#ifdef _DEBUG
#define new DEBUG_NEW
#undef THIS_FILE
static char THIS_FILE[] = __FILE__;
#endif

CADView*  gl_pADView;
extern LONG gl_LogLever = 0; // 硬件限位在此程序中默认都为低电平有效
extern BOOL gl_bSequenceRun = TRUE;    // 
extern BOOL gl_bBitProc = TRUE;
extern BOOL gl_bBitDataProc = TRUE;


extern HANDLE gl_hDevice;
extern BOOL  gl_bInstStop;
CWinThread* pStartSequenceThread;
CWinThread* pStartBitThread;
CWinThread* pStartIntWaitThread[4];
HANDLE      gl_hEventInt[4];

extern BOOL gl_hExitEvent;
BOOL gl_bStop = FALSE;
BOOL gl_bBitStop = FALSE;
BOOL gl_bSequenceStop = FALSE;

#define TEXT_COLOR RGB(238,238,238)
#define BK_COLOR   RGB(92,92,72)
UINT StartINTWaitX(PVOID hWnd);  // 线程函数
BOOL gl_bWaitInt = FALSE;        // 正在等待中断到来
// USHORT nBitData[24] = {   // 位插补的十六进制数据(六边形)
// 		0x0000, 0xFFFF, 0x0000, 0x0000, 
// 		0x0000, 0xFFFF, 0xFFFF, 0x0000,
// 		0xFFFF, 0x0000, 0xFFFF, 0x0000,
// 	 	0xFFFF, 0x0000, 0x0000, 0x0000,		
// 		0xFFFF, 0x0000, 0x0000, 0xFFFF,
// 		0x0000, 0xFFFF, 0x0000, 0xFFFF,
//	};
/////////////////////////////////////////////////////////////////////////////////////
IMPLEMENT_DYNCREATE(CADView, CFormView)

BEGIN_MESSAGE_MAP(CADView, CFormView)
//{{AFX_MSG_MAP(CADView)
ON_WM_CLOSE()
ON_BN_CLICKED(IDC_RADIO_FACT, OnRadioFact)
ON_WM_TIMER()
ON_BN_CLICKED(IDC_CLEAR_COUNTER, OnClearCounter)
ON_BN_CLICKED(IDC_SET_SLIMIT, OnSetSlimit)
ON_BN_CLICKED(IDC_Set_HLimit, OnSetHLimit)
ON_BN_CLICKED(IDC_SetStopNum, OnSetStopNum)
ON_BN_CLICKED(IDC_ClearInPos, OnClearInPos)
ON_WM_PAINT()
ON_NOTIFY(TCN_SELCHANGE, IDC_TAB1, OnSelchangeTabFuncton)
ON_NOTIFY(TCN_SELCHANGE, IDC_TAB2, OnSelchangeTabLimitSet)
	ON_WM_CREATE()
	ON_BN_CLICKED(IDC_BUTTON_Reset, OnBUTTONReset)
	ON_NOTIFY(TCN_SELCHANGE, IDC_TAB_ComSet, OnSelchangeTABComSet)
	//}}AFX_MSG_MAP
// Standard printing commands
ON_COMMAND(ID_FILE_PRINT, CFormView::OnFilePrint)
ON_COMMAND(ID_FILE_PRINT_DIRECT, CFormView::OnFilePrint)
ON_COMMAND(ID_FILE_PRINT_PREVIEW, CFormView::OnFilePrintPreview)
ON_MESSAGE(WM_DRAW_LINE,OnDrawRate)
ON_MESSAGE(WM_DRAW_LINEINTERPOLATION, OnDrawLineInterpolation)
ON_MESSAGE(WM_DRAW_CIRCLE, OnDrawCircle)
ON_MESSAGE(WM_DRAW_SEQUENCE, OnDrawSequence) 
ON_MESSAGE(WM_DRAW_BIT, OnDrawBit)
END_MESSAGE_MAP()

/////////////////////////////////////////////////////////////////////////////
// CADView construction/destruction

CADView::CADView()
: CFormView(CADView::IDD)
{
	//{{AFX_DATA_INIT(CADView)
	//}}AFX_DATA_INIT
	// TODO: add construction code here
	m_nStartBitMode = 0;
	m_nCurrentAxis = 0;
	m_bInstStop = FALSE;
	m_pXPulse = NULL;
	m_pYPulse = NULL;
	m_nIntCount = 0;
	m_nStopNum = 0;
	int j,i;
	for( i=0; i< 4; i++)
	{
		m_nStopNumSts[i] = FALSE;
		m_bSLimit[i] = FALSE;
		m_bHLimit[i] = FALSE;
		m_bAlarm[i] = FALSE;
		for ( j=0; j<4; j++)
		{
			m_bStopNum[i][j] = FALSE;
		}
		m_bInPos[i] = FALSE;
		m_nCount[i] = 0;
		m_bAxisRun[i] = FALSE;
//		m_bLvChange[i] = TRUE;
		m_nSynchronCOMPPValue[i] = 5000;
		m_nSynchronCOMPNValue[i] = -5000;
		m_nLPEP[i] = WTNMC4A_LOGIC;
		m_FilterPara[i].FE0 = 1;
		m_FilterPara[i].FE1 = 1;
		m_FilterPara[i].FE2 = 1;
		m_FilterPara[i].FE3 = 1;
		m_FilterPara[i].FE4 = 1;
		m_FilterPara[i].FL0 = 0;
		m_FilterPara[i].FL1 = 1;
		m_FilterPara[i].FL2 = 0;
		m_HomeSearchPara[i].ST1E = 1;
		m_HomeSearchPara[i].ST1D = 0;
		m_HomeSearchPara[i].ST2E = 1;
		m_HomeSearchPara[i].ST2D = 0;
		m_HomeSearchPara[i].ST3E = 1;
		m_HomeSearchPara[i].ST3D = 0;
		m_HomeSearchPara[i].ST4E = 1;
		m_HomeSearchPara[i].ST4D = 0;
		m_HomeSearchPara[i].SAND = 1;
		m_HomeSearchPara[i].PCLR = 1;
		m_HomeSearchPara[i].LIMIT = 1;
		m_nInData[i][0] = 1;
		m_nInData[i][1] = 1;
		m_nInData[i][2] = 1;
		m_StatusGeneralOut[i] = WTNMC4A_CONSTAND;

		m_ParaDO[i].OUT0 = 0;
		m_ParaDO[i].OUT1 = 0;
		m_ParaDO[i].OUT2 = 0;
		m_ParaDO[i].OUT3 = 0;
		m_ParaDO[i].OUT4 = 0;
		m_ParaDO[i].OUT5 = 0;
		m_ParaDO[i].OUT6 = 0;
		m_ParaDO[i].OUT7 = 0;
	}

	m_nHomeLowSpeed = 1000;
	m_nInData[i][0] = 1;
	m_nInData[i][1] = 1;
	m_nInData[i][2] = 1;
	m_bSynchronEnable = FALSE;
//	m_bHomeSearchEnable = FALSE;
	memset(m_SynchronActionOwnAxis, 0, sizeof(WTNMC4A_PARA_SynchronActionOwnAxis)*4);
	memset(m_SynchronActionOtherAxis, 0, sizeof(WTNMC4A_PARA_SynchronActionOtherAxis)*4);
	memset(m_Interrupt, 0, sizeof(WTNMC4A_PARA_Interrupt)*4);
	m_Color[0] = RGB(255, 0, 0);
	m_Color[1] = RGB(0, 255, 0);
	m_Color[2] = RGB(255, 255, 0);
	m_Color[3] = RGB(0, 0, 255);
	m_InterpAxis.Axis1 = 0;
	m_InterpAxis.Axis2 = 1;
	m_InterpAxis.Axis3 = 2;
	m_nFunction = 0;
	m_bAllAxisRun = FALSE;
	m_bDrawSingleStep = FALSE;
}

CADView::~CADView()
{
	if (m_pXPulse != NULL)
	{
		free(m_pXPulse);
	}
	if(m_pYPulse != NULL)
	{
		free(m_pYPulse);
	}	
}

void CADView::DoDataExchange(CDataExchange* pDX)
{
	CFormView::DoDataExchange(pDX);
	//{{AFX_DATA_MAP(CADView)
	DDX_Control(pDX, IDC_TAB_ComSet, m_TabComSet);
	DDX_Control(pDX, IDC_STATIC_FuncSet, m_Static_FuncSet);
	DDX_Control(pDX, IDC_TAB2, m_TabLimitSet);
	DDX_Control(pDX, IDC_TAB1, m_TabFunction);
	DDX_Control(pDX, IDC_ZZZTY, m_ZZZTY);
	DDX_Control(pDX, IDC_ZZZTX, m_ZZZTX);
	DDX_Control(pDX, IDC_ZYXWY, m_ZYXWY);
	DDX_Control(pDX, IDC_ZYXWX, m_ZYXWX);
	DDX_Control(pDX, IDC_ZXDDY, m_ZXDDY);
	DDX_Control(pDX, IDC_ZXDDX, m_ZXDDX);
	DDX_Control(pDX, IDC_ZRXWY, m_ZRXWY);
	DDX_Control(pDX, IDC_ZRXWX, m_ZRXWX);
	DDX_Control(pDX, IDC_JSZTY, m_JSZTY);
	DDX_Control(pDX, IDC_JSZTX, m_JSZTX);
	DDX_Control(pDX, IDC_JJZDY, m_JJZDY);
	DDX_Control(pDX, IDC_JJZDX, m_JJZDX);
	DDX_Control(pDX, IDC_JDSZTY, m_JDSZTY);
	DDX_Control(pDX, IDC_JDSZTX, m_JDSZTX);
	DDX_Control(pDX, IDC_FYXWY, m_FYXWY);
	DDX_Control(pDX, IDC_FYXWX, m_FYXWX);
	DDX_Control(pDX, IDC_FXDDY, m_FXDDY);
	DDX_Control(pDX, IDC_FXDDX, m_FXDDX);
	DDX_Control(pDX, IDC_FRXWY, m_FRXWY);
	DDX_Control(pDX, IDC_FRXWX, m_FRXWX);
	DDX_Control(pDX, IDC_CSZTY, m_CSZTY);
	DDX_Control(pDX, IDC_CSZTX, m_CSZTX);
	DDX_Control(pDX, IDC_DWZTY, m_DWZTY);
	DDX_Control(pDX, IDC_DWZTX, m_DWZTX);
	DDX_Control(pDX, IDC_BJXHY, m_BJXHY);
	DDX_Control(pDX, IDC_BJXHX, m_BJXHX);
	DDX_Control(pDX, IDC_BJXHZ, m_BJXHZ);
	DDX_Control(pDX, IDC_BJXHU, m_BJXHU);
	DDX_Control(pDX, IDC_ZZZTZ, m_ZZZTZ);
	DDX_Control(pDX, IDC_ZZZTU, m_ZZZTU);
	DDX_Control(pDX, IDC_ZYXWZ, m_ZYXWZ);
	DDX_Control(pDX, IDC_ZYXWU, m_ZYXWU);
	DDX_Control(pDX, IDC_ZXDDZ, m_ZXDDZ);
	DDX_Control(pDX, IDC_ZXDDU, m_ZXDDU);
	DDX_Control(pDX, IDC_ZRXWZ, m_ZRXWZ);
	DDX_Control(pDX, IDC_ZRXWU, m_ZRXWU);
	DDX_Control(pDX, IDC_JSZTZ, m_JSZTZ);
	DDX_Control(pDX, IDC_JSZTU, m_JSZTU);
	DDX_Control(pDX, IDC_JJZDZ, m_JJZDZ);
	DDX_Control(pDX, IDC_JJZDU, m_JJZDU);
	DDX_Control(pDX, IDC_JDSZTZ, m_JDSZTZ);
	DDX_Control(pDX, IDC_JDSZTU, m_JDSZTU);
	DDX_Control(pDX, IDC_FYXWZ, m_FYXWZ);
	DDX_Control(pDX, IDC_FYXWU, m_FYXWU);
	DDX_Control(pDX, IDC_FXDDZ, m_FXDDZ);
	DDX_Control(pDX, IDC_FXDDU, m_FXDDU);
	DDX_Control(pDX, IDC_FRXWZ, m_FRXWZ);
	DDX_Control(pDX, IDC_FRXWU, m_FRXWU);
	DDX_Control(pDX, IDC_DWZTZ, m_DWZTZ);
	DDX_Control(pDX, IDC_DWZTU, m_DWZTU);
	DDX_Control(pDX, IDC_CSZTZ, m_CSZTZ);
	DDX_Control(pDX, IDC_CSZTU, m_CSZTU); 
	//}}AFX_DATA_MAP
}

void CADView::OnInitialUpdate()
{
	CFormView::OnInitialUpdate();
	ResizeParentToFit();
	((CADDoc*)GetDocument())->m_pView = this;
	gl_pADView = this; 
	// 加载参数
	LoadPara(m_DataList, m_LCData, &m_LineData, &m_CircleData, m_OtherPara);
	// 初始化状态显示控件状态	
	m_ZZZTY.LoadBitmaps(IDB_GRAY);
	m_ZZZTX.LoadBitmaps(IDB_GRAY);
	m_ZYXWY.LoadBitmaps(IDB_GRAY);
	m_ZYXWX.LoadBitmaps(IDB_GRAY);
	m_ZXDDY.LoadBitmaps(IDB_GRAY);
	m_ZXDDX.LoadBitmaps(IDB_GRAY);
	m_ZRXWY.LoadBitmaps(IDB_GRAY);
	m_ZRXWX.LoadBitmaps(IDB_GRAY);
	m_JSZTY.LoadBitmaps(IDB_GRAY);
	m_JSZTX.LoadBitmaps(IDB_GRAY);
	m_JJZDY.LoadBitmaps(IDB_GRAY);
	m_JJZDX.LoadBitmaps(IDB_GRAY);
	m_JDSZTX.LoadBitmaps(IDB_GRAY);
	m_JDSZTY.LoadBitmaps(IDB_GRAY);
	m_FYXWY.LoadBitmaps(IDB_GRAY);
	m_FYXWX.LoadBitmaps(IDB_GRAY);
	m_FXDDY.LoadBitmaps(IDB_GRAY);
	m_FXDDX.LoadBitmaps(IDB_GRAY);
	m_FRXWY.LoadBitmaps(IDB_GRAY);
	m_FRXWX.LoadBitmaps(IDB_GRAY);
	m_CSZTY.LoadBitmaps(IDB_GRAY);
	m_CSZTX.LoadBitmaps(IDB_GRAY);
	m_DWZTY.LoadBitmaps(IDB_GRAY);
	m_DWZTX.LoadBitmaps(IDB_GRAY);
	m_BJXHY.LoadBitmaps(IDB_GRAY);
	m_BJXHX.LoadBitmaps(IDB_GRAY);
	///////////////////////////////////////
	m_ZZZTZ.LoadBitmaps(IDB_GRAY);
	m_ZZZTU.LoadBitmaps(IDB_GRAY);
	m_ZYXWZ.LoadBitmaps(IDB_GRAY);
	m_ZYXWU.LoadBitmaps(IDB_GRAY);
	m_ZXDDZ.LoadBitmaps(IDB_GRAY);
	m_ZXDDU.LoadBitmaps(IDB_GRAY);
	m_ZRXWZ.LoadBitmaps(IDB_GRAY);
	m_ZRXWU.LoadBitmaps(IDB_GRAY);
	m_JSZTZ.LoadBitmaps(IDB_GRAY);
	m_JSZTU.LoadBitmaps(IDB_GRAY);
	m_JJZDZ.LoadBitmaps(IDB_GRAY);
	m_JJZDU.LoadBitmaps(IDB_GRAY);
	m_JDSZTZ.LoadBitmaps(IDB_GRAY);
	m_JDSZTU.LoadBitmaps(IDB_GRAY);
	m_FYXWZ.LoadBitmaps(IDB_GRAY);
	m_FYXWU.LoadBitmaps(IDB_GRAY);
	m_FXDDZ.LoadBitmaps(IDB_GRAY);
	m_FXDDU.LoadBitmaps(IDB_GRAY);
	m_FRXWZ.LoadBitmaps(IDB_GRAY);
	m_FRXWU.LoadBitmaps(IDB_GRAY);
	m_DWZTZ.LoadBitmaps(IDB_GRAY);
	m_DWZTU.LoadBitmaps(IDB_GRAY);
	m_CSZTZ.LoadBitmaps(IDB_GRAY);
	m_CSZTU.LoadBitmaps(IDB_GRAY);
	m_BJXHZ.LoadBitmaps(IDB_GRAY);
	m_BJXHU.LoadBitmaps(IDB_GRAY);
	// 添加功能页
	m_TabFunction.InsertItem(0, L"线性运动");
	m_TabFunction.InsertItem(1, L"同步运动");
	m_TabFunction.InsertItem(2, L"自动原点搜寻");
	m_TabFunction.InsertItem(3, L"插补运动");
	m_TabFunction.InsertItem(4, L"插补应用");
	m_TabFunction.InsertItem(5, L"开关量测试");
	CRect rect;
	//插补应用和形状量属性页-----------------------------------------------------------------
	m_PageInterApp.Create(IDD_Page_InterpApp, this); // 插补应用
	m_PageDIO.Create(IDD_Page_DIO, this);            // 开关量测试
	m_TabFunction.GetWindowRect(rect);
	ScreenToClient(rect);
	rect.DeflateRect(3, 4);
	rect.top += 20;
	m_PageInterApp.MoveWindow(rect);
	m_PageDIO.MoveWindow(rect);     
	//直线和插补属性页--------------------------------------------------------------
	m_PageLine.Create(IDD_Page_Line, this);                   // 直线(S曲线)运动
	m_PageInterpolation.Create(IDD_Page_Interpolation, this); // 插补运动
	GetDlgItem(IDC_STATIC_FuncSet)->GetWindowRect(rect);
	rect.DeflateRect(3, 6);
	rect.OffsetRect(0, 4);
	ScreenToClient(rect);
	m_PageLine.MoveWindow(rect);
	m_PageInterpolation.MoveWindow(rect);
	m_PageLine.ShowWindow(SW_SHOW); 
	//软/硬件限位属性页---------------------------------------------------------------------------
	m_TabLimitSet.InsertItem(0, L"硬件限位");
	m_TabLimitSet.InsertItem(1, L"软件限位");
	m_PageHardLimit.Create(IDD_Page_HardLimit, this); // 硬件限位
	m_PageSoftLimit.Create(IDD_Page_SoftLimit, this); // 软件限位
	m_TabLimitSet.GetWindowRect(rect);

	ScreenToClient(rect);
	rect.DeflateRect(3, 4);
	rect.top += 20;
	m_PageHardLimit.MoveWindow(rect);
	m_PageSoftLimit.MoveWindow(rect);
	m_PageHardLimit.ShowWindow(SW_SHOW);
	//公用设置----------------------------------------------------------------
	m_TabComSet.InsertItem(0, L"X轴");
	m_TabComSet.InsertItem(1, L"Y轴");
	m_TabComSet.InsertItem(2, L"Z轴");
	m_TabComSet.InsertItem(3, L"U轴");
	m_PageComSet.Create(IDD_Page_ComSet, this);
	GetDlgItem(IDC_TAB_ComSet)->GetWindowRect(rect);
	ScreenToClient(rect);
	rect.DeflateRect(3, 4);
	rect.top += 20;
	m_PageComSet.MoveWindow(rect);
	m_PageComSet.ShowWindow(SW_SHOW);
	//----------------------------------------------------------------------------
	CClientDC dc(this);
	CRect rcPicture;
	CStatic* pStatic = (CStatic*)GetDlgItem(IDC_WAVE_VT);
	pStatic->GetClientRect(rcPicture);
	if(!m_memDCRate.GetSafeHdc())
	{
		m_memDCRate.CreateCompatibleDC(&dc);
		m_bmRate.CreateCompatibleBitmap(&dc, rcPicture.Width(), rcPicture.Height());
		m_memDCRate.SelectObject(&m_bmRate);
	}
	OnClearCounter();
}

/////////////////////////////////////////////////////////////////////////////
// CADView diagnostics

#ifdef _DEBUG
void CADView::AssertValid() const
{
	CFormView::AssertValid();
}

void CADView::Dump(CDumpContext& dc) const
{
	CFormView::Dump(dc);
}

/*CADDoc* CADView::GetDocument() // non-debug version is inline
{
ASSERT(m_pDocument->IsKindOf(RUNTIME_CLASS(CADDoc)));
return (CADDoc*)m_pDocument;
}*/
#endif //_DEBUG

/////////////////////////////////////////////////////////////////////////////
// CADView message handlers

//加载保存的参数
void CADView::LoadPara(PWTNMC4A_PARA_DataList pDataList2, 
						PWTNMC4A_PARA_LCData   pLCData2,
						PWTNMC4A_PARA_LineData pLineData,
						PWTNMC4A_PARA_CircleData pCircleData,
						POTHERPARA pOtherPara2)
{
	CSysApp* pApp = (CSysApp*)AfxGetApp();
	CString strSection;
	for(int i=0; i <4; i++)
	{
		strSection.Format(L"ComSetPara[%d]", i);
		// 公用参数
		pDataList2[i].Multiple = pApp->GetProfileInt(strSection, L"倍率", 1);
		pDataList2[i].StartSpeed = pApp->GetProfileInt(strSection, L"初始速度", 100);
		pDataList2[i].Acceleration = pApp->GetProfileInt(strSection, L"加速度", 125);
		pDataList2[i].Deceleration = pApp->GetProfileInt(strSection, L"减速度", 125);
		pDataList2[i].DriveSpeed = pApp->GetProfileInt(strSection, L"驱动速度", 3000);
		pDataList2[i].AccIncRate = pApp->GetProfileInt(strSection, L"加速度变化率", 1000);
		pDataList2[i].DecIncRate = pApp->GetProfileInt(strSection, L"减速度变化率", 1000);
	
		// 直线和S曲线参数
		pLCData2[i].AxisNum = pApp->GetProfileInt(strSection, L"轴号", i+1);
		pLCData2[i].Direction = pApp->GetProfileInt(strSection, L"转动方向", 1);
		pLCData2[i].PulseMode = pApp->GetProfileInt(strSection, L"脉冲方式", 0);
		m_lPulseModeIN[i] = pApp->GetProfileInt(strSection, L"脉冲输入模式", 0);
		pLCData2[i].PLSLogLever = pApp->GetProfileInt(strSection, L"脉冲方向", 0);
		pLCData2[i].DIRLogLever = pApp->GetProfileInt(strSection, L"方向信号逻辑电平", 0);
		pLCData2[i].LV_DV = pApp->GetProfileInt(strSection, L"驱动方式", 1);
		pLCData2[i].DecMode = pApp->GetProfileInt(strSection, L"减速方式", 0);
		pLCData2[i].nPulseNum = pApp->GetProfileInt(strSection, L"定长脉冲数", 10000);
		pLCData2[i].Line_Curve = pApp->GetProfileInt(strSection, L"直线曲线运动", 0);
		
		// 附加的参数
		pOtherPara2[i].AccOffset = pApp->GetProfileInt(strSection, L"加速计数偏移点", 9);
		pOtherPara2[i].HandDecPulse = pApp->GetProfileInt(strSection, L"手动减速点", 10000);
		pOtherPara2[i].LogicFact = pApp->GetProfileInt(strSection, L"逻辑或实际", 0);
		pOtherPara2[i].LowerLimit = pApp->GetProfileInt(strSection, L"上位下限", -10000);
		pOtherPara2[i].UpperLimit = pApp->GetProfileInt(strSection, L"限位上限", 10000);
	}
	strSection = "InterpPara";
	// 直线插补和固定线速度直线插补参数
	pLineData->n1AxisPulseNum = pApp->GetProfileInt(strSection, L"主轴终点脉冲数", 10000);
	pLineData->Line_Curve = pApp->GetProfileInt(strSection, L"运动方式", 1);
	pLineData->ConstantSpeed = pApp->GetProfileInt(strSection, L"直线固定线速度", 1);
	
	pLineData->n2AxisPulseNum = pApp->GetProfileInt(strSection, L"第二轴终点脉冲数", 10000);
	pLineData->n3AxisPulseNum = pApp->GetProfileInt(strSection, L"第三轴终点脉冲数", 125);
	// 正反方向圆弧插补参数
	pCircleData->ConstantSpeed = pApp->GetProfileInt(strSection, L"圆弧固定线速度", 1);
	pCircleData->Center1 = pApp->GetProfileInt(strSection, L"主轴圆心坐标", 10000);
	pCircleData->Center2 = pApp->GetProfileInt(strSection, L"第二轴圆心坐标", 0);
	pCircleData->Pulse1 = pApp->GetProfileInt(strSection, L"主轴终点坐标", 0);
	pCircleData->Pulse2 = pApp->GetProfileInt(strSection, L"第二轴终点坐标", 0);

}
// 保存参数
void CADView::SavePara(PWTNMC4A_PARA_DataList pDataList2, 
						PWTNMC4A_PARA_LCData   pLCData2,
						PWTNMC4A_PARA_LineData pLineData,
						PWTNMC4A_PARA_CircleData pCircleData,
						POTHERPARA pOtherPara2)
{
	CSysApp* pApp = (CSysApp*)AfxGetApp();
    CString str;
	for(int i=0; i <4; i++)
	{
		str.Format(L"ComSetPara[%d]", i);
		// 公用参数
		pApp->WriteProfileInt(str, L"倍率", pDataList2[i].Multiple);
		pApp->WriteProfileInt(str, L"初始速度", pDataList2[i].StartSpeed);
		pApp->WriteProfileInt(str, L"加速度", pDataList2[i].Acceleration);
		pApp->WriteProfileInt(str, L"减速度", pDataList2[i].Deceleration);
		pApp->WriteProfileInt(str, L"驱动速度", pDataList2[i].DriveSpeed);
		pApp->WriteProfileInt(str, L"加速度变化率", pDataList2[i].AccIncRate);
		pApp->WriteProfileInt(str, L"减速度变化率", pDataList2[i].DecIncRate);
		// 直线和S曲线参数
		pApp->WriteProfileInt(str, L"轴号", pLCData2[i].AxisNum);
		pApp->WriteProfileInt(str, L"转动方向", pLCData2[i].Direction);
		pApp->WriteProfileInt(str, L"脉冲方式", pLCData2[i].PulseMode);
		pApp->WriteProfileInt(str, L"脉冲输入模式", m_lPulseModeIN[i]);
		pApp->WriteProfileInt(str, L"脉冲方向", pLCData2[i].PLSLogLever);
		pApp->WriteProfileInt(str, L"方向信号逻辑电平", pLCData2[i].DIRLogLever);
		pApp->WriteProfileInt(str, L"驱动方式", pLCData2[i].LV_DV);
		pApp->WriteProfileInt(str, L"减速方式", pLCData2[i].DecMode);
		pApp->WriteProfileInt(str, L"定长脉冲数", pLCData2[i].nPulseNum);
		pApp->WriteProfileInt(str, L"直线曲线运动", pLCData2[i].Line_Curve);
		// 附加的参数
		pApp->WriteProfileInt(str, L"加速计数偏移点", pOtherPara2[i].AccOffset);
		pApp->WriteProfileInt(str, L"手动减速点", pOtherPara2[i].HandDecPulse);		
		pApp->WriteProfileInt(str, L"逻辑或实际", pOtherPara2[i].LogicFact);
		pApp->WriteProfileInt(str, L"上位下限", pOtherPara2[i].LowerLimit);
		pApp->WriteProfileInt(str, L"限位上限", pOtherPara2[i].UpperLimit);
	}
	str = "InterpPara";
	// 直线插补和固定线速度直线插补参数
	pApp->WriteProfileInt(str, L"主轴终点脉冲数", pLineData->n1AxisPulseNum);
	pApp->WriteProfileInt(str, L"运动方式", pLineData->Line_Curve);
	pApp->WriteProfileInt(str, L"直线固定线速度", pLineData->ConstantSpeed);
	pApp->WriteProfileInt(str, L"第二轴终点脉冲数", pLineData->n2AxisPulseNum);
	pApp->WriteProfileInt(str, L"第三轴终点脉冲数", pLineData->n3AxisPulseNum);
	// 正反方向圆弧插补参数
	pApp->WriteProfileInt(str, L"圆弧固定线速度", pCircleData->ConstantSpeed);
	pApp->WriteProfileInt(str, L"主轴圆心坐标", pCircleData->Center1);
	pApp->WriteProfileInt(str, L"第二轴圆心坐标", pCircleData->Center2);
	pApp->WriteProfileInt(str, L"主轴终点坐标", pCircleData->Pulse1);
	pApp->WriteProfileInt(str, L"第二轴终点坐标", pCircleData->Pulse2);
}

void CADView::OnClose() 
{
	// TODO: Add your message handler code here and/or call default
	KillTimer(1);
	KillTimer(2);
	KillTimer(3);
	CFormView::OnClose();
}

// 设置软件限位的(逻辑|实际限位)
void CADView::OnRadioFact() 
{
	// TODO: Add your control notification handler code here
	m_OtherPara[m_nCurrentAxis].LogicFact = WTNMC4A_FACT;	
}

void CADView::OnTimer(UINT_PTR nIDEvent) 
{
	// TODO: Add your message handler code here and/or call default
	switch(nIDEvent) {
	case 1: // 刷新状态
//		if(m_bAxisRun[0]) 
		{
			RefreshStatusX();
			EnableWindows(FALSE);
		}
//		if(m_bAxisRun[1])
		{
			RefreshStatusY();
			EnableWindows(FALSE);
		}
//		if(m_bAxisRun[2])
		{
			RefreshStatusZ();
			EnableWindows(FALSE);
		}
//		if(m_bAxisRun[3])
		{
			RefreshStatusU();
			EnableWindows(FALSE);
		}	
//		TRACE("Timer 1 RUNX = %d, RUNY = %d, RUNZ = %d, RUNU = %d\n", m_bAxisRun[0], m_bAxisRun[1], m_bAxisRun[2], m_bAxisRun[3]);
		if(!m_bAxisRun[0] && !m_bAxisRun[1] && !m_bAxisRun[2] && !m_bAxisRun[3])
		{
		//	TRACE("Timer 1 Stop\n");
			m_bAllAxisRun = FALSE;
			KillTimer(3);
			KillTimer(1); 
			//Sleep(1000);
			RefreshStatusX(); // 停止定时器后再刷新一下各状态
			RefreshStatusY();
			RefreshStatusZ();
			RefreshStatusU();
			EnableWindows(TRUE);
			Beep(3000, 1);
			ReadLineData(WTNMC4A_XAXIS);
			ReadLineData(WTNMC4A_YAXIS);
			ReadLineData(WTNMC4A_ZAXIS);
			ReadLineData(WTNMC4A_UAXIS);
			SendMessage(WM_DRAW_LINE, 0, 0); // 画速度时间图
		}
		break;
	case 2: // 读取开关量输入
		GetSwitchDI();
		break;
	case 3: // 各种运动
		if(m_bAxisRun[0])
		{
			ReadLineData(WTNMC4A_XAXIS);
			EnableWindows(FALSE);
		}
		if(m_bAxisRun[1])
		{
			ReadLineData(WTNMC4A_YAXIS);
			EnableWindows(FALSE);
		}
		if(m_bAxisRun[2])
		{
			ReadLineData(WTNMC4A_ZAXIS);
			EnableWindows(FALSE);
		}
		if(m_bAxisRun[3])
		{
			ReadLineData(WTNMC4A_UAXIS);
			EnableWindows(FALSE);
		}
		SendMessage(WM_DRAW_LINE, 0, 0); // 画速度时间图	
		if(m_nFunction == 1) // 直线插补
		{
			ReadInterpolationData();
			SendMessage(WM_DRAW_LINEINTERPOLATION, NULL, NULL);
		}
		if(m_nFunction == 2) // 圆弧插补
		{
			ReadInterpolationData();
			SendMessage(WM_DRAW_CIRCLE, NULL, NULL);
		}
		if(m_nFunction == 3) // 连续插补
		{
			ReadInterpolationData();
			SendMessage(WM_DRAW_SEQUENCE, NULL, NULL);	
		}
		if(m_nFunction == 4 || m_nFunction == 5) // 位插补 
		{
			ReadInterpolationData();
			SendMessage(WM_DRAW_BIT, NULL, NULL);
		}
		if (m_bDrawSingleStep)
		{
			KillTimer(3);
			m_bDrawSingleStep = FALSE;
		}
		break;
	}
	
	CFormView::OnTimer(nIDEvent);
}

// 清除计数器
void CADView::OnClearCounter()   
{
	// TODO: Add your control notification handler code here
	WTNMC4A_SetLP(m_hDevice, WTNMC4A_ALLAXIS, 0); // 四轴逻辑位置计数器清零
	WTNMC4A_SetEP(m_hDevice, WTNMC4A_ALLAXIS, 0); // 四轴实位计数器清零

	for(int nAxis=0; nAxis<4; nAxis++)
	{
		if(!m_bAxisRun[nAxis])
			m_nCount[nAxis] = 0;
	}
	RefreshStatusX();
	RefreshStatusY();	
	RefreshStatusZ();
	RefreshStatusU();
}

 
// 刷新X轴状态显示
void CADView::RefreshStatusX()
{
	CString str;
	LONG    nRa;  // 当前加速度
	LONG  nFv;  // 实位计数器值
	LONG  nBr;  // 同步缓冲寄存器
	// 读取X轴当前速度----------------------------------------------------------
	m_nRV[WTNMC4A_XAXIS] = WTNMC4A_ReadCV(m_hDevice, WTNMC4A_XAXIS);  
	m_nRV[WTNMC4A_XAXIS] *= m_DataList[WTNMC4A_XAXIS].Multiple; // 乘以倍率
	str.Format(L"%d", m_nRV[WTNMC4A_XAXIS]);
	CStatic* pRVX = (CStatic*)GetDlgItem(IDC_RVX);
	pRVX->SetWindowText(str);
	// 读取X轴当前加速度--------------------------------------------------------
	nRa = WTNMC4A_ReadCA(m_hDevice, WTNMC4A_XAXIS);  
	nRa = nRa*125*m_DataList[WTNMC4A_XAXIS].Multiple;
	// 读取X轴逻辑计数器---------------------------------------------------------
	m_nLPV[WTNMC4A_XAXIS] = WTNMC4A_ReadLP(m_hDevice, WTNMC4A_XAXIS);	
	str.Format(L"%d", m_nLPV[WTNMC4A_XAXIS]);
	CStatic* pRLX = (CStatic*)GetDlgItem(IDC_RLX);
	pRLX->SetWindowText(str);
	// 读取X轴实位计数器--------------------------------------------------------------
	nFv = WTNMC4A_ReadEP(m_hDevice, WTNMC4A_XAXIS);  
	str.Format(L"%d", nFv);
	CStatic* pRFX = (CStatic*)GetDlgItem(IDC_RFX);
	pRFX->SetWindowText(str);
	// 读取X轴同步缓冲寄存器BR
	nBr = WTNMC4A_ReadBR(m_hDevice, WTNMC4A_XAXIS);
	str.Format(L"%d", nBr);
	CStatic* pBRX = (CStatic*)GetDlgItem(IDC_BRX);
	pBRX->SetWindowText(str);
	//读取寄存器RR1 -------------------------------------------------------
	WTNMC4A_GetRR1Status(m_hDevice, WTNMC4A_XAXIS, &m_RR1[WTNMC4A_XAXIS]);
	// 加速状态
	(m_RR1[WTNMC4A_XAXIS].ASND)?m_JSZTX.LoadBitmaps(IDB_RED):m_JSZTX.LoadBitmaps(IDB_GRAY);
	m_JSZTX.RedrawWindow();
	// 常速状态
	(m_RR1[WTNMC4A_XAXIS].CNST)?m_CSZTX.LoadBitmaps(IDB_RED):m_CSZTX.LoadBitmaps(IDB_GRAY);
	m_CSZTX.RedrawWindow();
	// 减速状态
	if (m_RR1[WTNMC4A_XAXIS].DSND)
	{
		str.Format(L"-%d", nRa);
		m_JDSZTX.LoadBitmaps(IDB_RED);
	}
	else
	{
		str.Format(L"%d", nRa);	
		m_JDSZTX.LoadBitmaps(IDB_GRAY);
	}

//	TRACE("减速状态 = %d\n", m_RR1[WTNMC4A_XAXIS].DSND);

	CStatic* pRAX = (CStatic*)GetDlgItem(IDC_RAX);
	if((!m_bAxisRun[0]) || (m_nFunction==1 && m_InterpAxis.Axis1!=WTNMC4A_XAXIS))
	{
		pRAX->SetWindowText(L"0");  
	}
	else
	{
		pRAX->SetWindowText(str);
	}
	m_JDSZTX.RedrawWindow();
	// 报警状态
	(m_RR1[WTNMC4A_XAXIS].ALARM)?m_BJXHX.LoadBitmaps(IDB_RED):m_BJXHX.LoadBitmaps(IDB_GRAY);
	m_BJXHX.RedrawWindow();
	// 负硬限位
	(m_RR1[WTNMC4A_XAXIS].LMTM)?m_FYXWX.LoadBitmaps(IDB_RED):m_FYXWX.LoadBitmaps(IDB_GRAY);
	m_FYXWX.RedrawWindow();
	// 正硬限位
	(m_RR1[WTNMC4A_XAXIS].LMTP)?m_ZYXWX.LoadBitmaps(IDB_RED):m_ZYXWX.LoadBitmaps(IDB_GRAY);
	m_ZYXWX.RedrawWindow();
	//读紧急制动信号
    (m_RR1[WTNMC4A_XAXIS].EMG)?m_JJZDX.LoadBitmaps(IDB_RED):m_JJZDX.LoadBitmaps(IDB_GRAY);
	m_JJZDX.RedrawWindow();
	//读取寄存器RR0 ------------------------------------------------------------
	WTNMC4A_GetRR0Status(m_hDevice, &m_RR0);
	m_bAxisRun[WTNMC4A_XAXIS] = m_RR0.XDRV;
	m_bAxisRun[WTNMC4A_YAXIS] = m_RR0.YDRV;
	m_bAxisRun[WTNMC4A_ZAXIS] = m_RR0.ZDRV;
	m_bAxisRun[WTNMC4A_UAXIS] = m_RR0.UDRV;
//	TRACE("Run X = %d, Y = %d, Z = %d, U = %d\n", m_RR0.XDRV, m_RR0.YDRV, m_RR0.ZDRV, m_RR0.UDRV);
	if(!m_bAxisRun[WTNMC4A_XAXIS]) //  中止状态
	{
		m_ZZZTX.LoadBitmaps(IDB_RED);
	}
	else
	{
		m_ZZZTX.LoadBitmaps(IDB_GRAY);
	}
	
	//////////////////////////////////////////////////////////////////////////
	// 调试
// 	if (!m_bAllAxisRun)
// 	{
// 		TRACE("X轴中止状态:%d, 常速状态:%d\n", m_RR0.XDRV, m_RR1[WTNMC4A_XAXIS].CNST);
// 	}
	//////////////////////////////////////////////////////////////////////////

	m_ZZZTX.RedrawWindow();
	//读取寄存器RR2 ------------------------------------------------------
	WTNMC4A_GetRR2Status(m_hDevice, WTNMC4A_XAXIS, &m_RR2[WTNMC4A_XAXIS]);
	//读正软限位
	(m_RR2[WTNMC4A_XAXIS].SLMTP)?m_ZRXWX.LoadBitmaps(IDB_RED):m_ZRXWX.LoadBitmaps(IDB_GRAY);
	m_ZRXWX.RedrawWindow();
	//读负软限位
	(m_RR2[WTNMC4A_XAXIS].SLMTM)?m_FRXWX.LoadBitmaps(IDB_RED):m_FRXWX.LoadBitmaps(IDB_GRAY);
	m_FRXWX.RedrawWindow();
//	TRACE("负软限位=%d",m_RR2[WTNMC4A_XAXIS].SLMTM);
	//读取寄存器RR3 ------------------------------------------------------
	WTNMC4A_GetRR3Status(m_hDevice, &m_RR3); // RR3包含X、Y轴的硬件限位状态
	// 正向点动
	(!(m_RR3.XEXPP))?m_ZXDDX.LoadBitmaps(IDB_RED):m_ZXDDX.LoadBitmaps(IDB_GRAY);
	m_ZXDDX.RedrawWindow();
	// 负向点动
	(!(m_RR3.XEXPM))?m_FXDDX.LoadBitmaps(IDB_RED):m_FXDDX.LoadBitmaps(IDB_GRAY);
	m_FXDDX.RedrawWindow();
	// 定位状态
	(!(m_RR3.XINPOS))?m_DWZTX.LoadBitmaps(IDB_RED):m_DWZTX.LoadBitmaps(IDB_GRAY);
	m_DWZTX.RedrawWindow();
}

// 刷新Y轴状态显示
void CADView::RefreshStatusY()
{
	CString str;
	LONG    nRa;  // 当前加速度
	LONG   nFv;  // 实位计数器值
	LONG   nBr;  // 同步缓冲寄存器值

	m_nRV[WTNMC4A_YAXIS] = WTNMC4A_ReadCV(m_hDevice, WTNMC4A_YAXIS); // 读取Y轴当前速度
	m_nRV[WTNMC4A_YAXIS] *= m_DataList[WTNMC4A_YAXIS].Multiple; // 乘以倍率
	str.Format(L"%d", m_nRV[WTNMC4A_YAXIS]);
	CStatic* pRVY = (CStatic*)GetDlgItem(IDC_RVY);
	pRVY->SetWindowText(str);
	 // 读取Y轴当前加速度
	nRa = WTNMC4A_ReadCA(m_hDevice, WTNMC4A_YAXIS);  
	nRa = nRa*125*m_DataList[1].Multiple;
	// 读取Y轴逻辑计数器
	m_nLPV[WTNMC4A_YAXIS] = WTNMC4A_ReadLP(m_hDevice, WTNMC4A_YAXIS);	 
	str.Format(L"%d", m_nLPV[WTNMC4A_YAXIS]);
	CStatic* pRLY = (CStatic*)GetDlgItem(IDC_RLY);
	pRLY->SetWindowText(str);
	// 读取Y轴的实位寄存器
	nFv = WTNMC4A_ReadEP(m_hDevice, WTNMC4A_YAXIS); 
	str.Format(L"%d", nFv);
	//TRACE("Y轴 = %d\n",nFv);
	CStatic* pRFY = (CStatic*)GetDlgItem(IDC_RFY);
	pRFY->SetWindowText(str);
	// 读取Y轴同步缓冲寄存器BR
	nBr = WTNMC4A_ReadBR(m_hDevice, WTNMC4A_YAXIS);
	str.Format(L"%d", nBr);
	CStatic* pBRY = (CStatic*)GetDlgItem(IDC_BRY);
	pBRY->SetWindowText(str);
/////////////////////////////////////////////////////////////////////////////////////
	//读取寄存器RR1 -------------------------------------------------------
	WTNMC4A_GetRR1Status(m_hDevice, WTNMC4A_YAXIS, &m_RR1[WTNMC4A_YAXIS]);
	// 加速状态
	(m_RR1[WTNMC4A_YAXIS].ASND)?m_JSZTY.LoadBitmaps(IDB_RED):m_JSZTY.LoadBitmaps(IDB_GRAY);
	m_JSZTY.RedrawWindow();

	// 常速状态
	(m_RR1[WTNMC4A_YAXIS].CNST)?m_CSZTY.LoadBitmaps(IDB_RED):m_CSZTY.LoadBitmaps(IDB_GRAY);
	m_CSZTY.RedrawWindow();
	// 减速状态
	if (m_RR1[WTNMC4A_YAXIS].DSND)
	{
		str.Format(L"-%d", nRa);
		m_JDSZTY.LoadBitmaps(IDB_RED);
	}
	else
	{
		str.Format(L"%d", nRa);	
		m_JDSZTY.LoadBitmaps(IDB_GRAY);
	}
	CStatic* pRAY = (CStatic*)GetDlgItem(IDC_RAY);
	if((!m_bAxisRun[1]) || (m_nFunction==1 && m_InterpAxis.Axis1!=WTNMC4A_YAXIS))
	{
		pRAY->SetWindowText(L"0");  
	}
	else
	{
		pRAY->SetWindowText(str);
	}
	m_JDSZTY.RedrawWindow();
	// 报警状态
	(m_RR1[WTNMC4A_YAXIS].ALARM)?m_BJXHY.LoadBitmaps(IDB_RED):m_BJXHY.LoadBitmaps(IDB_GRAY);
	m_BJXHY.RedrawWindow();
	// 负硬限位
	(m_RR1[WTNMC4A_YAXIS].LMTM)?m_FYXWY.LoadBitmaps(IDB_RED):m_FYXWY.LoadBitmaps(IDB_GRAY);
	m_FYXWY.RedrawWindow();
	// 正硬限位
	(m_RR1[WTNMC4A_YAXIS].LMTP)?m_ZYXWY.LoadBitmaps(IDB_RED):m_ZYXWY.LoadBitmaps(IDB_GRAY);
	m_ZYXWY.RedrawWindow();
	//读紧急制动信号
    (m_RR1[WTNMC4A_YAXIS].EMG)?m_JJZDY.LoadBitmaps(IDB_RED):m_JJZDY.LoadBitmaps(IDB_GRAY);
	m_JJZDY.RedrawWindow();
	//读取寄存器RR0 ------------------------------------------------------------
	WTNMC4A_GetRR0Status(m_hDevice, &m_RR0);
	m_bAxisRun[WTNMC4A_XAXIS] = m_RR0.XDRV;
	m_bAxisRun[WTNMC4A_YAXIS] = m_RR0.YDRV;
	m_bAxisRun[WTNMC4A_ZAXIS] = m_RR0.ZDRV;
	m_bAxisRun[WTNMC4A_UAXIS] = m_RR0.UDRV;
	if(!m_bAxisRun[WTNMC4A_YAXIS]) //  中止状态
	{
		m_ZZZTY.LoadBitmaps(IDB_RED);
	//	m_bAxisRun[WTNMC4A_YAXIS] = FALSE;
	}
	else
	{
		m_ZZZTY.LoadBitmaps(IDB_GRAY);
	//	m_bAxisRun[WTNMC4A_YAXIS] = TRUE;
	}

	//////////////////////////////////////////////////////////////////////////
	// 调试
// 	if (!m_bAllAxisRun)
// 	{
// 		TRACE("Y轴中止状态:%d, 常速状态:%d\n", m_RR0.YDRV, m_RR1[WTNMC4A_YAXIS].CNST);
// 	}
	//////////////////////////////////////////////////////////////////////////

	m_ZZZTY.RedrawWindow();
	//读取寄存器RR2 ------------------------------------------------------
	WTNMC4A_GetRR2Status(m_hDevice, WTNMC4A_YAXIS, &m_RR2[WTNMC4A_YAXIS]);
	//读正软限位
	(m_RR2[WTNMC4A_YAXIS].SLMTP)?m_ZRXWY.LoadBitmaps(IDB_RED):m_ZRXWY.LoadBitmaps(IDB_GRAY);
	m_ZRXWY.RedrawWindow();
	//读负软限位
	(m_RR2[WTNMC4A_YAXIS].SLMTM)?m_FRXWY.LoadBitmaps(IDB_RED):m_FRXWY.LoadBitmaps(IDB_GRAY);
	m_FRXWY.RedrawWindow();
	//读取寄存器RR3 ------------------------------------------------------
	WTNMC4A_GetRR3Status(m_hDevice, &m_RR3); // RR3包含X、Y轴的硬件限位状态
	// 正向点动
	(!(m_RR3.YEXPP))?m_ZXDDY.LoadBitmaps(IDB_RED):m_ZXDDY.LoadBitmaps(IDB_GRAY);
	m_ZXDDY.RedrawWindow();
	// 负向点动
	(!(m_RR3.YEXPM))?m_FXDDY.LoadBitmaps(IDB_RED):m_FXDDY.LoadBitmaps(IDB_GRAY);
	m_FXDDY.RedrawWindow();
	// 定位状态
	(!(m_RR3.YINPOS))?m_DWZTY.LoadBitmaps(IDB_RED):m_DWZTY.LoadBitmaps(IDB_GRAY);
	m_DWZTY.RedrawWindow();
}

// 刷新Z轴的状态显示
void CADView::RefreshStatusZ()
{
	CString str;
	LONG    nRa;  // 当前加速度
	ULONG  nFv;  // 实位计数器值
	LONG   nBr;  // 同步缓冲寄存器值
	// 读取Z轴当前速度--------------------------------------------------------------------------------------------
	m_nRV[WTNMC4A_ZAXIS] = WTNMC4A_ReadCV(m_hDevice, WTNMC4A_ZAXIS);  
	m_nRV[WTNMC4A_ZAXIS] *= m_DataList[WTNMC4A_ZAXIS].Multiple; // 乘以倍率
	str.Format(L"%d", m_nRV[WTNMC4A_ZAXIS]);
//	TRACE("ZRV = %d\n", m_nRV[WTNMC4A_ZAXIS]);
	CStatic* pRVZ = (CStatic*)GetDlgItem(IDC_RVZ);
	pRVZ->SetWindowText(str);
	// 读取Z轴当前加速度-------------------------------------------------------------------------------------------
	nRa = WTNMC4A_ReadCA(m_hDevice, WTNMC4A_ZAXIS);  
	nRa = nRa*125*m_DataList[1].Multiple;
	// 读取Z轴逻辑计数器--------------------------------------------------------------------------------------------
	m_nLPV[WTNMC4A_ZAXIS] = WTNMC4A_ReadLP(m_hDevice, WTNMC4A_ZAXIS);	
	str.Format(L"%d", m_nLPV[WTNMC4A_ZAXIS]);
	CStatic* pRLZ = (CStatic*)GetDlgItem(IDC_RLZ);
	pRLZ->SetWindowText(str);
	// 读取Z轴实位计数器---------------------------------------------------------------------------------------------
	nFv = WTNMC4A_ReadEP(m_hDevice, WTNMC4A_ZAXIS);  
	str.Format(L"%d", nFv);
	CStatic* pRFZ = (CStatic*)GetDlgItem(IDC_RFZ);
	pRFZ->SetWindowText(str);
	// 读取Z轴同步缓冲寄存器BR
	nBr = WTNMC4A_ReadBR(m_hDevice, WTNMC4A_ZAXIS);
	str.Format(L"%d", nBr);
	CStatic* pBRZ = (CStatic*)GetDlgItem(IDC_BRZ);
	pBRZ->SetWindowText(str);
	//读取寄存器RR1 ------------------------------------------------------------------------------------------------
	WTNMC4A_GetRR1Status(m_hDevice, WTNMC4A_ZAXIS, &m_RR1[WTNMC4A_ZAXIS]);
	// 加速状态
	(m_RR1[WTNMC4A_ZAXIS].ASND)?m_JSZTZ.LoadBitmaps(IDB_RED):m_JSZTZ.LoadBitmaps(IDB_GRAY);
	m_JSZTZ.RedrawWindow();
	// 常速状态
	(m_RR1[WTNMC4A_ZAXIS].CNST)?m_CSZTZ.LoadBitmaps(IDB_RED):m_CSZTZ.LoadBitmaps(IDB_GRAY);
	m_CSZTZ.RedrawWindow();
	// 减速状态
	if (m_RR1[WTNMC4A_ZAXIS].DSND)
	{
		str.Format(L"-%d", nRa);
		m_JDSZTZ.LoadBitmaps(IDB_RED);
	}
	else
	{
		str.Format(L"%d", nRa);	
		m_JDSZTZ.LoadBitmaps(IDB_GRAY);
	}
	CStatic* pRAZ = (CStatic*)GetDlgItem(IDC_RAZ);
	if((!m_bAxisRun[2]) || (m_nFunction==1 && m_InterpAxis.Axis1!=WTNMC4A_ZAXIS))
	{
		pRAZ->SetWindowText(L"0");  
	}
	else
	{
		pRAZ->SetWindowText(str);
	}
	m_JDSZTZ.RedrawWindow();
	// 报警状态
	(m_RR1[WTNMC4A_ZAXIS].ALARM)?m_BJXHZ.LoadBitmaps(IDB_RED):m_BJXHZ.LoadBitmaps(IDB_GRAY);
	m_BJXHZ.RedrawWindow();
	// 负硬限位
	(m_RR1[WTNMC4A_ZAXIS].LMTM)?m_FYXWZ.LoadBitmaps(IDB_RED):m_FYXWZ.LoadBitmaps(IDB_GRAY);
	m_FYXWZ.RedrawWindow();
	// 正硬限位
	(m_RR1[WTNMC4A_ZAXIS].LMTP)?m_ZYXWZ.LoadBitmaps(IDB_RED):m_ZYXWZ.LoadBitmaps(IDB_GRAY);
	m_ZYXWZ.RedrawWindow();
	//读紧急制动信号
    (m_RR1[WTNMC4A_ZAXIS].EMG)?m_JJZDZ.LoadBitmaps(IDB_RED):m_JJZDZ.LoadBitmaps(IDB_GRAY);
	m_JJZDZ.RedrawWindow();
	//读取寄存器RR0 --------------------------------------------------------------------------------------------------
	WTNMC4A_GetRR0Status(m_hDevice, &m_RR0);
	m_bAxisRun[WTNMC4A_XAXIS] = m_RR0.XDRV;
	m_bAxisRun[WTNMC4A_YAXIS] = m_RR0.YDRV;
	m_bAxisRun[WTNMC4A_ZAXIS] = m_RR0.ZDRV;
	m_bAxisRun[WTNMC4A_UAXIS] = m_RR0.UDRV;
	if(!m_bAxisRun[WTNMC4A_ZAXIS]) //  Z轴中止状态
	{
		m_ZZZTZ.LoadBitmaps(IDB_RED);
	//	m_bAxisRun[WTNMC4A_ZAXIS] = FALSE;
	}
	else
	{
		m_ZZZTZ.LoadBitmaps(IDB_GRAY);
	//	m_bAxisRun[WTNMC4A_ZAXIS] = TRUE;
	}
	m_ZZZTZ.RedrawWindow();
	//读取寄存器RR2 ------------------------------------------------------
	WTNMC4A_GetRR2Status(m_hDevice, WTNMC4A_ZAXIS, &m_RR2[WTNMC4A_ZAXIS]);
	//读正软限位
	(m_RR2[WTNMC4A_ZAXIS].SLMTP)?m_ZRXWZ.LoadBitmaps(IDB_RED):m_ZRXWZ.LoadBitmaps(IDB_GRAY);
	m_ZRXWZ.RedrawWindow();
	//读负软限位
	(m_RR2[WTNMC4A_ZAXIS].SLMTM)?m_FRXWZ.LoadBitmaps(IDB_RED):m_FRXWZ.LoadBitmaps(IDB_GRAY);
	m_FRXWZ.RedrawWindow();
	//读取寄存器RR4 ------------------------------------------------------
	WTNMC4A_GetRR4Status(m_hDevice, &m_RR4); // RR3包含X、Y轴的硬件限位状态
	// 正向点动
	(!(m_RR4.ZEXPP))?m_ZXDDZ.LoadBitmaps(IDB_RED):m_ZXDDZ.LoadBitmaps(IDB_GRAY);
	m_ZXDDZ.RedrawWindow();
	// 负向点动
	(!(m_RR4.ZEXPM))?m_FXDDZ.LoadBitmaps(IDB_RED):m_FXDDZ.LoadBitmaps(IDB_GRAY);
	m_FXDDZ.RedrawWindow();
	// 定位状态
	(!(m_RR4.ZINPOS))?m_DWZTZ.LoadBitmaps(IDB_RED):m_DWZTZ.LoadBitmaps(IDB_GRAY);
	m_DWZTZ.RedrawWindow();
}

// 刷新U轴的状态显示
void CADView::RefreshStatusU()
{
	CString str;
	LONG    nRa;  // 当前加速度
	LONG  nFv;  // 实位计数器值
	LONG   nBr;  // 同步缓冲寄存器值
	// 读取U轴当前速度--------------------------------------------------------------------------------------------
	m_nRV[WTNMC4A_UAXIS] = WTNMC4A_ReadCV(m_hDevice, WTNMC4A_UAXIS); 
	m_nRV[WTNMC4A_UAXIS] *= m_DataList[WTNMC4A_UAXIS].Multiple; // 乘以倍率
	str.Format(L"%d", m_nRV[WTNMC4A_UAXIS]);
	CStatic* pRVU = (CStatic*)GetDlgItem(IDC_RVU);
	pRVU->SetWindowText(str);
	// 读取U轴当前加速度------------------------------------------------------------------------------------------
	nRa = WTNMC4A_ReadCA(m_hDevice, WTNMC4A_UAXIS);  
	nRa = nRa*125*m_DataList[1].Multiple;
	 // 读取U轴逻辑计数器------------------------------------------------------------------------------------------
	m_nLPV[WTNMC4A_UAXIS] = WTNMC4A_ReadLP(m_hDevice, WTNMC4A_UAXIS);	
	str.Format(L"%d", m_nLPV[WTNMC4A_UAXIS]);
	CStatic* pRLU = (CStatic*)GetDlgItem(IDC_RLU);
	pRLU->SetWindowText(str);
	// 实位计数器--------------------------------------------------------------------------------------------------
	nFv = WTNMC4A_ReadEP(m_hDevice, WTNMC4A_UAXIS);
	str.Format(L"%d", nFv);
	CStatic* pRFU = (CStatic*)GetDlgItem(IDC_RFU); 
	pRFU->SetWindowText(str);
	// 读取Y轴同步缓冲寄存器BR
	nBr = WTNMC4A_ReadBR(m_hDevice, WTNMC4A_UAXIS);
	str.Format(L"%d", nBr);
	CStatic* pBRU = (CStatic*)GetDlgItem(IDC_BRU);
	pBRU->SetWindowText(str);
	//读取寄存器RR1 ----------------------------------------------------------------------------------------------
	WTNMC4A_GetRR1Status(m_hDevice, WTNMC4A_UAXIS, &m_RR1[WTNMC4A_UAXIS]);
	// 加速状态
	(m_RR1[WTNMC4A_UAXIS].ASND)?m_JSZTU.LoadBitmaps(IDB_RED):m_JSZTU.LoadBitmaps(IDB_GRAY);
	m_JSZTU.RedrawWindow();
	// 常速状态
	(m_RR1[WTNMC4A_UAXIS].CNST)?m_CSZTU.LoadBitmaps(IDB_RED):m_CSZTU.LoadBitmaps(IDB_GRAY);
	m_CSZTU.RedrawWindow();
	// 减速状态
	if (m_RR1[WTNMC4A_UAXIS].DSND)
	{
		str.Format(L"-%d", nRa);
		m_JDSZTU.LoadBitmaps(IDB_RED);
	}
	else
	{
		str.Format(L"%d", nRa);	
		m_JDSZTU.LoadBitmaps(IDB_GRAY);
	}
	CStatic* pRAU = (CStatic*)GetDlgItem(IDC_RAU);
	if((!m_bAxisRun[3]) || (m_nFunction==1 && m_InterpAxis.Axis1!=WTNMC4A_UAXIS))
	{
		pRAU->SetWindowText(L"0");  
	}
	else
	{
		pRAU->SetWindowText(str);
	}
	m_JDSZTU.RedrawWindow();
	// 报警状态
	(m_RR1[WTNMC4A_UAXIS].ALARM)?m_BJXHU.LoadBitmaps(IDB_RED):m_BJXHU.LoadBitmaps(IDB_GRAY);
	m_BJXHU.RedrawWindow();
	// 负硬限位
	(m_RR1[WTNMC4A_UAXIS].LMTM)?m_FYXWU.LoadBitmaps(IDB_RED):m_FYXWU.LoadBitmaps(IDB_GRAY);
	m_FYXWU.RedrawWindow();
	// 正硬限位
	(m_RR1[WTNMC4A_UAXIS].LMTP)?m_ZYXWU.LoadBitmaps(IDB_RED):m_ZYXWU.LoadBitmaps(IDB_GRAY);
	m_ZYXWU.RedrawWindow();
	//读紧急制动信号
    (m_RR1[WTNMC4A_UAXIS].EMG)?m_JJZDU.LoadBitmaps(IDB_RED):m_JJZDU.LoadBitmaps(IDB_GRAY);
	m_JJZDU.RedrawWindow();
	//读取寄存器RR0 ------------------------------------------------------------
	WTNMC4A_GetRR0Status(m_hDevice, &m_RR0);
	m_bAxisRun[WTNMC4A_XAXIS] = m_RR0.XDRV;
	m_bAxisRun[WTNMC4A_YAXIS] = m_RR0.YDRV;
	m_bAxisRun[WTNMC4A_ZAXIS] = m_RR0.ZDRV;
	m_bAxisRun[WTNMC4A_UAXIS] = m_RR0.UDRV;
//	TRACE("RUN = %d\n", m_RR0.YDRV);
	if(!m_bAxisRun[WTNMC4A_UAXIS]) //  U轴中止状态
	{
		m_ZZZTU.LoadBitmaps(IDB_RED);
	}
	else
	{
		m_ZZZTU.LoadBitmaps(IDB_GRAY);
	}
	m_ZZZTU.RedrawWindow();
	//读取寄存器RR2 ------------------------------------------------------
	WTNMC4A_GetRR2Status(m_hDevice, WTNMC4A_UAXIS, &m_RR2[WTNMC4A_UAXIS]);
	//读正软限位
	(m_RR2[WTNMC4A_UAXIS].SLMTP)?m_ZRXWU.LoadBitmaps(IDB_RED):m_ZRXWU.LoadBitmaps(IDB_GRAY);
	m_ZRXWU.RedrawWindow();
	//读负软限位
	(m_RR2[WTNMC4A_UAXIS].SLMTM)?m_FRXWU.LoadBitmaps(IDB_RED):m_FRXWU.LoadBitmaps(IDB_GRAY);
	m_FRXWU.RedrawWindow();
	//读取寄存器RR4 ------------------------------------------------------
	WTNMC4A_GetRR4Status(m_hDevice, &m_RR4); // RR3包含X、Y轴的硬件限位状态
	// 正向点动
	(!(m_RR4.UEXPP))?m_ZXDDU.LoadBitmaps(IDB_RED):m_ZXDDU.LoadBitmaps(IDB_GRAY);
	m_ZXDDU.RedrawWindow();
	// 负向点动
	(!(m_RR4.UEXPM))?m_FXDDU.LoadBitmaps(IDB_RED):m_FXDDU.LoadBitmaps(IDB_GRAY);
	m_FXDDU.RedrawWindow();
	// 定位状态
	(!(m_RR4.UINPOS))?m_DWZTU.LoadBitmaps(IDB_RED):m_DWZTU.LoadBitmaps(IDB_GRAY);
	m_DWZTU.RedrawWindow();
}

// 软件限位
void CADView::OnSetSlimit()   
{	
	// TODO: Add your control notification handler code here
	if(!WTNMC4A_SetPDirSoftwareLimit(m_hDevice,      // 正向限位
		m_LCData[m_nCurrentAxis].AxisNum, 
		m_OtherPara[m_nCurrentAxis].LogicFact, 
		m_OtherPara[m_nCurrentAxis].UpperLimit))
	{
		AfxMessageBox(L"设置正方向软件限位失败！");
		return;
	}
	
	if(!WTNMC4A_SetMDirSoftwareLimit(m_hDevice,      // 负向限位
		m_LCData[m_nCurrentAxis].AxisNum,
		m_OtherPara[m_nCurrentAxis].LogicFact,
		m_OtherPara[m_nCurrentAxis].LowerLimit))
	{
		AfxMessageBox(L"设置反方向软件限位失败！");
		return;
	}
	
    m_bSLimit[m_nCurrentAxis] = TRUE;
	CButton* pSetLimit = (CButton*)GetDlgItem(IDC_SET_SLIMIT);
	pSetLimit->EnableWindow(FALSE);
	
	CButton* pClearLimit = (CButton*)GetDlgItem(IDC_CLEARLIMIT);
	pClearLimit->EnableWindow(TRUE);
	
}

// 硬件限位
void CADView::OnSetHLimit() 
{
	// TODO: Add your control notification handler code here
	CComboBox* pStopType = (CComboBox*)GetDlgItem(IDC_COMBO_StopType);
	
	USHORT nStopMode;
	if(pStopType->GetCurSel() == 0)
	{
		nStopMode = WTNMC4A_SUDDENSTOP;
	}
	else
	{
		nStopMode = WTNMC4A_DECSTOP;
	}
	if(!WTNMC4A_SetPDirLMTEnable(m_hDevice,
		m_LCData[m_nCurrentAxis].AxisNum,
		nStopMode,
		gl_LogLever))
	{
		AfxMessageBox(L"设置硬件限位失败！");
		return;
	}
	if(!WTNMC4A_SetMDirLMTEnable(m_hDevice,
		m_LCData[m_nCurrentAxis].AxisNum,
		nStopMode,
		gl_LogLever))
	{
		AfxMessageBox(L"设置硬件限位失败！");
		return;
	}
}
 
//设置停止号
void CADView::OnSetStopNum() 
{
	// TODO: Add your control notification handler code here
	CComboBox* pStopNum = (CComboBox*)GetDlgItem(IDC_COMBO_StopNum);
	m_nStopNum = pStopNum->GetCurSel();
	if(!WTNMC4A_SetStopEnable(m_hDevice, m_LCData[m_nCurrentAxis].AxisNum, m_nStopNum, gl_LogLever))
	{
		AfxMessageBox(L"设置外部停止号有效失败！");
		return;
	}
	
	CButton* pSetStopNum = (CButton*)GetDlgItem(IDC_SetStopNum);
	pSetStopNum->EnableWindow(FALSE);
	
	CButton* pClearStopNum = (CButton*)GetDlgItem(IDC_Clear_StopNum);
	pClearStopNum->EnableWindow(TRUE);
	
	m_nStopNumSts[m_nStopNum] = TRUE;
	m_bStopNum[m_nCurrentAxis][m_nStopNum] = TRUE;
}


//清除马达定位完毕输入信号有效
void CADView::OnClearInPos() 
{
	// TODO: Add your control notification handler code here
    if(!WTNMC4A_SetINPOSDisable(m_hDevice, m_LCData[m_nCurrentAxis].AxisNum))
	{
		AfxMessageBox(L"清除马达定位完毕输入信号有效失败！");
		return;
	}
	CButton* pSetInPos = (CButton*)GetDlgItem(IDC_SetInPos);
	pSetInPos->EnableWindow(TRUE);
	
	CButton* pClearInPos = (CButton*)GetDlgItem(IDC_ClearInPos);
	pClearInPos->EnableWindow(FALSE);
	
	m_bInPos[m_nCurrentAxis] = FALSE;
}


int gl_nPointCount = 0;
// 绘制X、Y轴速度曲线图
LRESULT CADView::OnDrawRate(WPARAM wParam,LPARAM lParam)
{
	CPen LinePen[4], *OldPen;;
	CStatic* pWave;
	CDC* pDC;
	int i;
	CRect rect;
	pWave = (CStatic*)GetDlgItem(IDC_WAVE_VT); // 取得picture指针
	pDC = pWave->GetDC(); 
	CRect rcPicture;
	pWave->GetClientRect(rcPicture);
////////////////////////////////////////////////////////////////////////	
	pWave->GetClientRect(rect);  // 取得客户区大小
	m_memDCRate.SetBkMode(TRANSPARENT);
	rect.DeflateRect(1, 1, 1, 1); // 向内缩一个像素
	CBrush brush(BK_COLOR);
	m_memDCRate.FillRect(rect, &brush); // 填充背景
	int nMode;
	nMode = m_memDCRate.SetROP2(R2_MERGEPENNOT);

	CFont font, *OldFont; // 创建字体
	font.CreateFont(14, 0, 0, 0, FW_BOLD, FALSE, FALSE, FALSE, ANSI_CHARSET, OUT_DEFAULT_PRECIS,
		CLIP_DEFAULT_PRECIS, PROOF_QUALITY, DEFAULT_PITCH|FF_DONTCARE, L"宋体");
	
	OldFont = m_memDCRate.SelectObject(&font);
	CPoint point0, point1; 
	
	point0.x = rect.left + 20;
	point0.y = rect.bottom - 20;
	
	point1.x = rect.left + 20;
	point1.y = rect.top + 10;
	LineArrow(&m_memDCRate, point0, point1, 30, 8);  // 绘制Y轴
	m_memDCRate.SetTextColor(TEXT_COLOR);
	m_memDCRate.TextOut(point1.x-15, point1.y-10, "V");
	
	point1.x = rect.right - 10;
	point1.y = rect.bottom - 20;
	m_memDCRate.TextOut(point1.x, point1.y +3, "T");	
	LineArrow(&m_memDCRate, point0, point1, 30, 8);	// 绘制X轴
	for(i=0; i<4; i++)
		LinePen[i].CreatePen(PS_SOLID, 1, m_Color[i]);

	m_memDCRate.TextOut(rect.left+20, rect.bottom-15, "X轴");
	OldPen = m_memDCRate.SelectObject(&LinePen[0]);
	m_memDCRate.MoveTo(rect.left+45, rect.bottom-10);
	m_memDCRate.LineTo(rect.left+70, rect.bottom-10);
	m_memDCRate.SelectObject(OldPen);
	m_memDCRate.TextOut(rect.left+75, rect.bottom-15, "Y轴");
	OldPen = m_memDCRate.SelectObject(&LinePen[1]);
	m_memDCRate.MoveTo(rect.left+100, rect.bottom-10);
	m_memDCRate.LineTo(rect.left+125, rect.bottom-10);
	m_memDCRate.SelectObject(OldPen);
	m_memDCRate.TextOut(rect.left+130, rect.bottom-15, "Z轴");
	OldPen = m_memDCRate.SelectObject(&LinePen[2]);
	m_memDCRate.MoveTo(rect.left+155, rect.bottom-10);
	m_memDCRate.LineTo(rect.left+180, rect.bottom-10);
	m_memDCRate.SelectObject(OldPen);
	m_memDCRate.TextOut(rect.left+185, rect.bottom-15, "U轴");
	OldPen = m_memDCRate.SelectObject(&LinePen[3]);
	m_memDCRate.MoveTo(rect.left+210, rect.bottom-10);
	m_memDCRate.LineTo(rect.left+235, rect.bottom-10);
	m_memDCRate.SelectObject(OldPen);
	
	CRect rcClient = rect;
	rcClient.InflateRect(-20, -20, -20, -20); // 画速度线的区域
	
	// 绘制Y轴刻度
	UINT nData = max(m_DataList[0].DriveSpeed, m_DataList[1].DriveSpeed);

	double nPerHeight = rcClient.Height()/8; // 八等分
	
	CPoint tmpPoint;
	CString str;
	CPen pen(PS_COSMETIC, 1, TEXT_COLOR), *oldPen;
	oldPen = m_memDCRate.SelectObject(&pen);
	for(int nIndex = 0; nIndex < 8; nIndex++) // 写刻度
	{
		tmpPoint.y = (int)(rcClient.bottom - (nIndex+1)* nPerHeight);
		tmpPoint.x = rcClient.left;
		m_memDCRate.MoveTo(tmpPoint.x - 5, tmpPoint.y);
		m_memDCRate.LineTo(tmpPoint);
		str.Format(L"%d", nIndex + 1);
		m_memDCRate.TextOut(tmpPoint.x - 15, tmpPoint.y - 8, str);
	}
	m_memDCRate.SelectObject(oldPen);
	m_memDCRate.TextOut(point0.x - 15, point0.y - 8, "0");
	m_memDCRate.SelectObject(OldFont);
//------------------------------------------------------------------------------------
	double PerVLsb = rcClient.Height()/(8000.0);
	CPoint point[4][8192];
	for(i = 0; i < 4; i++) // 绘制四个通道的图形
	{	
		OldPen = m_memDCRate.SelectObject(&LinePen[i]);
		int nLeft = rcClient.left;
		m_nCount[i] = m_nCount[i] >8191 ? 8191:m_nCount[i] ;
		for(int Index = 0; Index <m_nCount[i]; Index++)  // 计算点的坐标
		{
			if(m_nCount[i]>rcClient.Width())
			{
				point[i][Index].x  = nLeft + Index + 1 - (m_nCount[i]-rcClient.Width());
				if(point[i][Index].x < rcClient.left + 1)  point[i][Index].x = rcClient.left + 1;
			}
			else
			{
				point[i][Index].x  = nLeft + Index + 1;
			}
			point[i][Index].y = rcClient.top + (int)(rcClient.Height() - (m_pLineBuffer[i][Index])*PerVLsb);
		}	

		m_memDCRate.Polyline(&point[i][0], m_nCount[i]);      // 绘制速度-时间曲线	
		
		// 绘制最后一个点
		if((point[i][m_nCount[i] -1].x != 0) && (point[i][m_nCount[i] -1].y != 0))
		{
			CBrush CircleBrush(m_Color[i]);
			CPen CirclePen(PS_SOLID, 1, m_Color[i]);
			CBrush* OldBrush;
			CPen* OldPen;
			OldPen = m_memDCRate.SelectObject(&CirclePen);
			OldBrush = m_memDCRate.SelectObject(&CircleBrush);
			CRect rcCircle;			
			rcCircle.left	 = point[i][m_nCount[i]-1].x - 3;
			rcCircle.top	 = point[i][m_nCount[i]-1].y - 3;
			rcCircle.right	 = point[i][m_nCount[i]-1].x + 3;
			rcCircle.bottom  = point[i][m_nCount[i]-1].y + 3;
			m_memDCRate.Ellipse(rcCircle);
			m_memDCRate.SelectObject(OldBrush);
			m_memDCRate.SelectObject(OldPen);
		}
	
		m_memDCRate.SelectObject(OldPen);
	}
//-----------------------------------------------------------------------------------
	m_memDCRate.SetROP2(nMode);
	pDC->BitBlt(0,0,rcPicture.Width(), rcPicture.Height(), &m_memDCRate, 0,0,SRCCOPY);
	ReleaseDC(pDC);
	return 1;
}

void CADView::OnPaint() 
{
	CPaintDC dc(this); // device context for painting
	
	// TODO: Add your message handler code here
	SendMessage(WM_DRAW_LINE, NULL, NULL); // 直线运动
	SendMessage(WM_DRAW_LINEINTERPOLATION, NULL, NULL); // 直线插补
	switch(m_nFunction)
	{
	case 1:
		SendMessage(WM_DRAW_LINEINTERPOLATION, NULL, NULL); // 直线插补
		break;
	case 2:
		SendMessage(WM_DRAW_CIRCLE, NULL, NULL);    // 圆弧插补	
		break;
	case 3:
		SendMessage(WM_DRAW_SEQUENCE, NULL, NULL);	// 连续插补
		break;
	case 4:
		SendMessage(WM_DRAW_BIT, NULL, NULL); // 位插补
		break;
	case 5:
		SendMessage(WM_DRAW_BIT, NULL, NULL); // 位插补
		break;
	default:
		break;
	}
	
}

// 画箭头
void CADView::LineArrow(CDC* pDC, CPoint P1, CPoint P2, double theta, int length)
{
	// 以P2为原点， 得到向量P2P1
	double Xx, Xy, X1x, X1y, X2x, X2y;
	CPoint point[3];
	Xx = P1.x - P2.x;
	Xy = P1.y - P2.y;
	theta = 3.1415926*theta/180;
	//向量X旋theta角的到X1
	X1x = Xx* cos(theta) - Xy*sin(theta);
	X1y = Xx* sin(theta) + Xy*cos(theta);
	
	//向量X旋转-theta角的到X2
	X2x = Xx* cos(theta) + Xy*sin(theta);
	X2y = Xx* sin(-theta) + Xy*cos(theta);
	
	//伸缩变换为length
	double x1, x2;
	x1 = sqrt(X1x*X1x + X1y*X1y);
	x2 = sqrt(X2x*X2x + X2y*X2y);
	
	X1x = X1x*length/x1;
	X1y = X1y*length/x1;
	
	X2x = X2x*length/x2;
	X2y = X2y*length/x2;
	
	//平移变换，将原点恢复
	
	X1x = X1x + P2.x;
	X1y = X1y + P2.y;
	
	X2x = X2x + P2.x;
	X2y = X2y + P2.y;
	
	point[0] = P2;
	point[1].x = (int)X1x;
	point[1].y = (int)X1y;
	point[2].x = (int)X2x;
	point[2].y = (int)X2y;
	CPen pen, pen1, *OldPen;
	pen.CreatePen(PS_COSMETIC, 2, TEXT_COLOR);
	OldPen = pDC->SelectObject(&pen);
	pDC->MoveTo(P1.x, P1.y);
	pDC->LineTo(P2.x, P2.y);
	pDC->MoveTo(P2.x, P2.y);
	pDC->LineTo((int)X1x, (int)X1y);
	pDC->MoveTo(P2.x, P2.y);
	pDC->LineTo((int)X2x, (int)X2y);
	pDC->SelectObject(OldPen);
}


// 读取速度值
void CADView::ReadLineData(int nAxis)
{
	int nStatus;
	nStatus = WTNMC4A_ReadRR(m_hDevice, nAxis, 1);
	BOOL m_bDec = nStatus&0x10;	     // 减速状态位
	nStatus = WTNMC4A_ReadRR(m_hDevice, NULL, 0);
//	BOOL m_bNotStop = nStatus&nAxis; // 停止状态位
	if (m_nCount[nAxis]>8191)// m_nCount[4]绘制速度曲线图的点的个数
	{
		m_pLineBuffer[nAxis][8191] = m_nRV[nAxis];// m_pLineBuffer[4][8192]X、Y轴绘制速度曲线图的点
	}
	else
		m_pLineBuffer[nAxis][m_nCount[nAxis]] = m_nRV[nAxis];//m_nRV[4]当前速度
	(m_nCount[nAxis])++;
	if(m_bInstStop) // 如果是立即停止，则增加一个速度为0的点
	{
		//for(int i= 0; i<4; i++)
		{		
			if(m_pLineBuffer[nAxis][m_nCount[nAxis]-1]  != 0)
			{
				m_pLineBuffer[nAxis][m_nCount[nAxis]] = 0;
				//(m_nCount[nAxis])++;
				
			}	
		}
		m_bInstStop = FALSE;
	}		
}

// 绘制直线插补位移图
LRESULT CADView::OnDrawLineInterpolation(WPARAM wPara, LPARAM lPara)
{ 
	CStatic* pWave;
	CDC* pDC;
	CRect rect;
	int nArea;
	pWave = (CStatic*)GetDlgItem(IDC_WAVE_XY);
	CClientDC dc(pWave);
	pDC = &dc;
	pWave->GetClientRect(rect); 
	rect.DeflateRect(1, 1, 1, 1);
	CBrush brush(BK_COLOR);
	pDC->FillRect(rect, &brush);
	pDC->SetBkMode(TRANSPARENT);
	pDC->SetTextColor(TEXT_COLOR);

	CPoint point0, point1, point2;
	// 根据终点脉冲数判断将要在哪个象限绘制，确定并绘制坐标轴
 	if(m_LineData.n1AxisPulseNum >= 0)      // 主轴为正
	{ 
 		if(m_LineData.n2AxisPulseNum >= 0)	// 第二轴为正
		{
			point0.x = rect.left + 20;
			point0.y = rect.bottom - 20;
			point1.x = rect.left + 20;
			point1.y = rect.top  + 10;
			point2.x = rect.right - 10;
			point2.y = rect.bottom - 20;		
			pDC->TextOut(point2.x, point2.y+5, GetAxisString(m_InterpAxis.Axis1));
			pDC->TextOut(point1.x-15, point1.y-5, GetAxisString(m_InterpAxis.Axis2));
			nArea = 1;							 // 第一象限
		}
		else									 // Y轴为负
		{
			point0.x = rect.left + 20;
			point0.y = rect.top + 20;
			point1.x = rect.left + 20;
			point1.y = rect.bottom  - 10;
			point2.x = rect.right - 10;
			point2.y = rect.top + 20;
			pDC->TextOut(point2.x, point2.y-15, GetAxisString(m_InterpAxis.Axis1));
			pDC->TextOut(point1.x-15, point1.y-5, GetAxisString(m_InterpAxis.Axis2));
			nArea = 4;							// 第四象限
		}
	}

	if(m_LineData.n1AxisPulseNum <= 0)      // X轴为负
	{ 
		if(m_LineData.n2AxisPulseNum >= 0) // Y轴为正
		{
			point0.x = rect.right - 20;
			point0.y = rect.bottom - 20;
			point1.x = rect.right  -20;
			point1.y = rect.top  + 10;
			point2.x = rect.left + 10;
			point2.y = rect.bottom - 20;
			pDC->TextOut(point2.x-5, point2.y+5, GetAxisString(m_InterpAxis.Axis1));
			pDC->TextOut(point1.x+5, point1.y-5, GetAxisString(m_InterpAxis.Axis2));
			nArea = 2;							// 第二象限
		} 
		else									// Y轴为负
		{
			point0.x = rect.right - 20;
			point0.y = rect.top + 20;
			point1.x = rect.left + 10;
			point1.y = rect.top  + 20;
			point2.x = rect.right - 20;
			point2.y = rect.bottom - 10;
			pDC->TextOut(point1.x-5, point1.y-20, GetAxisString(m_InterpAxis.Axis1));
			pDC->TextOut(point2.x+10, point2.y-10, GetAxisString(m_InterpAxis.Axis2));
			nArea = 3;							// 第三象限
		}
	}

	LineArrow(pDC, point0, point1, 30, 8); // 画箭头
	LineArrow(pDC, point0, point2, 30, 8); // 画箭头

	rect.InflateRect(-20, -20, -20, -20);
	
	LONG nMaxData = max(m_LineData.n1AxisPulseNum, m_LineData.n2AxisPulseNum);
	double PerSLSB = 1.0* rect.Height() / abs(nMaxData);
	CPoint* pPoint;

	pPoint = (CPoint*)malloc(sizeof(CPoint));
	
	CPen Pen(PS_SOLID, 1, RGB(255, 255, 100));
	CPen* OldPen;
	OldPen = pDC->SelectObject(&Pen);

	for(int i = 0; i<m_nIntCount; i++) // 计算点的坐标
	{
		switch(nArea) 
		{
		case 1:
			pPoint[i].x = rect.left + (int)(m_pXPulse[i]* PerSLSB);
			pPoint[i].y = rect.top  +(int)(rect.Height()- m_pYPulse[i]*PerSLSB);
			break;
		case 2:
			pPoint[i].x = rect.right - (int)(abs(m_pXPulse[i])* PerSLSB);
			pPoint[i].y = rect.bottom - (int)(m_pYPulse[i]*PerSLSB);
			break;
		case 3:
			pPoint[i].x = rect.right - (int)( abs(m_pXPulse[i])* PerSLSB);
			pPoint[i].y = rect.top + (int)(abs(m_pYPulse[i])*PerSLSB);			
			break;
		case 4:
			pPoint[i].x = rect.left + (int)(m_pXPulse[i]* PerSLSB);
			pPoint[i].y = rect.top + (int)(abs(m_pYPulse[i])*PerSLSB);			
			break;
		}

		if(pPoint[i].x < rect.left)
		{
			pPoint[i].x = rect.left;
		}
		if(pPoint[i].x > rect.right)
		{
			pPoint[i].x = rect.right;
		}
		if(pPoint[i].y > rect.bottom)
		{
			pPoint[i].y = rect.bottom;
		}
		if(pPoint[i].y < rect.top)
		{
			pPoint[i].y = rect.top;
		}

		int size =(int)(_msize(pPoint));
		pPoint = (CPoint*)realloc(pPoint, size+ sizeof(CPoint));
	}
	pDC->Polyline(pPoint, m_nIntCount);
	
	CBrush CircleBrush(RGB(255, 255, 100));	 
	CBrush* OldBrush;
    // 绘制最后一个点
	OldBrush = pDC->SelectObject(&CircleBrush);
	CRect rcCircle;			
	rcCircle.left	 = pPoint[m_nIntCount-1].x - 3;
	rcCircle.top	 = pPoint[m_nIntCount-1].y - 3;
	rcCircle.right	 = pPoint[m_nIntCount-1].x + 3;
	rcCircle.bottom  = pPoint[m_nIntCount-1].y + 3;
	pDC->Ellipse(rcCircle);
	pDC->SelectObject(OldBrush);
	pDC->SelectObject(OldPen);

	free(pPoint);
	pPoint = NULL;	
	return 1;
}

int gl_nCount = 0;
// 读取插补运动的位移值
void CADView::ReadInterpolationData()
{
/*	int nStatus;
	nStatus = WTNMC4A_ReadRR(m_hDevice, m_InterpAxis.Axis1, 0);
	BOOL m_bNotStop = nStatus&0x01;

	if(m_bNotStop)
	{	
//		m_pXPulse[m_nIntCount] = m_nLPV[m_InterpAxis.Axis1];
//		m_pYPulse[m_nIntCount] = m_nLPV[m_InterpAxis.Axis2];
		m_pXPulse[m_nIntCount] = WTNMC4A_ReadLP(m_hDevice, m_InterpAxis.Axis1);
		m_pYPulse[m_nIntCount] = WTNMC4A_ReadLP(m_hDevice, m_InterpAxis.Axis2);
		m_nIntCount++;
		int size;
		// 动态分配一个点的内存
		size = _msize(m_pXPulse);
		m_pXPulse = (LONG*)realloc(m_pXPulse,size + sizeof(LONG)); 
		size = _msize(m_pYPulse);
		m_pYPulse = (LONG*)realloc(m_pYPulse,size + sizeof(LONG));
	}
	else
	{
//		OnSTOPInstStop();
		ImmediateStop(WTNMC4A_ALLAXIS);
		KillTimer(3);
		KillTimer(1);


	}
*/
	int nStatus;
	nStatus = WTNMC4A_ReadRR(m_hDevice, m_InterpAxis.Axis1, 0);
	BOOL m_bNotStop = nStatus&0x10F;

	if(m_bNotStop)
	{	
//		m_pXPulse[m_nIntCount] = m_nLPV[m_InterpAxis.Axis1];
//		m_pYPulse[m_nIntCount] = m_nLPV[m_InterpAxis.Axis2];

		m_pXPulse[m_nIntCount] = WTNMC4A_ReadLP(m_hDevice, m_InterpAxis.Axis1);
		m_pYPulse[m_nIntCount] = WTNMC4A_ReadLP(m_hDevice, m_InterpAxis.Axis2);
		if (m_pYPulse[m_nIntCount - 1] != m_pYPulse[m_nIntCount]/* || abs(m_pYPulse[m_nIntCount] - m_pYPulse[m_nIntCount - 1]) >= 2*/)
		{
			m_pXPulse[m_nIntCount] = WTNMC4A_ReadLP(m_hDevice, m_InterpAxis.Axis1);
		//	m_pYPulse[m_nIntCount] = WTNMC4A_ReadLP(m_hDevice, m_InterpAxis.Axis2);
		}

		if (abs(m_pYPulse[m_nIntCount] - m_pYPulse[m_nIntCount - 1]) >= 2)
		{
			gl_nCount++;
//			TRACE("Y1 - Y2 = %d, COUNT = %d\n", m_pYPulse[m_nIntCount] - m_pYPulse[m_nIntCount - 1], gl_nCount);
			m_pXPulse[m_nIntCount] = WTNMC4A_ReadLP(m_hDevice, m_InterpAxis.Axis1);
		}

		m_nIntCount++;
		int size;
		// 动态分配一个点的内存
		size =(int)( _msize(m_pXPulse));
		m_pXPulse = (LONG*)realloc(m_pXPulse,size + sizeof(LONG)); 
		size = (int)(_msize(m_pYPulse));
		m_pYPulse = (LONG*)realloc(m_pYPulse,size + sizeof(LONG));
	}
	else
	{
//		OnSTOPInstStop();
		ImmediateStop(WTNMC4A_ALLAXIS);
		KillTimer(3);
		KillTimer(1);
	}
}

// 绘制圆弧插补时的位移曲线图
LRESULT CADView::OnDrawCircle(WPARAM wPara, LPARAM lPara)
{
	CStatic* pWave;
	CDC* pDC;
	CRect rect;
	pWave = (CStatic*)GetDlgItem(IDC_WAVE_XY);
	CClientDC dc(pWave);
	pDC = &dc;
	pWave->GetClientRect(rect); 
	
	rect.DeflateRect(1, 1, 1, 1);
	
	CBrush brush(BK_COLOR);
	pDC->FillRect(rect, &brush);
	pDC->SetBkMode(TRANSPARENT);
	LONG nMaxData = max(abs(m_CircleData.Center1), abs(m_CircleData.Center2));
	double PerSLSB = 1.0*((rect.Height()-20)>>1)/(nMaxData<<1);

	CPoint Center, pointX0, pointX1, pointY0, pointY1;
	Center = rect.CenterPoint();
	Center.x -= int(m_CircleData.Center1*PerSLSB);
	Center.y += int(m_CircleData.Center2*PerSLSB);

	pointX0 = CPoint(rect.left + 5, Center.y);
	pointX1 = CPoint(rect.right - 5, Center.y);
	pointY1 = CPoint(Center.x, rect.top + 5);
	pointY0 = CPoint(Center.x, rect.bottom - 5);
	LineArrow(pDC, pointX0, pointX1, 30, 8);
	LineArrow(pDC, pointY0, pointY1, 30, 8);
	pDC->SetTextColor(TEXT_COLOR);
	
	pDC->TextOut(pointX1.x - 5, pointX1.y + 3, GetAxisString(m_InterpAxis.Axis1));
	pDC->TextOut(pointY1.x - 15, pointY1.y - 5, GetAxisString(m_InterpAxis.Axis2));
	rect.InflateRect(-15, -15, -15, -15);

	CPoint* pPoint;
	pPoint = (CPoint*)malloc(sizeof(CPoint));
	CPen Pen(PS_SOLID, 1, RGB(255, 0, 0));
	CPen* OldPen;
	OldPen = pDC->SelectObject(&Pen);

	for(int i = 0; i<m_nIntCount; i++)
	{
		pPoint[i].x = rect.left + (int)(Center.x + m_pXPulse[i]* PerSLSB) - 15;
		pPoint[i].y = rect.top  + (int)(Center.y + (-m_pYPulse[i])*PerSLSB) - 15 ;
	
		if(pPoint[i].x < rect.left)
		{
			pPoint[i].x = rect.left;
		}
		if(pPoint[i].x > rect.right)
		{
			pPoint[i].x = rect.right;
		}
		if(pPoint[i].y > rect.bottom)
		{
			pPoint[i].y = rect.bottom;
		}
		if(pPoint[i].y < rect.top)
		{
			pPoint[i].y = rect.top;
		}
		int size = (int)(_msize(pPoint));
		pPoint = (CPoint*)realloc(pPoint, size+ sizeof(CPoint));
	}
 	pDC->Polyline(pPoint, m_nIntCount);
	
	CBrush CircleBrush(RGB(255, 0, 0));	 
	CBrush* OldBrush;
	
	OldBrush = pDC->SelectObject(&CircleBrush);
	CRect rcCircle;	
	if(m_nIntCount>0)
	{
		
		rcCircle.left	 = pPoint[m_nIntCount-1].x - 3;
		rcCircle.top	 = pPoint[m_nIntCount-1].y - 3;
		rcCircle.right	 = pPoint[m_nIntCount-1].x + 3;
		rcCircle.bottom  = pPoint[m_nIntCount-1].y + 3;	
		pDC->Ellipse(rcCircle);
	}		

	pDC->SelectObject(OldBrush);
	pDC->SelectObject(OldPen);

	free(pPoint);
	pPoint = NULL;
	return 1;
}

// 绘制连续插补时的位移图
LRESULT CADView::OnDrawSequence(WPARAM wPara, LPARAM lPara)
{
	CStatic* pWave;
	CDC* pDC;
	CRect rect;
	pWave = (CStatic*)GetDlgItem(IDC_WAVE_XY);
	//	pDC = pWave->GetDC();
	CClientDC dc(pWave);
	pDC = &dc;
	pWave->GetClientRect(rect); 
	
	rect.DeflateRect(1, 1, 1, 1);
	
	CBrush brush(BK_COLOR);
	pDC->FillRect(rect, &brush);
 
	rect.InflateRect(-15, -15, -15, -15);

	double PerSLSB = rect.Width()/14000.0;	

	
	CPoint Center;
	Center.x = rect.left + (int)(2000*PerSLSB);
	Center.y = rect.bottom - 30;
	CPoint* pPoint;
	
	pPoint = (CPoint*)malloc(sizeof(CPoint));
	
	CPen Pen(PS_SOLID, 1, RGB(255, 0, 0));
	CPen* OldPen;
	OldPen = pDC->SelectObject(&Pen);
	
	for(int i = 0; i<m_nIntCount; i++)
	{
		pPoint[i].x =  (int)(Center.x + m_pXPulse[i]* PerSLSB);
		pPoint[i].y =  (int)(Center.y + (-m_pYPulse[i])*PerSLSB);
		if(pPoint[i].x < rect.left)
		{
			pPoint[i].x = rect.left;
		}
		if(pPoint[i].x > rect.right)
		{
			pPoint[i].x = rect.right;
		}
		if(pPoint[i].y > rect.bottom)
		{
			pPoint[i].y = rect.bottom;
		}
		if(pPoint[i].y < rect.top)
		{
			pPoint[i].y = rect.top;
		}
		int size = (int)(_msize(pPoint));
		pPoint = (CPoint*)realloc(pPoint, size+ sizeof(CPoint));
	}
	
	pDC->Polyline(pPoint, m_nIntCount);
	
	CBrush CircleBrush(RGB(255, 0, 0));	 
	CBrush* OldBrush;
	
	OldBrush = pDC->SelectObject(&CircleBrush);
	CRect rcCircle;	
	if(m_nIntCount>0)
	{
		
		rcCircle.left	 = pPoint[m_nIntCount-1].x - 3;
		rcCircle.top	 = pPoint[m_nIntCount-1].y - 3;
		rcCircle.right	 = pPoint[m_nIntCount-1].x + 3;
		rcCircle.bottom  = pPoint[m_nIntCount-1].y + 3;	
		pDC->Ellipse(rcCircle);
	}		
	
	pDC->SelectObject(OldBrush);
	pDC->SelectObject(OldPen);
	
	free(pPoint);
	pPoint = NULL;
	return 1;
}

// 画跑道图形
UINT StartSequenceProc(PVOID hWnd) 
{
	gl_bSequenceRun = TRUE;
	HANDLE hDevice = (HANDLE)hWnd;
	WTNMC4A_PARA_DataList DataList;
	WTNMC4A_PARA_InterpolationAxis InterAxis;
	WTNMC4A_PARA_LineData LineData;
	WTNMC4A_PARA_CircleData CircleData;
	InterAxis.Axis1 = WTNMC4A_XAXIS;
	InterAxis.Axis2 = WTNMC4A_YAXIS;

 	while (gl_bSequenceRun)
	{
 		DataList.Multiple = 1;			  // 倍率
		DataList.StartSpeed = 1000;       // 初始速度
		DataList.DriveSpeed = 1000;       // 驱动速度
		DataList.Acceleration = 1250;     // 加速度
		DataList.Deceleration = 1250;
		// 先启动直线插补---------------------------------------------------------
		LineData.Line_Curve = 0;          // 直线运动
		LineData.ConstantSpeed = 1;		  // 固定线速度
		LineData.n1AxisPulseNum = 10000;  // X轴插补脉冲数
		LineData.n2AxisPulseNum = 0;      // Y轴插补脉冲数
		WTNMC4A_InitLineInterpolation_2D(hDevice, &DataList, &InterAxis, &LineData); 
		WTNMC4A_StartLineInterpolation_2D(hDevice);
 		WTNMC4A_NextWait(hDevice);              // 等待连续插补下一个数据
 		if(!gl_bSequenceRun) goto EXIT;
		// 再启动反方向圆弧插补运动------------------------------------------------
		CircleData.ConstantSpeed = 1;     // 固定线速度
		CircleData.Center1 = 0;           // X轴圆心坐标 (脉冲数)
		CircleData.Center2 = 2000;        // Y轴圆心坐标 (脉冲数)
		CircleData.Pulse1 = 0;            // X轴终点坐标 (脉冲数)
		CircleData.Pulse2 = 4000;         // Y轴终点坐标 (脉冲数)
		WTNMC4A_InitCWInterpolation_2D(hDevice, &DataList, &InterAxis, &CircleData); 
		WTNMC4A_StartCWInterpolation_2D(hDevice, WTNMC4A_MDIRECTION); // 启动反方向圆弧插补
 		WTNMC4A_NextWait(hDevice);
 		if(!gl_bSequenceRun) goto EXIT;
		// 再启动直线插补运动--------------------------------------------------------
		LineData.n1AxisPulseNum = -10000; // X轴插补脉冲数
		LineData.n2AxisPulseNum = 0;      // Y轴插补脉冲数
		LineData.ConstantSpeed = 1;		  // 固定线速度
		WTNMC4A_InitLineInterpolation_2D(hDevice, &DataList, &InterAxis, &LineData);
		WTNMC4A_StartLineInterpolation_2D(hDevice); // 启动直线插补
 		WTNMC4A_NextWait(hDevice);
 		if(!gl_bSequenceRun) goto EXIT;
		// 再启动反方向圆弧插补-------------------------------------------------------
		CircleData.Center1 = 0;          // X轴圆心坐标 (脉冲数)
		CircleData.Center2 = -2000;      // Y轴圆心坐标 (脉冲数)
		CircleData.Pulse1 = 0;           // X轴终点坐标 (脉冲数)
		CircleData.Pulse2 = -4000;       // Y轴终点坐标 (脉冲数)
		WTNMC4A_InitCWInterpolation_2D(hDevice, &DataList, &InterAxis, &CircleData); // 初始化圆弧插补运动
		WTNMC4A_StartCWInterpolation_2D(hDevice, WTNMC4A_MDIRECTION); // 启动反方向圆弧插补
 		WTNMC4A_NextWait(hDevice);              // 等待连续插补下一个数据
	}
	
EXIT:
	gl_bSequenceStop = TRUE;
	return 0;
	
}
// 位插补的线程函数(六边形)
UINT SetBitDataThread_2D(PVOID hWnd)
{
	HANDLE hDevice = (HANDLE)hWnd;
	WTNMC4A_PARA_DataList DataList;
	WTNMC4A_PARA_InterpolationAxis InterAxis;
	USHORT nBitData[6][4] = {   // 位插补的十六进制数据(六边形)
		{0x0000, 0xFFFF, 0x0000, 0x0000}, 
		{0x0000, 0xFFFF, 0xFFFF, 0x0000},
		{0xFFFF, 0x0000, 0xFFFF, 0x0000},
		{0xFFFF, 0x0000, 0x0000, 0x0000},		
		{0xFFFF, 0x0000, 0x0000, 0xFFFF},
		{0x0000, 0xFFFF, 0x0000, 0xFFFF},
	};
	DataList.Multiple = 1;
	DataList.StartSpeed = 2;    // 初始速度
	DataList.DriveSpeed = 2;    // 驱动速度
	DataList.AccIncRate = 1;
	DataList.Acceleration = 1000;
	InterAxis.Axis1 = WTNMC4A_XAXIS;
	InterAxis.Axis2 = WTNMC4A_YAXIS;
	gl_bBitDataProc = TRUE;
	WTNMC4A_InitBitInterpolation_2D(hDevice, &InterAxis, &DataList);
	for(int i = 0; i<3; i++)
	{
		WTNMC4A_SetBP_2D(hDevice, nBitData[i][0], nBitData[i][1], nBitData[i][2], nBitData[i][3]);
		//WTNMC4A_SetBP_2D(hDevice, (LONG)nBitData[i*4], nBitData[i*4+1], nBitData[i*4+2], nBitData[i*4+3]);
		
	}
	WTNMC4A_StartBitInterpolation_2D(hDevice);
	while (gl_bBitDataProc)
	{	
		
		WTNMC4A_BPWait(hDevice, &gl_bBitDataProc);
// 		if (!gl_bBitDataProc)
// 			break;
		
		WTNMC4A_SetBP_2D(hDevice, nBitData[3][0], nBitData[3][1], nBitData[3][2], nBitData[3][3]);
		WTNMC4A_BPWait(hDevice, &gl_bBitDataProc);
// 		
// 		if (!gl_bBitDataProc)
// 			break;
// 		
		WTNMC4A_SetBP_2D(hDevice, nBitData[4][0], nBitData[4][1], nBitData[4][2], nBitData[4][3]);
		WTNMC4A_BPWait(hDevice, &gl_bBitDataProc);
// 		if (!gl_bBitDataProc)
// 			break;
// 		
		WTNMC4A_SetBP_2D(hDevice, nBitData[5][0], nBitData[5][1], nBitData[5][2], nBitData[5][3]);
		WTNMC4A_BPWait(hDevice, &gl_bBitDataProc);
// 		if (!gl_bBitDataProc)
// 			break;
// 		
		WTNMC4A_SetBP_2D(hDevice, nBitData[0][0], nBitData[0][1], nBitData[0][2], nBitData[0][3]);
// 		if (!gl_bBitDataProc)
// 			break;
		WTNMC4A_BPWait(hDevice, &gl_bBitDataProc);
// 		if (!gl_bBitDataProc)
// 			break;
		
		WTNMC4A_SetBP_2D(hDevice, nBitData[1][0], nBitData[1][1], nBitData[1][2], nBitData[1][3]);
// 		if (!gl_bBitDataProc)
// 			break;
		WTNMC4A_BPWait(hDevice, &gl_bBitDataProc);
// 		if (!gl_bBitDataProc)
// 			break;
		
		WTNMC4A_SetBP_2D(hDevice, nBitData[2][0], nBitData[2][1], nBitData[2][2], nBitData[2][3]);
// 		if (!gl_bBitDataProc)
// 			break;

	}
	
	gl_bBitStop = TRUE;
	WTNMC4A_ReleaseBitInterpolation(hDevice);
	
	return 0;

}
	

// 位插补的线程函数(心形)
UINT StartBitProc(PVOID hWnd)
{
	HANDLE hDevice = (HANDLE)hWnd;
	WTNMC4A_PARA_DataList DataList;
	WTNMC4A_PARA_InterpolationAxis InterAxis;
    USHORT nData[4][4] = {
		{0x0000, 0x2BFF, 0xFFD4, 0x0000}, 
		{0xF6FE, 0x0000, 0x000F, 0x3FC0},
		{0x1FDB, 0x0000, 0x00FF, 0xFC00},
		{0x0000, 0x5FF5, 0x0000, 0x0AFF}
	};
	DataList.Multiple = 1;
	DataList.StartSpeed = 1;     // 初始速度
	DataList.DriveSpeed = 1;     // 驱动速度
	DataList.Acceleration = 1000;
	DataList.AccIncRate = 1000;

	InterAxis.Axis1 = WTNMC4A_XAXIS;
	InterAxis.Axis2 = WTNMC4A_YAXIS;
	gl_bBitProc = TRUE;
	WTNMC4A_InitBitInterpolation_2D(hDevice, &InterAxis, &DataList); // 初始化位插补
//	gl_bSequenceRun = FALSE; // ??????????????????????
	for(int i = 0; i<3; i++)
	{
		WTNMC4A_SetBP_2D(hDevice, nData[i][0], nData[i][1], nData[i][2], nData[i][3]);
	}
	WTNMC4A_StartBitInterpolation_2D(hDevice);
	while (gl_bBitProc)
	{	

		WTNMC4A_BPWait(hDevice, &gl_bBitProc);
// 		if (!gl_bBitProc)
// 			break;
		
		WTNMC4A_SetBP_2D(hDevice, nData[3][0], nData[3][1], nData[3][2], nData[3][3]);
		WTNMC4A_BPWait(hDevice, &gl_bBitProc);

// 		if (!gl_bBitProc)
// 			break;
	
		WTNMC4A_SetBP_2D(hDevice, nData[0][0], nData[0][1], nData[0][2], nData[0][3]);
		WTNMC4A_BPWait(hDevice, &gl_bBitProc);
// 		if (!gl_bBitProc)
// 			break;
		
		WTNMC4A_SetBP_2D(hDevice, nData[1][0], nData[1][1], nData[1][2], nData[1][3]);
		WTNMC4A_BPWait(hDevice, &gl_bBitProc);
// 		if (!gl_bBitProc)
// 			break;
// 	
		WTNMC4A_SetBP_2D(hDevice, nData[2][0], nData[2][1], nData[2][2], nData[2][3]);
// 		if (!gl_bBitProc)
// 			break;
	}
	
	gl_bStop = TRUE;
	WTNMC4A_ReleaseBitInterpolation(hDevice);
	
	return 0;

}


// 绘制位插补时位移曲线图
LRESULT CADView::OnDrawBit(WPARAM wPara, LPARAM lPara)
{
	CStatic* pWave;
	CDC* pDC;
	CRect rect;
	pWave = (CStatic*)GetDlgItem(IDC_WAVE_XY);
	CClientDC dc(pWave);
	pDC = &dc;
	pWave->GetClientRect(rect); 

	rect.DeflateRect(1, 1, 1, 1);
	
	CBrush brush(BK_COLOR);
	pDC->FillRect(rect, &brush);
	
	rect.InflateRect(-15, -15, -15, -15);
	double PerSLSB;
	CPoint Center;
	if(m_nStartBitMode == 1) // 心形
	{
		PerSLSB = rect.Width()/40.0;
		Center.x = rect.left + (int)(rect.Width()/2);	
	}
	if(m_nStartBitMode == 2) // 六边形
	{
		PerSLSB = rect.Width()/70.0;
		Center.x = rect.right - 150;
	}
	
	Center.y = rect.bottom ;
	CPoint* pPoint;
	
	pPoint = (CPoint*)malloc(sizeof(CPoint));
	
	CPen Pen(PS_SOLID, 1, RGB(255, 0, 0));
	CPen* OldPen;

	OldPen = pDC->SelectObject(&Pen);

	for(int i = 0; i<m_nIntCount; i++)
	{
		pPoint[i].x =  (int)(Center.x + m_pXPulse[i]* PerSLSB);
		pPoint[i].y =  (int)(Center.y + (-m_pYPulse[i])*PerSLSB);

		if(pPoint[i].x < rect.left)
		{
			pPoint[i].x = rect.left;
		}
		if(pPoint[i].x > rect.right)
		{
			pPoint[i].x = rect.right;
		}
		if(pPoint[i].y > rect.bottom)
		{
			pPoint[i].y = rect.bottom;
		}
		if(pPoint[i].y < rect.top)
		{
			pPoint[i].y = rect.top;
		}
		int size = (int)(_msize(pPoint));
		pPoint = (CPoint*)realloc(pPoint, size+ sizeof(CPoint));
	}
 
	pDC->Polyline(pPoint, m_nIntCount);
	
	CBrush CircleBrush(RGB(255, 0, 0));	 
	CBrush* OldBrush;
	
	OldBrush = pDC->SelectObject(&CircleBrush);
	CRect rcCircle;	
	if(m_nIntCount>0)
	{	
		rcCircle.left	 = pPoint[m_nIntCount-1].x - 3;
		rcCircle.top	 = pPoint[m_nIntCount-1].y - 3;
		rcCircle.right	 = pPoint[m_nIntCount-1].x + 3;
		rcCircle.bottom  = pPoint[m_nIntCount-1].y + 3;	
		pDC->Ellipse(rcCircle);
	}		
	
	pDC->SelectObject(OldBrush);
	pDC->SelectObject(OldPen);
	
	free(pPoint);
	pPoint = NULL;
	return 1;
}


void CADView::OnSelchangeTabFuncton(NMHDR* pNMHDR, LRESULT* pResult) 
{
	// TODO: Add your control notification handler code here
	int nIndex = m_TabFunction.GetCurSel();
	switch(nIndex)
	{
	case 0: // 线性运动
		m_TabComSet.ShowWindow(SW_SHOW);
		m_PageComSet.ShowWindow(SW_SHOW);
		m_TabLimitSet.ShowWindow(SW_SHOW);
		m_PageHardLimit.ShowWindow(SW_SHOW);
		m_PageSoftLimit.ShowWindow(SW_HIDE);
		m_PageLine.ShowWindow(SW_SHOW);
		m_PageInterpolation.ShowWindow(SW_HIDE);
		m_PageDIO.ShowWindow(SW_HIDE);
		m_PageInterApp.ShowWindow(SW_HIDE);
		m_Static_FuncSet.ShowWindow(SW_SHOW);
		m_PageComSet.ShowDVImpulseNumWindow(TRUE);
//		m_bHomeSearchEnable = FALSE;
		m_PageLine.SetFunction(LINECURVE_FUNC);
		break;
	case 1: // 同步运动 
		m_TabComSet.ShowWindow(SW_SHOW);
		m_PageComSet.ShowWindow(SW_SHOW);
		m_TabLimitSet.ShowWindow(SW_SHOW);
		m_PageHardLimit.ShowWindow(SW_SHOW);
		m_PageSoftLimit.ShowWindow(SW_HIDE);
		m_PageLine.ShowWindow(SW_SHOW);
		m_PageInterpolation.ShowWindow(SW_HIDE);
		m_PageDIO.ShowWindow(SW_HIDE);
		m_PageInterApp.ShowWindow(SW_HIDE);
		m_Static_FuncSet.ShowWindow(SW_SHOW);
		m_PageComSet.ShowDVImpulseNumWindow(TRUE);
//		m_bHomeSearchEnable = FALSE;
		m_PageLine.SetFunction(SYNCHRON_FUNC);
		break;
	case 2: // 自动原点搜寻
		m_TabComSet.ShowWindow(SW_SHOW);
		m_PageComSet.ShowWindow(SW_SHOW);
		m_TabLimitSet.ShowWindow(SW_SHOW);
		m_PageHardLimit.ShowWindow(SW_SHOW);
		m_PageSoftLimit.ShowWindow(SW_HIDE);
		m_PageLine.ShowWindow(SW_SHOW);
		m_PageInterpolation.ShowWindow(SW_HIDE);
		m_PageDIO.ShowWindow(SW_HIDE);
		m_PageInterApp.ShowWindow(SW_HIDE);
		m_Static_FuncSet.ShowWindow(SW_SHOW);
		m_PageComSet.ShowDVImpulseNumWindow(TRUE);
		SetHomeSearchWnd();
//		m_bHomeSearchEnable = TRUE;
		m_PageLine.SetFunction(HOMESEARCH_FUNC);
		break;
	case 3: // 插补运动
		m_TabComSet.ShowWindow(SW_SHOW);
		m_PageComSet.ShowWindow(SW_SHOW);
		m_TabLimitSet.ShowWindow(SW_SHOW);
		m_PageHardLimit.ShowWindow(SW_SHOW);
		m_PageSoftLimit.ShowWindow(SW_HIDE);
		m_PageInterpolation.ShowWindow(SW_SHOW);
		m_PageLine.ShowWindow(SW_HIDE);
		m_PageDIO.ShowWindow(SW_HIDE);
		m_PageInterApp.ShowWindow(SW_HIDE);
		m_Static_FuncSet.ShowWindow(SW_SHOW);
		m_PageComSet.ShowDVImpulseNumWindow(FALSE);
		break;
	case 4: // 插补实际应用
		m_TabComSet.ShowWindow(SW_HIDE);
		m_PageComSet.ShowWindow(SW_HIDE);
		m_TabLimitSet.ShowWindow(SW_HIDE);
		m_PageHardLimit.ShowWindow(SW_HIDE);
		m_PageSoftLimit.ShowWindow(SW_HIDE);
		m_PageInterpolation.ShowWindow(SW_HIDE);
		m_PageLine.ShowWindow(SW_HIDE);
		m_PageDIO.ShowWindow(SW_HIDE);
		m_Static_FuncSet.ShowWindow(SW_HIDE);
		m_PageInterApp.ShowWindow(SW_SHOW);
		break;
	case 5: // 开关量测试
		m_PageDIO.ShowWindow(SW_SHOW);
		m_PageComSet.ShowWindow(SW_HIDE);
		m_TabLimitSet.ShowWindow(SW_HIDE);
		m_TabComSet.ShowWindow(SW_HIDE);
		m_PageHardLimit.ShowWindow(SW_HIDE);
		m_PageSoftLimit.ShowWindow(SW_HIDE);
		m_PageLine.ShowWindow(SW_HIDE);
		m_PageInterpolation.ShowWindow(SW_HIDE);
		m_PageInterApp.ShowWindow(SW_HIDE);
		m_Static_FuncSet.ShowWindow(SW_HIDE);
		break;
	default:
		break;
	}
	*pResult = 0;
}

void CADView::OnSelchangeTabLimitSet(NMHDR* pNMHDR, LRESULT* pResult) 
{
	// TODO: Add your control notification handler code here
	int nIndex = m_TabLimitSet.GetCurSel();
	switch(nIndex)
	{
	case 0: // 硬件限位
		m_PageHardLimit.ShowWindow(SW_SHOW);
		m_PageSoftLimit.ShowWindow(SW_HIDE);
		break;
	case 1: // 软件限位
		m_PageSoftLimit.ShowWindow(SW_SHOW);
		m_PageHardLimit.ShowWindow(SW_HIDE);
		break;
	default:
		break;
	}		
	*pResult = 0;
}

// 启动直线(S曲线)运动
void CADView::StartLineMovement(int nAxisNum)
{
	if(m_hDevice == INVALID_HANDLE_VALUE)
		return;
	m_LCData[nAxisNum].AxisNum = nAxisNum; // 指定要初始化的轴号
	if(nAxisNum != WTNMC4A_ALLAXIS) // 如果是启动单轴
	{
		WTNMC4A_SetLP(m_hDevice, nAxisNum, 0); // 逻辑位置计数器清零
		WTNMC4A_SetEP(m_hDevice, nAxisNum, 0); // 实位计数器清零
// 		WTNMC4A_SetEncoderSignalType(m_hDevice,WTNMC4A_XAXIS, 1, 0);		// 上下脉冲方式
// 		WTNMC4A_SetEncoderSignalType(m_hDevice, WTNMC4A_YAXIS, 1, 0);		// 上下脉冲方式
// 		WTNMC4A_SetEncoderSignalType(m_hDevice, WTNMC4A_ZAXIS, 1, 0);		// 上下脉冲方式
// 		WTNMC4A_SetEncoderSignalType(m_hDevice, WTNMC4A_UAXIS, 1, 0);		// 上下脉冲方式
		WTNMC4A_PulseInputMode( m_hDevice,	
								nAxisNum,	
								gl_pADView->m_lPulseModeIN[nAxisNum] );  
		m_nCount[nAxisNum] = 0;
		if(!WTNMC4A_InitLVDV(m_hDevice,      // 初始化当前轴
			&m_DataList[nAxisNum],
			&m_LCData[nAxisNum]))
		{
			AfxMessageBox(L"初始化单轴直线动失败！");
		}

		SetInterrruptBit(nAxisNum);         // 初始化中断位，并启动子线程
		if(!WTNMC4A_StartLVDV(m_hDevice,    //	启动单轴
			nAxisNum))
		{
			AfxMessageBox(L"单轴启动失败！");
			return;
		}
		m_bAxisRun[nAxisNum] = TRUE;
	}
	else
	{
		OnClearCounter(); // 清除所有的计数器
		for(int Index=0; Index<4; Index++)
		{
//			WTNMC4A_SetEncoderSignalType(m_hDevice, Index, 1, 0);		// 上下脉冲方式
			WTNMC4A_PulseInputMode( m_hDevice,	
								Index,	
								gl_pADView->m_lPulseModeIN[Index] );  
			if(!WTNMC4A_InitLVDV(m_hDevice,      // 初始化当前轴
				&m_DataList[Index],
				&m_LCData[Index]))
			{
				AfxMessageBox(L"初始化单轴直线动失败！");
			}
			m_nCount[Index] = 0;
		}
		SetInterrruptBit(WTNMC4A_ALLAXIS); // 初始化中断位，并启动子线程
		WTNMC4A_Start4D(m_hDevice); // 4轴同时启动
		m_bAxisRun[WTNMC4A_XAXIS] = TRUE;
		m_bAxisRun[WTNMC4A_YAXIS] = TRUE;
		m_bAxisRun[WTNMC4A_ZAXIS] = TRUE;
		m_bAxisRun[WTNMC4A_UAXIS] = TRUE;
	}
	m_bAllAxisRun = TRUE;
	EnableWindows(FALSE);

	SetTimer(1, 10 , NULL);  // 启动定时器1，用来刷新计数器的状态
	SetTimer(3, 50, NULL);   // 启动定时器3，用来读取轴的当前速度
	m_nFunction = 0;         // 直线运动
}

// 减速停止
void CADView::DecStop(int nAxisNum)
{

	if (!WTNMC4A_DecStop(m_hDevice, nAxisNum))
	{
		AfxMessageBox(L"减速停止失败!");
		return;
	}
}

// 立即停止
void CADView::ImmediateStop(int nAxisNum)
{

	if(!WTNMC4A_InstStop(m_hDevice, nAxisNum))
	{
		AfxMessageBox(L"立即停止失败!");
		return;
	}
	if (gl_bInstStop)
	{
		EnableWindows(TRUE);
	//	gl_bInstStop = FALSE;
	} 
	else
	{
		if(nAxisNum == (int)m_nCurrentAxis)	
		{
			EnableWindows(TRUE);
			
		}
		else
		{
			EnableWindows(FALSE);
 		}
	} 
 
 	RefreshStatusX(); // 停止定时器后再刷新一下各状态
 	RefreshStatusY();
 	RefreshStatusZ();
 	RefreshStatusU();
	//KillTimer(1);

//	ClearZeroStatus(nAxisNum);
	m_bInstStop = TRUE;	
	gl_bWaitInt = FALSE;
}

int CADView::OnCreate(LPCREATESTRUCT lpCreateStruct) 
{
	if (CFormView::OnCreate(lpCreateStruct) == -1)
		return -1;
	
	// TODO: Add your specialized creation code here
	CSysApp* pApp = (CSysApp*)AfxGetApp();
	m_hDevice = pApp->m_hDeviceApp;

	return 0;
}


// 状态清零(速度和加速度)
void CADView::ClearZeroStatus(int nAxisNum)
{
	CStatic* pRV, *pRA; 
	switch(nAxisNum)
	{
	case WTNMC4A_XAXIS: // X轴
		pRV = (CStatic*)GetDlgItem(IDC_RVX);
		pRA = (CStatic*)GetDlgItem(IDC_RAX);
		pRV->SetWindowText(L"0");
		pRA->SetWindowText(L"0");
		break;
	case WTNMC4A_YAXIS: // Y轴
		pRV = (CStatic*)GetDlgItem(IDC_RVY);
		pRA = (CStatic*)GetDlgItem(IDC_RAY);
		pRV->SetWindowText(L"0");
		pRA->SetWindowText(L"0");
		break;
	case WTNMC4A_ZAXIS: //Z轴
		pRV = (CStatic*)GetDlgItem(IDC_RVZ);
		pRA = (CStatic*)GetDlgItem(IDC_RAZ);
		pRV->SetWindowText(L"0");
		pRA->SetWindowText(L"0");
		break;
	case WTNMC4A_UAXIS: //U轴
		pRV = (CStatic*)GetDlgItem(IDC_RVU);
		pRA = (CStatic*)GetDlgItem(IDC_RAU);
		pRV->SetWindowText(L"0");
		pRA->SetWindowText(L"0");
		break;
	case WTNMC4A_ALLAXIS: // 四轴
		ClearZeroStatus(WTNMC4A_XAXIS);
		ClearZeroStatus(WTNMC4A_YAXIS);
		ClearZeroStatus(WTNMC4A_ZAXIS);
		ClearZeroStatus(WTNMC4A_UAXIS);
		break;
	default:
		break;
	}

}

// 属性页的可用状态
void CADView::EnableWindows(BOOL bEnable)
{
	m_PageComSet.EnableWindows(bEnable);
	m_PageLine.EnableWindows(bEnable);
	m_PageInterpolation.EnableWindows(bEnable);
}

// 外部点动
void CADView::OutStart(int nAixsNum)
{
	if(nAixsNum != WTNMC4A_ALLAXIS)
	{
		WTNMC4A_SetLP(m_hDevice, nAixsNum, 0); // 逻辑位置计数器清零
		WTNMC4A_SetEP(m_hDevice, nAixsNum, 0); // 实位计数器清零
		m_nCount[nAixsNum] = 0;
	
// 		WTNMC4A_SetEncoderSignalType(m_hDevice,WTNMC4A_XAXIS, 1, 0);		// 上下脉冲方式
// 		WTNMC4A_SetEncoderSignalType(m_hDevice, WTNMC4A_YAXIS, 1, 0);		// 上下脉冲方式
// 		WTNMC4A_SetEncoderSignalType(m_hDevice, WTNMC4A_ZAXIS, 1, 0);		// 上下脉冲方式
// 		WTNMC4A_SetEncoderSignalType(m_hDevice, WTNMC4A_UAXIS, 1, 0);		// 上下脉冲方式
		WTNMC4A_PulseInputMode( m_hDevice,	
								nAixsNum,	
								gl_pADView->m_lPulseModeIN[nAixsNum] );  
		WTNMC4A_InitLVDV(						// 初始化连续,定长脉冲驱动
						m_hDevice,		        // 设备句柄
						&m_DataList[nAixsNum],	// 公共参数结构体指针
						&m_LCData[nAixsNum]);	// 直线S曲线参数结构体指针
	
		if (m_LCData[nAixsNum].LV_DV)
		{
			WTNMC4A_SetOutEnableLV(				// 设置外部使能连续驱动(保持低电平有效)
								m_hDevice,		// 设备句柄
								nAixsNum);		// 轴号(WTNMC4A_XAXIS:X轴,WTNMC4A_YAXIS:Y轴, WTNMC4A_ZAXIS:Z轴,WTNMC4A_UAXIS:U轴) 
		}
		else
		{
			WTNMC4A_SetOutEnableDV(m_hDevice, nAixsNum); // 设置外部使能定量驱动(下降沿有效)			
		}
		m_bAxisRun[nAixsNum] = TRUE;
		
	}
	else
	{
		OnClearCounter();   // 所有计数器清零
		for(int Index=0; Index<4; Index++)
		{
//			WTNMC4A_SetEncoderSignalType(m_hDevice, Index, 1, 0);		// 上下脉冲方式
			WTNMC4A_PulseInputMode( m_hDevice,	
								Index,	
								gl_pADView->m_lPulseModeIN[Index] );  
			if(!WTNMC4A_InitLVDV(m_hDevice,      // 初始化当前轴
				&m_DataList[Index],
				&m_LCData[Index]))
			{
				AfxMessageBox(L"初始化单轴直线动失败！");
			}
			m_nCount[Index] = 0;
		}
		if (m_LCData[m_nCurrentAxis].LV_DV)
		{

			WTNMC4A_SetOutEnableLV(
				m_hDevice,		// 设备句柄
				m_LCData[m_nCurrentAxis].AxisNum);	// 轴号(WTNMC4A_XAXIS:X轴,WTNMC4A_YAXIS:Y轴, WTNMC4A_ZAXIS:Z轴,WTNMC4A_UAXIS:U轴) 

		}
		else
		{
			WTNMC4A_SetOutEnableDV(m_hDevice, m_LCData[m_nCurrentAxis].AxisNum);			
		}
		
		for (int i=0; i<4; i++)
		{
			m_bAxisRun[i] = TRUE;
		}
		
	}

	SetTimer(3, 1000, NULL); // 启动定时器3，用来读取轴的当前速度
	SetTimer(1, 10 , NULL);  // 启动定时器1，用来刷新计数器的状态
	m_nFunction = 0;
}

// 开始直线插补运动(任意两轴或三轴)
void CADView::StartLineInterpMovement(int nAxisCount)
{
	if(m_hDevice == INVALID_HANDLE_VALUE)
		return;
	OnClearCounter(); // 所有计数器清零
	m_PageComSet.EnableWindows(FALSE);
	m_PageInterpolation.EnableWindows(FALSE);
// 	WTNMC4A_SetEncoderSignalType(m_hDevice,WTNMC4A_XAXIS, 1, 0);		// 上下脉冲方式
// 	WTNMC4A_SetEncoderSignalType(m_hDevice, WTNMC4A_YAXIS, 1, 0);		// 上下脉冲方式
// 	WTNMC4A_SetEncoderSignalType(m_hDevice, WTNMC4A_ZAXIS, 1, 0);		// 上下脉冲方式
// 	WTNMC4A_SetEncoderSignalType(m_hDevice, WTNMC4A_UAXIS, 1, 0);		// 上下脉冲方式
	for (int Index = 0;Index<4;Index++)
	{
		WTNMC4A_PulseInputMode( m_hDevice,	
								Index,	
								gl_pADView->m_lPulseModeIN[Index] );  
	}

	if(nAxisCount == 2) // 任意两轴直线插补
	{
		WTNMC4A_InitLineInterpolation_2D(			// 初始化直线插补运动 
								m_hDevice,	     	// 设备句柄
								&m_DataList[gl_pADView->m_InterpAxis.Axis1],
								&m_InterpAxis,
								&m_LineData); 
		WTNMC4A_StartLineInterpolation_2D(m_hDevice);
		m_bAxisRun[m_InterpAxis.Axis1] = TRUE; 
		m_bAxisRun[m_InterpAxis.Axis2] = TRUE;
	}
	else // 任意三轴直线插补
	{
		WTNMC4A_InitLineInterpolation_3D(			// 初始化直线插补运动 
								m_hDevice,	     	// 设备句柄
								&m_DataList[gl_pADView->m_InterpAxis.Axis1],
								&m_InterpAxis,
								&m_LineData); 
		WTNMC4A_StartLineInterpolation_3D(m_hDevice);
		m_bAxisRun[m_InterpAxis.Axis1] = TRUE; 
		m_bAxisRun[m_InterpAxis.Axis2] = TRUE;
		m_bAxisRun[m_InterpAxis.Axis3] = TRUE;
	}
	for(int nAxis=0; nAxis<4; nAxis++)
	{
		if(!m_bAxisRun[nAxis])
			m_nCount[nAxis] = 0;
	}
	SetTimer(1, 10, NULL);
	SetTimer(3, 500, NULL);
	if (m_pXPulse != NULL)
	{
		free(m_pXPulse);
		m_pXPulse = NULL;
	}
	if(m_pYPulse != NULL)
	{
		free(m_pYPulse);
		m_pYPulse = NULL;
	}
	
	m_nIntCount = 0;
	m_pXPulse = (LONG*)malloc(sizeof(LONG));
	m_pYPulse = (LONG*)malloc(sizeof(LONG));
	m_nFunction = 1; // 直线插补运动

}

// 两轴圆弧插补(正方向或反方向)
void CADView::StartCircleInterpMovement(int nDirection)
{
	if(m_hDevice == INVALID_HANDLE_VALUE)
		return;
	OnClearCounter(); // 所有计数器清零
	for(int nAxis=0; nAxis<4; nAxis++)
	{
		WTNMC4A_PulseInputMode( m_hDevice,	
								nAxis,	
								gl_pADView->m_lPulseModeIN[nAxis] );
		if(!m_bAxisRun[nAxis])
			m_nCount[nAxis] = 0;
	
	}
	WTNMC4A_HanDec(m_hDevice,m_InterpAxis.Axis1,m_OtherPara[m_InterpAxis.Axis1].HandDecPulse);// 手动减速点设定
	if(nDirection == 0) // 正方向
	{
		WTNMC4A_InitCWInterpolation_2D(m_hDevice,// 初始化圆弧插补 
			&m_DataList[m_InterpAxis.Axis1],
			&m_InterpAxis,
			&m_CircleData);
		WTNMC4A_StartCWInterpolation_2D(m_hDevice, WTNMC4A_PDIRECTION);// 启动正方向圆弧插补					  
	}
	else // 反方向
	{
		WTNMC4A_InitCWInterpolation_2D(m_hDevice,// 初始化圆弧插补 
			&m_DataList[m_InterpAxis.Axis1],
			&m_InterpAxis,
			&m_CircleData);
		WTNMC4A_StartCWInterpolation_2D(m_hDevice, WTNMC4A_MDIRECTION); 	// 启动反方向圆弧插补					 		
	}
	m_bAxisRun[m_InterpAxis.Axis1] = TRUE;
	m_bAxisRun[m_InterpAxis.Axis2] = TRUE;
	SetTimer(1, 10, NULL);
	SetTimer(3, 100, NULL);

	if (m_pXPulse != NULL)
	{
		free(m_pXPulse);
		m_pXPulse = NULL;
	}
	if(m_pYPulse != NULL)
	{
		free(m_pYPulse);
		m_pYPulse = NULL;
	}
	
	m_nIntCount = 0;
	m_pXPulse = (LONG*)malloc(sizeof(LONG));
	m_pYPulse = (LONG*)malloc(sizeof(LONG));
	m_nFunction = 2; // 圆弧插补
}

// 单步插补
void CADView::StartSingleStepInterpMovement()
{
	SetTimer(3, 10, NULL);
	m_bDrawSingleStep = TRUE;

	WTNMC4A_StartSingleStepInterpolation(m_hDevice);

	
	RefreshStatusX();
    RefreshStatusY();	
	RefreshStatusZ();
	RefreshStatusU();
 	KillTimer(1);
// 	KillTimer(3);
}


CString CADView::GetAxisString(int nAxisNum)
{
	CString str;
	switch(nAxisNum)
	{
	case WTNMC4A_XAXIS:
		str = "X";
		break;
	case WTNMC4A_YAXIS:
		str = "Y";
		break;
	case WTNMC4A_ZAXIS:
		str = "Z";
		break;
	case WTNMC4A_UAXIS:
		str = "U";
		break;
	default:
		break;
	}
	return str;
}

// 开始跑道
void CADView::StartSequenceMovement()
{
	if(m_hDevice == INVALID_HANDLE_VALUE)
		return;
	OnClearCounter(); // 计数器清零
	pStartSequenceThread = AfxBeginThread(StartSequenceProc,m_hDevice, THREAD_PRIORITY_NORMAL, 0, CREATE_SUSPENDED,NULL);
	pStartSequenceThread->m_bAutoDelete = TRUE;
	pStartSequenceThread->ResumeThread();
	if (m_pXPulse != NULL)
	{
		free(m_pXPulse);
	}
	if(m_pYPulse != NULL)
	{
		free(m_pYPulse);
	}
	m_nIntCount = 0;
	m_pXPulse = (LONG*)malloc(sizeof(LONG));
	m_pYPulse = (LONG*)malloc(sizeof(LONG));
	m_bAxisRun[0] = TRUE;
	m_bAxisRun[1] = TRUE;
	SetTimer(1, 10, NULL);
	SetTimer(3, 100, NULL);
	m_nFunction = 3; // 连续插补 
}

// 位插补应用（画一个心形）
void CADView::StartBitInterpMovement()
{
	OnClearCounter();
	if (m_pXPulse != NULL)
	{
		free(m_pXPulse);
	}
	if(m_pYPulse != NULL)
	{
		free(m_pYPulse);
	}
  	m_nCount[0] = 0;
	m_nCount[1] = 0;
	m_nIntCount = 0;
	m_pXPulse = (LONG*)malloc(sizeof(LONG));
	m_pYPulse = (LONG*)malloc(sizeof(LONG));
	m_bAxisRun[0] = TRUE;
	m_bAxisRun[1] = TRUE;
	SetTimer(1, 10, NULL);
	SetTimer(3, 50, NULL);
	m_nStartBitMode = 1;
	m_nFunction = 4;
//	gl_hExitEvent = CreateEvent(0, 0 ,0);
	pStartBitThread = AfxBeginThread(StartBitProc, m_hDevice,  THREAD_PRIORITY_NORMAL, 0, CREATE_SUSPENDED);
//	gl_hEvent = CreateEvent();
	pStartBitThread->m_bAutoDelete = TRUE;
	pStartBitThread->ResumeThread();
}

// 启动位插补画一个六边形
void CADView::StartINTBitInterpMovement()
{
	OnClearCounter(); // 所有计数器清零
// 	if(m_hDevice == INVALID_HANDLE_VALUE)
// 		return;
// 	ULONG ulDataCount = 0;
// 	for(int nAxis=0; nAxis<4; nAxis++)
// 	{
// 		if(!m_bAxisRun[nAxis])
// 			m_nCount[nAxis] = 0;
// 	}
// 	WTNMC4A_PARA_DataList DataList;
// 	WTNMC4A_PARA_InterpolationAxis InterAxis;
// 	DataList.StartSpeed = 2;    // 初始速度
// 	DataList.DriveSpeed = 2;    // 驱动速度
// 	DataList.AccIncRate = 1;
// 	DataList.Acceleration = 1000;
// 	InterAxis.Axis1 = WTNMC4A_XAXIS;
// 	InterAxis.Axis2 = WTNMC4A_YAXIS;
	
// 	WTNMC4A_InitBitInterpolation_2D(m_hDevice, &InterAxis, &DataList);
// 	WTNMC4A_AutoBitInterpolation_2D(m_hDevice, nBitData, 24);  // 
	
	if (m_pXPulse != NULL)
	{
		free(m_pXPulse);
	}
	if(m_pYPulse != NULL)
	{
		free(m_pYPulse);
	}
	m_nCount[0] = 0;
	m_nCount[1] = 0;
	m_nIntCount = 0;
	m_nStartBitMode = 2;
	m_pXPulse = (LONG*)malloc(sizeof(LONG));
	m_pYPulse = (LONG*)malloc(sizeof(LONG));
	m_bAxisRun[0] = TRUE;
	m_bAxisRun[1] = TRUE;
	SetTimer(1, 10, NULL);
	SetTimer(3, 50, NULL);
	m_nFunction = 5; // 位插补(六边形)
// AfxBeginThread(SetBitDataThread_2D,hDevice, THREAD_PRIORITY_NORMAL, 0, CREATE_SUSPENDED, NULL);
	pStartBitThread = AfxBeginThread(SetBitDataThread_2D, m_hDevice,  THREAD_PRIORITY_NORMAL, 0, CREATE_SUSPENDED);
	//	gl_hEvent = CreateEvent();
	pStartBitThread->m_bAutoDelete = TRUE;
	pStartBitThread->ResumeThread();

// 	WTNMC4A_InitBitInterpolation_2D(m_hDevice, &InterAxis, &DataList);// 初始化任意2轴位插补参数
// 	WTNMC4A_AutoBitInterpolation_2D(m_hDevice, nBitData, 24);  // 启动任意2轴位插补子线程



}


// 初始化同步轴的参数
void CADView::SetSynchronPara(int nAxisNum)
{
	// 如果选择了LP/EP >= COMP+ 或 LP/EP < COMP+
	if(m_SynchronActionOwnAxis[nAxisNum].PBCP || m_SynchronActionOwnAxis[nAxisNum].PSCP)
		WTNMC4A_SetCOMPP(m_hDevice,	nAxisNum,m_nLPEP[nAxisNum], m_nSynchronCOMPPValue[nAxisNum]); // 设置COMP+寄存器
	// 如果选择了LP/EP >= COMP- 或 LP/EP < COMP-
	if(m_SynchronActionOwnAxis[nAxisNum].PBCM || m_SynchronActionOwnAxis[nAxisNum].PSCM)
		WTNMC4A_SetCOMPM(m_hDevice,	nAxisNum,m_nLPEP[nAxisNum], m_nSynchronCOMPNValue[nAxisNum]); // 设置COMP-寄存器

	WTNMC4A_SetSynchronAction(  // 设置同步轴参数
		m_hDevice,				// 设备句柄
		nAxisNum,			    // 轴号(WTNMC4A_XAXIS:X轴,WTNMC4A_YAXIS:Y轴, WTNMC4A_ZAXIS:Z轴,WTNMC4A_UAXIS:U轴) 
		&m_SynchronActionOwnAxis[nAxisNum],	  // 自己轴的参数结构体指针
		&m_SynchronActionOtherAxis[nAxisNum]);
}

void CADView::ResetDevice(HANDLE hDevice)
{
	m_PageComSet.m_Combo_ImpulseMode.SetCurSel(0);
	m_PageComSet.m_Combo_ImpulseModeIN.SetCurSel(0);
	m_PageComSet.m_Combo_PulseLogLever.SetCurSel(0);
	m_PageComSet.m_Combo_DirLogLever.SetCurSel(0);
	m_PageComSet.m_Edit_AccOffset.SetWindowText(L"0");
	OnClearCounter();	

	WTNMC4A_Reset(hDevice);

	for(int Index=0; Index<4; Index++)	
	{
		if(!WTNMC4A_ClearSoftwareLimit(m_hDevice, Index))
		{
			AfxMessageBox(L"清除软件限位失败！");
			return;
		}
		
		m_bSLimit[Index] = FALSE;
		m_PageComSet.InitCfg(Index);
		
	}	
	
	for(int i=0; i<4; i++)
	{
		m_nCount[i] = 0;
	}

	RefreshStatusX();
	RefreshStatusY();	
	RefreshStatusZ();
	RefreshStatusU();
	m_nIntCount = 0;
	
	SendMessage(WM_DRAW_LINE, NULL, NULL); // 直线运动
	SendMessage(WM_DRAW_LINEINTERPOLATION, NULL, NULL); // 直线插补
	m_PageComSet.RedrawWindow(NULL, NULL, RDW_INVALIDATE|RDW_UPDATENOW);
	
// 	int nHandDecNum;
// 	CString str;
// 	m_PageLine.m_Edit_HandDecNum.GetWindowText(str);
// 	nHandDecNum = wcstol(str, NULL, 10);
// 	gl_pADView->m_OtherPara[m_nCurrentAxis].HandDecPulse = nHandDecNum;
// 	WTNMC4A_HanDec(gl_pADView->m_hDevice, m_nCurrentAxis, nHandDecNum);

}

void CADView::OnBUTTONReset() 
{
	ResetDevice(m_hDevice);
}

// 开关量输出
void CADView::SetSwitchOut(int nAxisNum)
{
	if(m_hDevice == INVALID_HANDLE_VALUE)
		return;
	WTNMC4A_SetDeviceDO(m_hDevice, nAxisNum, &m_ParaDO[nAxisNum]); // 数字量输出
}

// 开关量输入
void CADView::GetSwitchDI()
{
	if(m_hDevice == INVALID_HANDLE_VALUE)
		return;
	WTNMC4A_GetRR3Status(m_hDevice, &m_ParaRR3);
	WTNMC4A_GetRR4Status(m_hDevice, &m_ParaRR4);
	m_PageDIO.RefreshButton(&m_ParaRR3, &m_ParaRR4);
}

// 设置原点搜寻时的一控件的可用与否
void CADView::SetHomeSearchWnd()
{
	
}

// 开始自动原点搜寻
void CADView::StartAutoHomeSearch(int nAxisNum)
{
	OnClearCounter();	 	
	// IN0,IN1,IN2默认是高电平，有效电平是低电平
	WTNMC4A_SetInEnable(m_hDevice, nAxisNum, 0, 0); // 设置IN0的有效电平
	WTNMC4A_SetInEnable(m_hDevice, nAxisNum, 1, 0); // 设置IN1的有效电平
	WTNMC4A_SetInEnable(m_hDevice, nAxisNum, 2, 0); // 设置IN2的有效电平
	
    WTNMC4A_SetAutoHomeSearch(  // 设置原点搜寻参数
				m_hDevice,	    // 设备句柄
				nAxisNum,		// 轴号(WTNMC4A_XAXIS:X轴,WTNMC4A_YAXIS:Y轴, WTNMC4A_ZAXIS:Z轴,WTNMC4A_UAXIS:U轴)
				&m_HomeSearchPara[nAxisNum]);

// 	WTNMC4A_SetEncoderSignalType(m_hDevice,WTNMC4A_XAXIS, 1, 0);		// 上下脉冲方式
// 	WTNMC4A_SetEncoderSignalType(m_hDevice, WTNMC4A_YAXIS, 1, 0);		// 上下脉冲方式
// 	WTNMC4A_SetEncoderSignalType(m_hDevice, WTNMC4A_ZAXIS, 1, 0);		// 上下脉冲方式
// 	WTNMC4A_SetEncoderSignalType(m_hDevice, WTNMC4A_UAXIS, 1, 0);		// 上下脉冲方式
	WTNMC4A_PulseInputMode( m_hDevice,	
								nAxisNum,	
								gl_pADView->m_lPulseModeIN[nAxisNum] );
	WTNMC4A_InitLVDV(m_hDevice, &m_DataList[m_nCurrentAxis], &m_LCData[m_nCurrentAxis]);
	WTNMC4A_SetHV(m_hDevice, nAxisNum, m_nHomeLowSpeed); // 设置低速原点搜寻速度

	WTNMC4A_StartAutoHomeSearch(
					 m_hDevice,			// 设备句柄		
					 nAxisNum);
	EnableWindows(FALSE);
	m_bAxisRun[nAxisNum] = TRUE;
	m_nCount[nAxisNum] = 0;
	SetTimer(1, 10 , NULL);  // 启动定时器1，用来刷新计数器的状态
	SetTimer(3, 50, NULL);   // 启动定时器3，用来读取轴的当前速度
	m_nFunction = 0;         // 直线运动
}

// 启动同步运动
void CADView::StartSynchronMovement(int nStartAxis)
{
	OnClearCounter();      // 清除所有的计数器
	int nSynchronAixsNum;  // 同步轴的轴号
	m_nCount[nStartAxis] = 0;
	for(int i = 0;i<4;i++)
	{
		WTNMC4A_PulseInputMode( m_hDevice,	
								i,	
								gl_pADView->m_lPulseModeIN[i] );
	}
// 	WTNMC4A_SetEncoderSignalType(m_hDevice,WTNMC4A_XAXIS, 1, 0);		// 上下脉冲方式
// 	WTNMC4A_SetEncoderSignalType(m_hDevice, WTNMC4A_YAXIS, 1, 0);		// 上下脉冲方式
// 	WTNMC4A_SetEncoderSignalType(m_hDevice, WTNMC4A_ZAXIS, 1, 0);		// 上下脉冲方式
// 	WTNMC4A_SetEncoderSignalType(m_hDevice, WTNMC4A_UAXIS, 1, 0);		// 上下脉冲方式
	WTNMC4A_PulseInputMode( m_hDevice,	
							nStartAxis,	
							gl_pADView->m_lPulseModeIN[nStartAxis] );
	if(!WTNMC4A_InitLVDV(m_hDevice, &m_DataList[nStartAxis],&m_LCData[nStartAxis]))
	{
		AfxMessageBox(L"初始化单轴直线动失败！");
	}
	
	SetSynchronPara(nStartAxis); // 设置同步参数		
	if(m_SynchronActionOwnAxis[nStartAxis].AXIS1) // 如果选择同步的轴1
	{
		nSynchronAixsNum = (nStartAxis+1)%4;
	//	WTNMC4A_SetEncoderSignalType(m_hDevice, m_SynchronActionOwnAxis[nStartAxis].AXIS1, 1, 0);		// 上下脉冲方式
	
		if(!WTNMC4A_InitLVDV(m_hDevice,&m_DataList[nSynchronAixsNum],&m_LCData[nSynchronAixsNum]))
		{
			AfxMessageBox(L"初始化单轴直线动失败！");
		}
		SetSynchronPara(nSynchronAixsNum); // 设置同步参数
	}
	if(m_SynchronActionOwnAxis[nStartAxis].AXIS2) // 如果选择同步的轴2
	{
		nSynchronAixsNum = (nStartAxis+2)%4;
		//WTNMC4A_SetEncoderSignalType(m_hDevice, m_SynchronActionOwnAxis[nStartAxis].AXIS2, 1, 0);		// 上下脉冲方式
		if(!WTNMC4A_InitLVDV(m_hDevice,&m_DataList[nSynchronAixsNum],&m_LCData[nSynchronAixsNum]))
		{
			AfxMessageBox(L"初始化单轴直线运动失败！");
		}
		SetSynchronPara(nSynchronAixsNum); // 设置同步参数
	}
	if(m_SynchronActionOwnAxis[nStartAxis].AXIS3) // 如果选择同步的轴3
	{
		nSynchronAixsNum = (nStartAxis+3)%4;
		//WTNMC4A_SetEncoderSignalType(m_hDevice, m_SynchronActionOwnAxis[nStartAxis].AXIS3, 1, 0);		// 上下脉冲方式
		if(!WTNMC4A_InitLVDV(m_hDevice,    // 初始化直线运动
			&m_DataList[nSynchronAixsNum],
			&m_LCData[nSynchronAixsNum]))
		{
			AfxMessageBox(L"初始化单轴直线运动失败！");
		}
		SetSynchronPara(nSynchronAixsNum); // 设置同步参数
	}

		if(!WTNMC4A_StartLVDV(m_hDevice, nStartAxis)) //	启动单轴nStartAxis
		{
			AfxMessageBox(L"单轴启动失败！");
			return;
		}
		m_bAxisRun[nStartAxis] = TRUE;
		EnableWindows(FALSE);
		SetTimer(1, 10 , NULL);  // 启动定时器1，用来刷新计数器的状态
		SetTimer(3, 50, NULL);   // 启动定时器3，用来读取轴的当前速度
		m_nFunction = 0;         // 直线运动
		
}

void CADView::OnSelchangeTABComSet(NMHDR* pNMHDR, LRESULT* pResult)
{
	// TODO: Add your control notification handler code here
	SetFocus()->ActivateTopParent();
	ULONG UL = GetFocus()->GetDlgCtrlID();
	m_nCurrentAxis = m_TabComSet.GetCurSel();
	m_PageComSet.SetCurrentAxisNum(m_nCurrentAxis); // 设置公用参数页的当前轴号
	m_PageComSet.InitCfg(m_nCurrentAxis);	        // 初始化对应的轴号的参数
	m_PageLine.InitCfg(m_nCurrentAxis);             // 初始化直线运动参数
	m_PageHardLimit.InitCfg(m_nCurrentAxis);        // 初始化硬件限位参数
	*pResult = 0;
}

// X轴中断等待函数
UINT StartINTWaitX(PVOID hWnd)
{
	int Index=0;
	gl_bWaitInt = TRUE;
//	BYTE IntSrc[16];
	CADView* pView = (CADView*)hWnd;
	while(gl_bWaitInt)
	{
		WaitForSingleObject(gl_hEventInt[0], INFINITE);
	//	Beep(3000, 1);
		goto EXIT;
	}
EXIT:
	gl_bWaitInt = FALSE;
	Beep(3000, 1);
//	WTNMC4A_GetDeviceIntSrc(pView->m_hDevice, IntSrc);
//	AfxMessageBox(L"X轴中断来了");
	return 0;
}

// Y轴中断等待函数
UINT StartINTWaitY(PVOID hWnd)
{
	int Index=0;
	while(gl_bWaitInt)
	{
		WaitForSingleObject(gl_hEventInt[1], INFINITE);
		switch(Index)
		{
		case 0:
			AfxMessageBox(L"Y轴中断来了");
			gl_bWaitInt = FALSE;
			break;
		}
	}
	return 0;
}

// Z轴中断等待函数
UINT StartINTWaitZ(PVOID hWnd)
{
	int Index=0;
	while(gl_bWaitInt)
	{
		WaitForSingleObject(gl_hEventInt[2], INFINITE);
		switch(Index)
		{
		case 0:
			AfxMessageBox(L"Z轴中断来了");
			gl_bWaitInt = FALSE;
			break;
		}
	}
	return 0;
}

// U轴中断等待函数
UINT StartINTWaitU(PVOID hWnd)
{
	int Index=0;
	while(gl_bWaitInt)
	{
		WaitForSingleObject(gl_hEventInt[3], INFINITE);
		switch(Index)
		{
		case 0:
			AfxMessageBox(L"U轴中断来了");
			gl_bWaitInt = FALSE;
			break;
		}
	}
	return 0;
}
// 设置中断位，并启动等待中断的子线程
void CADView::SetInterrruptBit(int nAxis)
{
	typedef UINT  (*PTHREADFUNC)(PVOID hWnd);
	PTHREADFUNC pThreadFunc[4];
	pThreadFunc[0] = (PTHREADFUNC)StartINTWaitX;
	pThreadFunc[1] = StartINTWaitY;
	pThreadFunc[2] = StartINTWaitZ;
	pThreadFunc[3] = StartINTWaitU;
	if(nAxis != WTNMC4A_ALLAXIS)
	{
	// 如果选择了LP/EP >= COMP+ 或 LP/EP < COMP+
	if(m_Interrupt[nAxis].PBCP || m_Interrupt[nAxis].PSCP)
		WTNMC4A_SetCOMPP(m_hDevice,	nAxis,m_nLPEP[nAxis], m_nSynchronCOMPPValue[nAxis]); // 设置COMP+寄存器
	// 如果选择了LP/EP >= COMP- 或 LP/EP < COMP-
	if(m_Interrupt[nAxis].PBCM || m_Interrupt[nAxis].PSCM)
		WTNMC4A_SetCOMPM(m_hDevice,	nAxis,m_nLPEP[nAxis], m_nSynchronCOMPNValue[nAxis]); // 设置COMP-寄存器

		WTNMC4A_SetInterruptBit(    // 设置中断位		
					m_hDevice,		// 设备句柄
					nAxis,		// 轴号
					&m_Interrupt[nAxis]);	// 中断位结构体指针
		gl_hEventInt[nAxis] = CreateEvent(NULL, FALSE, FALSE, NULL); // 信号量
//		WTNMC4A_InitDeviceInt(m_hDevice, gl_hEventInt[nAxis]);       // 初始化INT
		pStartIntWaitThread[nAxis] = AfxBeginThread(pThreadFunc[nAxis], this, THREAD_PRIORITY_NORMAL, NULL, CREATE_SUSPENDED);
		pStartIntWaitThread[nAxis]->m_bAutoDelete = TRUE;
		pStartIntWaitThread[nAxis]->ResumeThread(); // 启动线程
	}
	else
	{
		for(int i=0; i<4; i++)
		{
			WTNMC4A_SetInterruptBit(// 设置中断位		
						m_hDevice,		// 设备句柄
						i,		// 轴号
						&m_Interrupt[i]);	// 中断位结构体指针
			gl_hEventInt[i] = CreateEvent(NULL, FALSE, FALSE, NULL); // 信号量
		//	WTNMC4A_InitDeviceInt(m_hDevice, gl_hEventInt[i]);       // 初始化INT
			pStartIntWaitThread[i] = AfxBeginThread(pThreadFunc[i], this, THREAD_PRIORITY_NORMAL, NULL, CREATE_SUSPENDED);
			pStartIntWaitThread[i]->m_bAutoDelete = TRUE;
			pStartIntWaitThread[i]->ResumeThread(); // 启动线程
		}
	}

}
