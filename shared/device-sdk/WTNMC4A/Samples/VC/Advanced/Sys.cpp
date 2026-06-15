// Sys.cpp : Defines the class behaviors for the application.
//

#include "stdafx.h"
#include "Sys.h"
#include "SelDevNetDlg.h"
#include "DevNetCfgDlg.h"

#include "MainFrm.h"
#include "ADFrm.h"
#include "ADDoc.h"
#include "ADView.h"

#ifdef _DEBUG
#define new DEBUG_NEW
#undef THIS_FILE
static char THIS_FILE[] = __FILE__;
#endif

/////////////////////////////////////////////////////////////////////////////
// CSysApp
HANDLE  gl_hDevice;
BEGIN_MESSAGE_MAP(CSysApp, CWinApp)
	//{{AFX_MSG_MAP(CSysApp)
	ON_COMMAND(ID_APP_ABOUT, OnAppAbout)
	ON_COMMAND(ID_OPEN_AD, OnOpenAD)
	//}}AFX_MSG_MAP
	// Standard file based document commands
	ON_COMMAND(ID_FILE_NEW, CWinApp::OnFileNew)
	ON_COMMAND(ID_FILE_OPEN, CWinApp::OnFileOpen)
	// Standard print setup command
	ON_COMMAND(ID_FILE_PRINT_SETUP, CWinApp::OnFilePrintSetup)
	ON_COMMAND(IDM_NET_CFG, &CSysApp::OnNetCfg)
	ON_COMMAND(IDM_ListDeviceInfo, &CSysApp::OnListdeviceinfo)
END_MESSAGE_MAP()

/////////////////////////////////////////////////////////////////////////////
// CSysApp construction

CSysApp::CSysApp()
{
	// TODO: add construction code here,
	// Place all significant initialization in InitInstance
}

/////////////////////////////////////////////////////////////////////////////
// The one and only CSysApp object

CSysApp theApp;

/////////////////////////////////////////////////////////////////////////////
// CSysApp initialization

