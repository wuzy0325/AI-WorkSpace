// FileOpenDlg.cpp : implementation file
//

#include "stdafx.h"

#ifdef _DEBUG
#define new DEBUG_NEW
#undef THIS_FILE
static char THIS_FILE[] = __FILE__;
#endif

/////////////////////////////////////////////////////////////////////////////
// CFileOpenDlg dialog

CFileOpenDlg::CFileOpenDlg(CWnd* pParent /*=NULL*/)
	: CDialog(CFileOpenDlg::IDD, pParent)
{
	//{{AFX_DATA_INIT(CFileOpenDlg)
		// NOTE: the ClassWizard will add member initialization here
	//}}AFX_DATA_INIT
	m_nBatCode = 0;
}

void CFileOpenDlg::DoDataExchange(CDataExchange* pDX)
{
	CDialog::DoDataExchange(pDX);
	//{{AFX_DATA_MAP(CFileOpenDlg)
	DDX_Control(pDX, IDC_EDIT_FilePath, m_Edit_Path);
	//}}AFX_DATA_MAP
}


BEGIN_MESSAGE_MAP(CFileOpenDlg, CDialog)
	//{{AFX_MSG_MAP(CFileOpenDlg)
	ON_BN_CLICKED(IDC_BUTTON_FileSel0, OnBUTTONFileSel0)
	//}}AFX_MSG_MAP
END_MESSAGE_MAP()

/////////////////////////////////////////////////////////////////////////////
// CFileOpenDlg message handlers

void CFileOpenDlg::OnOK() 
{
	// TODO: Add extra validation here
	CSysApp* pApp = (CSysApp*)AfxGetApp();
	CButton* pButton;
	SetWindowText("正在打开文件.....");
	// 从文本框中取得文件路径
	m_Edit_Path.GetWindowText(pApp->m_strFilePath);

	// 禁止两个按钮
	pButton	= (CButton*)GetDlgItem(IDOK);
	pButton->EnableWindow(FALSE);
	pButton = (CButton*)GetDlgItem(IDCANCEL);
	pButton->EnableWindow(FALSE);
	CDialog::OnOK();
}

BOOL CFileOpenDlg::OnInitDialog() 
{
	CDialog::OnInitDialog();
	CSysApp* pApp = (CSysApp*)AfxGetApp();

	for (int Channel=0; Channel<MAX_AD_CHANNELS; Channel++)
	{
		if (pApp->m_strFilePath != "")
		InintFilePath(pApp->m_strFilePath); // 用此路径来初始化其它路径
	}

	return TRUE;  // return TRUE unless you set the focus to a control
	              // EXCEPTION: OCX Property Pages should return FALSE
}

void CFileOpenDlg::OnBUTTONFileSel0() 
{
	// TODO: Add your control notification handler code here
	CString strFilePath;
	strFilePath = SelFilePath();  // 选择文件路径
	ReadFileHead(strFilePath, 0); // 读文件头, 设置文件路径
}


// 利用文件打开对话框，选择文件路径
CString CFileOpenDlg::SelFilePath() 
{
	CString strFileName;
	if (!(theApp.DoPromptFileName(strFileName, IDS_ADHist, 
		OFN_HIDEREADONLY, TRUE, NULL))) 
		return "";
	
	return strFileName;
}

// 读文件头
BOOL CFileOpenDlg::ReadFileHead(CString strFilepath, int Channel)
{
	CString str;
	CFile file;
	FILE_HEADER FileHeader;  // 保存文件头信息
	if (!file.Open(strFilepath, PCI8603_modeRead)) // 打开文件
		return FALSE;

	file.Seek(0, CFile::begin);
	file.Read((WORD*)(&FileHeader), sizeof(FileHeader)); // 读取文件头
	file.Close(); // 关闭文件
	if (FileHeader.DeviceNum != DEFAULT_DEVICE_NUM)
	{
		AfxMessageBox("此文件并非PCI8603板卡配套文件，请重新选择!");
		return FALSE;
	}

	if (FileHeader.BatCode != m_nBatCode) // 如果不是同一批数据文件
	{
		AfxMessageBox("此文件与其它通道不是同批文件，请重新选择!");
		return FALSE;
	}

	m_Edit_Path.SetWindowText(strFilepath); // 设置路径
	return TRUE;
}

// 初始化文件路径到文本框中
void CFileOpenDlg::InintFilePath(CString strPath)
{
	CFile file;
	FILE_HEADER FileHeader;  // 保存文件头信息
	CString strTail, strBase, strWhole, str;
	if (!file.Open(strPath, PCI8603_modeRead)) // 打开文件
	{
		AfxMessageBox("文件打开错误或无此文件:"+strPath);
		return;
	}

	file.Seek(0, CFile::begin);
	file.Read((WORD*)&FileHeader, sizeof(FileHeader)); // 读取文件头
	file.Close(); // 关闭文件
	m_nBatCode = FileHeader.BatCode;     // 同批文件码
	strBase = strPath.Left(strPath.GetLength() - 5); // 取得基本文件名
	m_Edit_Path.SetWindowText(strPath);
	strTail.Format(".pci", NULL);
	strWhole = strBase + strTail;
	if (!file.Open(strWhole, PCI8603_modeRead)) // 打开文件strWhole
	{
		AfxMessageBox("未搜索到数据文件");
		m_Edit_Path.SetWindowText("");
	}
	
	file.Seek(0, CFile::begin);
	file.Read((WORD*)&FileHeader, sizeof(FileHeader)); // 读取文件头
	file.Close(); // 关闭文件

}
