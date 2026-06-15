// InterruptSetDlg.cpp : implementation file
//

#include "stdafx.h"
#include "sys.h"
#include "InterruptSetDlg.h"
#include "ADView.h"
#ifdef _DEBUG
#define new DEBUG_NEW
#undef THIS_FILE
static char THIS_FILE[] = __FILE__;
#endif
extern CADView* gl_pADView;
/////////////////////////////////////////////////////////////////////////////
// CInterruptSetDlg dialog


CInterruptSetDlg::CInterruptSetDlg(CWnd* pParent /*=NULL*/)
	: CDialog(CInterruptSetDlg::IDD, pParent)
{
	//{{AFX_DATA_INIT(CInterruptSetDlg)
	//}}AFX_DATA_INIT
}


void CInterruptSetDlg::DoDataExchange(CDataExchange* pDX)
{
	CDialog::DoDataExchange(pDX);
	//{{AFX_DATA_MAP(CInterruptSetDlg)
	DDX_Control(pDX, IDC_CHECK_INT_PULSE, m_Button_PULSE);
	DDX_Control(pDX, IDC_CHECK_INT_PSCP, m_Button_PSCP);
	DDX_Control(pDX, IDC_CHECK_INT_PSCM, m_Button_PSCM);
	DDX_Control(pDX, IDC_CHECK_INT_PBCP, m_Button_PBCP);
	DDX_Control(pDX, IDC_CHECK_INT_PBCM, m_Button_PBCM);
	DDX_Control(pDX, IDC_CHECK_INT_DEND, m_Button_DEND);
	DDX_Control(pDX, IDC_CHECK_INT_CSTA, m_Button_CSTA);
	DDX_Control(pDX, IDC_CHECK_INT_CIINT, m_Button_CIINT);
	DDX_Control(pDX, IDC_CHECK_INT_CDEC, m_Button_CDEC);
	DDX_Control(pDX, IDC_CHECK_INT_BPINT, m_Button_BPINT);
	DDX_Control(pDX, IDC_EDIT_INT_COMP, m_Edit_COMP);
	DDX_Control(pDX, IDC_EDIT_INT_COMN, m_Edit_COMN);
	//}}AFX_DATA_MAP
}


BEGIN_MESSAGE_MAP(CInterruptSetDlg, CDialog)
	//{{AFX_MSG_MAP(CInterruptSetDlg)
	ON_BN_CLICKED(IDC_CHECK_INT_PBCM, OnCheckIntPbcm)
	ON_BN_CLICKED(IDC_CHECK_INT_PSCM, OnCheckIntPscm)
	ON_BN_CLICKED(IDC_CHECK_INT_PSCP, OnCheckIntPscp)
	ON_BN_CLICKED(IDC_CHECK_INT_PBCP, OnCheckIntPbcp)
	ON_BN_CLICKED(IDC_CHECK_INT_PULSE, OnCheckIntPulse)
	ON_BN_CLICKED(IDC_CHECK_INT_CDEC, OnCheckIntCdec)
	ON_BN_CLICKED(IDC_CHECK_INT_CSTA, OnCheckIntCsta)
	ON_BN_CLICKED(IDC_CHECK_INT_DEND, OnCheckIntDend)
	ON_BN_CLICKED(IDC_CHECK_INT_CIINT, OnCheckIntCiint)
	ON_BN_CLICKED(IDC_CHECK_INT_BPINT, OnCheckIntBpint)
	ON_EN_CHANGE(IDC_EDIT_INT_COMP, OnChangeEditIntComp)
	ON_EN_CHANGE(IDC_EDIT_INT_COMN, OnChangeEditIntComn)
	
	//}}AFX_MSG_MAP
END_MESSAGE_MAP()

/////////////////////////////////////////////////////////////////////////////
// CInterruptSetDlg message handlers