BOOL CSysApp::InitInstance()
{
// 	BOOL bCreate=FALSE;
// 	HANDLE hDeviceTemp;  // 
// 	m_CurrentDeviceID=0;   // 指定当前设备的ID标示符
// 	/////
// 	int DeviceCount;
// 	WCHAR szExeName[256];
// 	hDeviceTemp = WTNMC4A_DEV_CreateA(0); // 创建设备对象，保存在App中，可供其他任何子窗体或子功能使用
// 	DeviceCount = WTNMC4A_GetDeviceCount(hDeviceTemp);
// 	WTNMC4A_DEV_Release(hDeviceTemp);
// 
// 	for(int i=0; i<DeviceCount; i++)
// 	{
// 		swprintf_s(szExeName, L"WTNMC4A-%d", i);
// 		
// 		// 创建互斥对象
// 		m_hMutex=::CreateMutex(NULL, NULL, szExeName);  // m_pszExeName为本程序的执行名
// 		if(GetLastError()==ERROR_ALREADY_EXISTS)  // 第二次创建应用程序
// 		{
// 			bCreate=FALSE;
// 			continue;  // 如果已经创建，则继续下一个设备的应用程序创建
// 		}
// 		else
// 		{ 	
// 			bCreate=TRUE;
// 			m_CurrentDeviceID = i;
// 			break;
// 		}
// 		
// 	}
// 	
// 	if(DeviceCount!=0)
// 	{
// 		if(bCreate==FALSE)  // 当该实例不能被创建时
// 		{
// 			AfxMessageBox(L"对不起，您的所有设备已被相应程序管理，您不能再创建新实例...",MB_ICONWARNING,0);
// 			return FALSE; 
// 		}
// 		m_hDeviceApp = WTNMC4A_DEV_CreateA(m_CurrentDeviceID); // 创建设备对象，保存在App中，可供其他任何子窗体或子功能使用
// 		gl_hDevice = m_hDeviceApp;
// //	CString str;
// //str.Format(L"CWinApp:gl_hDevice=%x", gl_hDevice);
// ///	AfxMessageBox(str);
// //
// //				CString str4;
// //	str4.Format(L"m_hDevice=%x", gl_hDevice);
// //	AfxMessageBox(str4);
// 
// 	}
// 	else
// 	{
// 		m_hDeviceApp = INVALID_HANDLE_VALUE;
// 	}


	CSelDevNetDlg SelDlg;
	m_bLinkSuccess = FALSE;
	m_hDeviceApp = INVALID_HANDLE_VALUE;
	if(SelDlg.DoModal() == IDOK)
	{
		if (m_bLinkSuccess)
		{
			m_hDeviceApp = WTNMC4A_DEV_CreateW(m_IPAddr, m_nRSTimeout, m_nRSTimeout);
		}
	}
	else
	{
		return FALSE;
	}
	
	

	//if(m_CurrentDeviceID<DeviceCount) AfxMessageBox(L"您可以再启动该应用程序实例来管理下一个设备");
	///////////////////////////////////////////////
	// 判断用户的显示器模式是否为1024*768
	int Len = GetSystemMetrics(SM_CXSCREEN);  // 取得屏幕宽度
	if(Len<1024) // 如果屏幕宽度大小1024，则
	{
		if(AfxMessageBox(L"请最好使用1024*768或以上的显示器分辨率，继续吗？",MB_ICONWARNING|MB_YESNO,0)==IDNO)	
		{
			ExitInstance();
			return FALSE;
		}
	}
	
	AfxEnableControlContainer();

	// Standard initialization
	// If you are not using these features and wish to reduce the size
	//  of your final executable, you should remove from the following
	//  the specific initialization routines you do not need.

#ifdef _AFXDLL
	Enable3dControls();			// Call this when using MFC in a shared DLL
#else
//	Enable3dControlsStatic();	// Call this when linking to MFC statically
#endif

	// Change the registry key under which our settings are stored.
	// TODO: You should modify this string to be something appropriate
	// such as the name of your company or organization.
	SetRegistryKey(_T("Local AppWizard-Generated Applications"));

	LoadStdProfileSettings();  // Load standard INI file options (including MRU)

	// Register the application's document templates.  Document templates
	//  serve as the connection between documents, frame windows and views.


	pADTemplate = new CMultiDocTemplate(
		IDR_SYSTYPE,
		RUNTIME_CLASS(CADDoc),
		RUNTIME_CLASS(CADFrame), // custom MDI child frame
		RUNTIME_CLASS(CADView));
	AddDocTemplate(pADTemplate);

	// create main MDI Frame window
	CMainFrame* pMainFrame = new CMainFrame;
	if (!pMainFrame->LoadFrame(IDR_MAINFRAME))
		return FALSE;
	m_pMainWnd = pMainFrame;

	// Parse command line for standard shell commands, DDE, file open
	CCommandLineInfo cmdInfo;
	ParseCommandLine(cmdInfo);

	// Dispatch commands specified on the command line
//	if (!ProcessShellCommand(cmdInfo))
//		return FALSE;

	// The main window has been initialized, so show and update it.
	m_nCmdShow = SW_SHOWMAXIMIZED;	
	
	m_pMainWnd->DragAcceptFiles();  // 支持拖放功能	
	::SetProp(m_pMainWnd->GetSafeHwnd(), m_pszExeName, (HANDLE)1);	
	//LONG DeviceLgcID, DevicePhysID;
	CString MainFrmName; WCHAR str[100];
	//swprintf_s(str, L"WTNMC4A-%d-%d ", DeviceLgcID, DevicePhysID);
	swprintf_s(str, L"%s-%s ", L"WTNMC4A",	m_IPAddr);
	MainFrmName = pMainFrame->GetTitle();
	MainFrmName = str + MainFrmName;
	pMainFrame->SetTitle(MainFrmName);
	
	pMainFrame->ShowWindow(m_nCmdShow);
	pMainFrame->UpdateWindow();	
	OnOpenAD();
	return TRUE;
}


/////////////////////////////////////////////////////////////////////////////
// CAboutDlg dialog used for App About

class CAboutDlg : public CDialog
{
public:
	CAboutDlg();

// Dialog Data
	//{{AFX_DATA(CAboutDlg)
	enum { IDD = IDD_ABOUTBOX };
	//}}AFX_DATA

	// ClassWizard generated virtual function overrides
	//{{AFX_VIRTUAL(CAboutDlg)
	protected:
	virtual void DoDataExchange(CDataExchange* pDX);    // DDX/DDV support
	//}}AFX_VIRTUAL
protected:
	CFont 	m_fontLogo;
	CString m_LogoText;
	void SetLogoFont(CString Name, int nHeight = 24, int nWeight = FW_BOLD,
		BYTE bItalic = true, BYTE bUnderline = false);
	void SetLogoText(CString Text);
// Implementation
protected:
	//{{AFX_MSG(CAboutDlg)
	virtual BOOL OnInitDialog();
	afx_msg void OnPaint();
	//}}AFX_MSG
	DECLARE_MESSAGE_MAP()
};

CAboutDlg::CAboutDlg() : CDialog(CAboutDlg::IDD)
{
	//{{AFX_DATA_INIT(CAboutDlg)
	//}}AFX_DATA_INIT
}

void CAboutDlg::DoDataExchange(CDataExchange* pDX)
{
	CDialog::DoDataExchange(pDX);
	//{{AFX_DATA_MAP(CAboutDlg)
	//}}AFX_DATA_MAP
}

