 #include "StdAfx.h"

////////////////////////////////////////////////////////////////////////
CADStatusView* gl_pADStatusView = NULL;
CADParaCfgView* gl_pParaCfgView = NULL;
WORD gl_ADBuffer[PCI8603_MAX_SEGMENT_COUNT][MAX_SEGMENT_SIZE]; // 缓冲队列1024*64
LONG gl_ReadSizeWords = 0;	// 读入的数据长度

BOOL gl_bDeviceADRun = FALSE;
PCI8603_PARA_AD gl_ADPara;
int  gl_nSampleMode = DMA_MODE;      // 采集模式(1、查询，2、中断， 3、DMA)
UINT ProcessDataThread(PVOID hWnd);  // 绘制数据线程
BOOL gl_bCreateFile = FALSE;
float  gl_ScreenVolume;     // 设置屏幕显示的量程
float gl_voltVolume; // 电压量程
int  gl_InputRange[MAX_AD_CHANNELS];    // 各通道设置的电压量程范围
int  gl_TopTriggerVolt;          // 上限电压
int  gl_BottomTriggerVolt;       // 下限电压
int gl_MiddleLsb[MAX_AD_CHANNELS]; // 求平移电压时的中间值
int  gl_nChannelCount = MAX_AD_CHANNELS;
float gl_PerLsbVolt; // 单位LSB的电压值
float gl_AnalyzeAllCount;
UINT gl_OverLimitCount;
int gl_ProcessMoveVolt; // 为1时, 平移电压
BOOL gl_bCreateDevice = FALSE;
BOOL gl_bProgress = FALSE;  // 是否更新进度条
int gl_nProcessMode;  // 数据处理方式 1：数字显示  2：波形显示  3：数据存盘
HANDLE gl_hEvent;  // 采集线程与绘制线程的同步信号
int gl_DigitShowMode; // 数字窗口显示模式
HANDLE gl_hFileObject;
BOOL gl_bCloseFile;
HANDLE gl_hExitEvent = NULL;
ULONGLONG gl_FileLenghtWords;
int gl_Offset=0;					// 当前缓冲段内偏移
int gl_nDrawIndex = 0;				// 绘图段索引
int gl_nReadIndex = 0;       // 读数据索引

BOOL	gl_bDataProcessing = FALSE;
CString g_strFileFullName;
LONGLONG gl_nTriggerPos = 0;		// 触发点位置
CString strMsg;
BOOL gl_FirstScreenStop = FALSE;
BOOL gl_bTileWave = TRUE;
int  gl_nProcMode = PROC_MODE_DIGIT;
BOOL gl_bCollected = FALSE;			// 是否已经进行过一次采集
ULONG gl_TrigCnt = 0;