void CInterruptSetDlg::OnCheckIntPbcm() 
{
	// TODO: Add your control notification handler code here
	int nCheck = m_Button_PBCM.GetCheck();
	if(nCheck)
	{
		m_Button_PSCM.SetCheck(0);
		gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCM = !nCheck;
	}
	gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCM = nCheck;

	if(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCP || gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCP)
		WTNMC4A_SetCOMPP(gl_pADView->m_hDevice,	
						gl_pADView->m_nCurrentAxis,
						gl_pADView->m_nLPEP[gl_pADView->m_nCurrentAxis], 
						gl_pADView->m_nSynchronCOMPPValue[gl_pADView->m_nCurrentAxis]); // 设置COMP+寄存器
	// 如果选择了LP/EP >= COMP- 或 LP/EP < COMP-
	if(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCM || gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCM)
		WTNMC4A_SetCOMPM(gl_pADView->m_hDevice,	
						gl_pADView->m_nCurrentAxis,
						gl_pADView->m_nLPEP[gl_pADView->m_nCurrentAxis], 
						gl_pADView->m_nSynchronCOMPNValue[gl_pADView->m_nCurrentAxis]); // 设置COMP-寄存器

		WTNMC4A_SetInterruptBit(    // 设置中断位		
					gl_pADView->m_hDevice,		// 设备句柄
					gl_pADView->m_nCurrentAxis,		// 轴号
					&gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis]);
}

void CInterruptSetDlg::OnCheckIntPscm() 
{
	// TODO: Add your control notification handler code here
	int nCheck = m_Button_PSCM.GetCheck();
	if(nCheck)
	{
		m_Button_PBCM.SetCheck(0);
		gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCM = !nCheck;
	}
	gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCM = nCheck;

	if(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCP || gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCP)
		WTNMC4A_SetCOMPP(gl_pADView->m_hDevice,	
						gl_pADView->m_nCurrentAxis,
						gl_pADView->m_nLPEP[gl_pADView->m_nCurrentAxis], 
						gl_pADView->m_nSynchronCOMPPValue[gl_pADView->m_nCurrentAxis]); // 设置COMP+寄存器
	// 如果选择了LP/EP >= COMP- 或 LP/EP < COMP-
	if(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCM || gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCM)
		WTNMC4A_SetCOMPM(gl_pADView->m_hDevice,	
						gl_pADView->m_nCurrentAxis,
						gl_pADView->m_nLPEP[gl_pADView->m_nCurrentAxis], 
						gl_pADView->m_nSynchronCOMPNValue[gl_pADView->m_nCurrentAxis]); // 设置COMP-寄存器

		WTNMC4A_SetInterruptBit(    // 设置中断位		
					gl_pADView->m_hDevice,		// 设备句柄
					gl_pADView->m_nCurrentAxis,		// 轴号
					&gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis]);
}

void CInterruptSetDlg::OnCheckIntPscp() 
{
	// TODO: Add your control notification handler code here
	int nCheck = m_Button_PSCP.GetCheck();
	if(nCheck)
	{
		m_Button_PBCP.SetCheck(0);
		gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCP = !nCheck;
	}
	gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCP = nCheck;

	if(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCP || gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCP)
		WTNMC4A_SetCOMPP(gl_pADView->m_hDevice,	
						gl_pADView->m_nCurrentAxis,
						gl_pADView->m_nLPEP[gl_pADView->m_nCurrentAxis], 
						gl_pADView->m_nSynchronCOMPPValue[gl_pADView->m_nCurrentAxis]); // 设置COMP+寄存器
	// 如果选择了LP/EP >= COMP- 或 LP/EP < COMP-
	if(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCM || gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCM)
		WTNMC4A_SetCOMPM(gl_pADView->m_hDevice,	
						gl_pADView->m_nCurrentAxis,
						gl_pADView->m_nLPEP[gl_pADView->m_nCurrentAxis], 
						gl_pADView->m_nSynchronCOMPNValue[gl_pADView->m_nCurrentAxis]); // 设置COMP-寄存器

		WTNMC4A_SetInterruptBit(    // 设置中断位		
					gl_pADView->m_hDevice,		// 设备句柄
					gl_pADView->m_nCurrentAxis,		// 轴号
					&gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis]);
}