BEGIN_MESSAGE_MAP(CAboutDlg, CDialog)
	//{{AFX_MSG_MAP(CAboutDlg)
	ON_WM_PAINT()
	//}}AFX_MSG_MAP
END_MESSAGE_MAP()

void CAboutDlg::SetLogoFont(CString Name, int nHeight/* = 24*/,
	int nWeight/* = FW_BOLD*/, BYTE bItalic/* = true*/, BYTE bUnderline/* = false*/)
{
	if(m_fontLogo.m_hObject)
		m_fontLogo.Detach();
	m_fontLogo.CreateFont(nHeight, 0, 0, 0, nWeight, bItalic, bUnderline,0,0,0,0,0,0, Name);
}

void CAboutDlg::SetLogoText(CString Text)
{
	m_LogoText = Text;
}

BOOL CAboutDlg::OnInitDialog() 
{
	CDialog::OnInitDialog();
	
	// TODO: Add extra initialization here
	SetLogoFont("Arial");
	SetLogoText("北京阿尔泰科技有限公司");
	return TRUE;  // return TRUE unless you set the focus to a control
	              // EXCEPTION: OCX Property Pages should return FALSE
}
void CAboutDlg::OnPaint() 
{
	CPaintDC dc(this); // device context for painting
	CRect rectText;
	GetClientRect(rectText);
	rectText.top = rectText.bottom-40;
	rectText.left = 100;

	dc.SetBkMode(TRANSPARENT);
	// TODO: Add your message handler code here
	CFont * OldFont = dc.SelectObject(&m_fontLogo);
	// draw text in DC
	COLORREF OldColor = dc.SetTextColor( ::GetSysColor( COLOR_3DHILIGHT));
	dc.DrawText( m_LogoText, rectText + CPoint(1,1), DT_SINGLELINE | DT_LEFT | DT_VCENTER);
	dc.SetTextColor( RGB(72,72,72));//::GetSysColor( COLOR_3DSHADOW));
	dc.DrawText( m_LogoText, rectText, DT_SINGLELINE | DT_LEFT | DT_VCENTER);

	// restore old text color
	dc.SetTextColor( OldColor);
	// restore old font
	dc.SelectObject(OldFont);
	// Do not call CDialog::OnPaint() for painting messages
}

// App command to run the dialog
void CSysApp::OnAppAbout()
{
	CAboutDlg aboutDlg;
	aboutDlg.DoModal();
}



/////////////////////////////////////////////////////////////////////////////
// CSysApp message handlers

void CSysApp::OnOpenAD() 
{
	CDocument *pDoc;
	pDoc =  pADTemplate->CreateNewDocument();
	m_pADDoc = (CADDoc*)pDoc;
	m_pADFrm = (CADFrame*)pADTemplate->CreateNewFrame(m_pADDoc, NULL);
	pADTemplate->InitialUpdateFrame(m_pADFrm, m_pADDoc, TRUE);
	pDoc->SetTitle(L"步进电机控制");
}

int CSysApp::ExitInstance() 
{
	// TODO: Add your specialized code here and/or call the base class
 	if(m_hDeviceApp!=INVALID_HANDLE_VALUE) WTNMC4A_DEV_Release(m_hDeviceApp);	
	ReleaseMutex(m_hMutex);
	return CWinApp::ExitInstance();
}




void CSysApp::OnNetCfg()
{
	// TODO: 在此添加命令处理程序代码

	CDevNetCfgDlg SelDlg;

	if(SelDlg.DoModal() == IDCANCEL)
		return ;
}

void CSysApp::OnListdeviceinfo()
{
	// TODO: 在此添加命令处理程序代码
	if (m_hDeviceApp == INVALID_HANDLE_VALUE)
		return;

	CString strMsg;
	U32 nDllVer, nDriverVer, nFirmwareVer;
	if(!WTNMC4A_DEV_GetVersion(m_hDeviceApp, &nDllVer, &nDriverVer, &nFirmwareVer))
	{
		AfxMessageBox(L"读取版本信息失败");
		return;
	}

	ULONG nSerialNum;
	if(!WTNMC4A_DEV_GetSerialNum(m_hDeviceApp, &nSerialNum))
	{
		AfxMessageBox(L"读取序列号失败");
		return;
	}

	ULONG nUserPID;
	if(!WTNMC4A_DEV_GetUserPID(m_hDeviceApp, &nUserPID))
	{
		AfxMessageBox(L"读取用户PID失败");
		return;
	}

	strMsg.Format(L"动态库版本号: %u\n驱动程序版本号: %u\n固件版本号: %u\n产品序列号: %u\n用户PID: %u", nDllVer, nDriverVer, nFirmwareVer, nSerialNum, nUserPID);
	AfxMessageBox(strMsg);
}