void CInterruptSetDlg::OnCheckIntPbcp() 
{
	// TODO: Add your control notification handler code here
	int nCheck = m_Button_PBCP.GetCheck();
	if(nCheck)
	{
		m_Button_PSCP.SetCheck(0);
		gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCP = !nCheck;
	}
	gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCP = nCheck;

	if(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCP || gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCP)
		WTNMC4A_SetCOMPP(gl_pADView->m_hDevice,	
						gl_pADView->m_nCurrentAxis,
						gl_pADView->m_nLPEP[gl_pADView->m_nCurrentAxis], 
						gl_pADView->m_nSynchronCOMPPValue[gl_pADView->m_nCurrentAxis]); // 设置COMP+寄存器
	// 如果选择了LP/EP >= COMP- 或 LP/EP < COMP-
	if(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCM || gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCM)
		WTNMC4A_SetCOMPM(gl_pADView->m_hDevice,	
						gl_pADView->m_nCurrentAxis,
						gl_pADView->m_nLPEP[gl_pADView->m_nCurrentAxis], 
						gl_pADView->m_nSynchronCOMPNValue[gl_pADView->m_nCurrentAxis]); // 设置COMP-寄存器

		WTNMC4A_SetInterruptBit(    // 设置中断位		
					gl_pADView->m_hDevice,		// 设备句柄
					gl_pADView->m_nCurrentAxis,		// 轴号
					&gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis]);
}

void CInterruptSetDlg::OnCheckIntPulse() 
{
	// TODO: Add your control notification handler code here
	int nCheck = m_Button_PULSE.GetCheck();
	gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PULSE = nCheck;

	if(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCP || gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCP)
		WTNMC4A_SetCOMPP(gl_pADView->m_hDevice,	
						gl_pADView->m_nCurrentAxis,
						gl_pADView->m_nLPEP[gl_pADView->m_nCurrentAxis], 
						gl_pADView->m_nSynchronCOMPPValue[gl_pADView->m_nCurrentAxis]); // 设置COMP+寄存器
	// 如果选择了LP/EP >= COMP- 或 LP/EP < COMP-
	if(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCM || gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCM)
		WTNMC4A_SetCOMPM(gl_pADView->m_hDevice,	
						gl_pADView->m_nCurrentAxis,
						gl_pADView->m_nLPEP[gl_pADView->m_nCurrentAxis], 
						gl_pADView->m_nSynchronCOMPNValue[gl_pADView->m_nCurrentAxis]); // 设置COMP-寄存器

		WTNMC4A_SetInterruptBit(    // 设置中断位		
					gl_pADView->m_hDevice,		// 设备句柄
					gl_pADView->m_nCurrentAxis,		// 轴号
					&gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis]);
}

void CInterruptSetDlg::OnCheckIntCdec() 
{
	// TODO: Add your control notification handler code here
	int nCheck = m_Button_CDEC.GetCheck();
	gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].CDEC = nCheck;

	if(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCP || gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCP)
		WTNMC4A_SetCOMPP(gl_pADView->m_hDevice,	
						gl_pADView->m_nCurrentAxis,
						gl_pADView->m_nLPEP[gl_pADView->m_nCurrentAxis], 
						gl_pADView->m_nSynchronCOMPPValue[gl_pADView->m_nCurrentAxis]); // 设置COMP+寄存器
	// 如果选择了LP/EP >= COMP- 或 LP/EP < COMP-
	if(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCM || gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCM)
		WTNMC4A_SetCOMPM(gl_pADView->m_hDevice,	
						gl_pADView->m_nCurrentAxis,
						gl_pADView->m_nLPEP[gl_pADView->m_nCurrentAxis], 
						gl_pADView->m_nSynchronCOMPNValue[gl_pADView->m_nCurrentAxis]); // 设置COMP-寄存器

		WTNMC4A_SetInterruptBit(    // 设置中断位		
					gl_pADView->m_hDevice,		// 设备句柄
					gl_pADView->m_nCurrentAxis,		// 轴号
					&gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis]);
}

void CInterruptSetDlg::OnCheckIntCsta() 
{
	// TODO: Add your control notification handler code here
	int nCheck = m_Button_CSTA.GetCheck();
	gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].CSTA = nCheck;

	if(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCP || gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCP)
		WTNMC4A_SetCOMPP(gl_pADView->m_hDevice,	
						gl_pADView->m_nCurrentAxis,
						gl_pADView->m_nLPEP[gl_pADView->m_nCurrentAxis], 
						gl_pADView->m_nSynchronCOMPPValue[gl_pADView->m_nCurrentAxis]); // 设置COMP+寄存器
	// 如果选择了LP/EP >= COMP- 或 LP/EP < COMP-
	if(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCM || gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCM)
		WTNMC4A_SetCOMPM(gl_pADView->m_hDevice,	
						gl_pADView->m_nCurrentAxis,
						gl_pADView->m_nLPEP[gl_pADView->m_nCurrentAxis], 
						gl_pADView->m_nSynchronCOMPNValue[gl_pADView->m_nCurrentAxis]); // 设置COMP-寄存器

		WTNMC4A_SetInterruptBit(    // 设置中断位		
					gl_pADView->m_hDevice,		// 设备句柄
					gl_pADView->m_nCurrentAxis,		// 轴号
					&gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis]);
}

void CInterruptSetDlg::OnCheckIntDend() 
{
	// TODO: Add your control notification handler code here
	int nCheck = m_Button_DEND.GetCheck();
	gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].DEND = nCheck;

	if(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCP || gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCP)
		WTNMC4A_SetCOMPP(gl_pADView->m_hDevice,	
						gl_pADView->m_nCurrentAxis,
						gl_pADView->m_nLPEP[gl_pADView->m_nCurrentAxis], 
						gl_pADView->m_nSynchronCOMPPValue[gl_pADView->m_nCurrentAxis]); // 设置COMP+寄存器
	// 如果选择了LP/EP >= COMP- 或 LP/EP < COMP-
	if(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCM || gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCM)
		WTNMC4A_SetCOMPM(gl_pADView->m_hDevice,	
						gl_pADView->m_nCurrentAxis,
						gl_pADView->m_nLPEP[gl_pADView->m_nCurrentAxis], 
						gl_pADView->m_nSynchronCOMPNValue[gl_pADView->m_nCurrentAxis]); // 设置COMP-寄存器

		WTNMC4A_SetInterruptBit(    // 设置中断位		
					gl_pADView->m_hDevice,		// 设备句柄
					gl_pADView->m_nCurrentAxis,		// 轴号
					&gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis]);
}

void CInterruptSetDlg::OnCheckIntCiint() 
{
	// TODO: Add your control notification handler code here
	int nCheck = m_Button_CIINT.GetCheck();
	gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].CIINT = nCheck;

	if(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCP || gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCP)
		WTNMC4A_SetCOMPP(gl_pADView->m_hDevice,	
						gl_pADView->m_nCurrentAxis,
						gl_pADView->m_nLPEP[gl_pADView->m_nCurrentAxis], 
						gl_pADView->m_nSynchronCOMPPValue[gl_pADView->m_nCurrentAxis]); // 设置COMP+寄存器
	// 如果选择了LP/EP >= COMP- 或 LP/EP < COMP-
	if(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCM || gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCM)
		WTNMC4A_SetCOMPM(gl_pADView->m_hDevice,	
						gl_pADView->m_nCurrentAxis,
						gl_pADView->m_nLPEP[gl_pADView->m_nCurrentAxis], 
						gl_pADView->m_nSynchronCOMPNValue[gl_pADView->m_nCurrentAxis]); // 设置COMP-寄存器

		WTNMC4A_SetInterruptBit(    // 设置中断位		
					gl_pADView->m_hDevice,		// 设备句柄
					gl_pADView->m_nCurrentAxis,		// 轴号
					&gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis]);
}

void CInterruptSetDlg::OnCheckIntBpint() 
{
	// TODO: Add your control notification handler code here
	int nCheck = m_Button_BPINT.GetCheck();
	gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].BPINT = nCheck;

	if(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCP || gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCP)
		WTNMC4A_SetCOMPP(gl_pADView->m_hDevice,	
						gl_pADView->m_nCurrentAxis,
						gl_pADView->m_nLPEP[gl_pADView->m_nCurrentAxis], 
						gl_pADView->m_nSynchronCOMPPValue[gl_pADView->m_nCurrentAxis]); // 设置COMP+寄存器
	// 如果选择了LP/EP >= COMP- 或 LP/EP < COMP-
	if(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCM || gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCM)
		WTNMC4A_SetCOMPM(gl_pADView->m_hDevice,	
						gl_pADView->m_nCurrentAxis,
						gl_pADView->m_nLPEP[gl_pADView->m_nCurrentAxis], 
						gl_pADView->m_nSynchronCOMPNValue[gl_pADView->m_nCurrentAxis]); // 设置COMP-寄存器

		WTNMC4A_SetInterruptBit(    // 设置中断位		
					gl_pADView->m_hDevice,		// 设备句柄
					gl_pADView->m_nCurrentAxis,		// 轴号
					&gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis]);
}

void CInterruptSetDlg::OnChangeEditIntComp() 
{
	// TODO: If this is a RICHEDIT control, the control will not
	// send this notification unless you override the CDialog::OnInitDialog()
	// function and call CRichEditCtrl().SetEventMask()
	// with the ENM_CHANGE flag ORed into the mask.
	CString str;
	m_Edit_COMP.GetWindowText(str);
	gl_pADView->m_nSynchronCOMPPValue[gl_pADView->m_nCurrentAxis] = wcstol(str, NULL, 10);
	// TODO: Add your control notification handler code here
	
}

void CInterruptSetDlg::OnChangeEditIntComn() 
{
	// TODO: If this is a RICHEDIT control, the control will not
	// send this notification unless you override the CDialog::OnInitDialog()
	// function and call CRichEditCtrl().SetEventMask()
	// with the ENM_CHANGE flag ORed into the mask.
	CString str;
	m_Edit_COMN.GetWindowText(str);
	gl_pADView->m_nSynchronCOMPNValue[gl_pADView->m_nCurrentAxis] = wcstol(str, NULL, 10);
	// TODO: Add your control notification handler code here
	
}

void CInterruptSetDlg::OnButton1() 
{
	// TODO: Add your control notification handler code here
	CDialog::OnOK();
}

BOOL CInterruptSetDlg::OnInitDialog() 
{
	CDialog::OnInitDialog();
	
	// TODO: Add extra initialization here
	CString str;
	str.Format(L"%d", gl_pADView->m_nSynchronCOMPPValue[gl_pADView->m_nCurrentAxis]);
	m_Edit_COMP.SetWindowText(str);
	str.Format(L"%d", gl_pADView->m_nSynchronCOMPNValue[gl_pADView->m_nCurrentAxis]);
	m_Edit_COMN.SetWindowText(str);
	
	m_Button_PBCM.SetCheck(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCM);
	m_Button_PSCM.SetCheck(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCM);
	m_Button_PSCP.SetCheck(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PSCP);
	m_Button_PBCP.SetCheck(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PBCP);
	m_Button_PULSE.SetCheck(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].PULSE);
	m_Button_CDEC.SetCheck(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].CDEC);
	m_Button_CSTA.SetCheck(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].CSTA);
	m_Button_DEND.SetCheck(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].DEND);
	m_Button_CIINT.SetCheck(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].CIINT);
	m_Button_BPINT.SetCheck(gl_pADView->m_Interrupt[gl_pADView->m_nCurrentAxis].BPINT);

	return TRUE;  // return TRUE unless you set the focus to a control
	              // EXCEPTION: OCX Property Pages should return FALSE
}
