extern CSysApp theApp;
////////////////////////////////////////////////////////////////////////
extern CADStatusView* gl_pADStatusView;
extern CADParaCfgView* gl_pParaCfgView;

extern WORD gl_ADBuffer[PCI8603_MAX_SEGMENT_COUNT][MAX_SEGMENT_SIZE]; // 缓冲队列
extern LONG gl_ReadSizeWords;	// 读入的数据长度
extern BOOL gl_bDeviceADRun;
extern PCI8603_PARA_AD gl_ADPara;
extern int  gl_nSampleMode;            // 采集模式(1, 查询， 2, 中断， 3， DMA)
extern UINT ProcessDataThread(PVOID pThreadPara);  // 绘制数据线程
extern BOOL gl_bCreateFile;
extern float  gl_ScreenVolume;	// 设置屏幕显示的量程
extern float gl_voltVolume;
extern int  gl_InputRange[MAX_AD_CHANNELS];	// 各通道设置的电压量程范围
extern int  gl_TopTriggerVolt;          // 上限电压
extern int  gl_BottomTriggerVolt;       // 下限电压
extern int	gl_MiddleLsb[MAX_AD_CHANNELS]; // 求平移电压时的中间值 MAX_AD_CHANNELS
extern int  gl_nChannelCount;
extern float gl_PerLsbVolt; // 单位LSB的电压值
extern float gl_AnalyzeAllCount;
extern UINT gl_OverLimitCount;
extern int gl_ProcessMoveVolt;	// 为1时, 平移电压
extern BOOL gl_bCreateDevice;
extern BOOL gl_bProgress;  // 是否更新进度条

extern int gl_nProcMode;  // 数据处理方式 1：数字显示  2：波形显示  3：数据存盘
// 采样数据处理方式(gl_nProcMode使用的选项)
#define PROC_MODE_DIGIT 1 // 数字显示
#define PROC_MODE_WAVE 2 // 波形显示
#define PROC_MODE_SAVE 3 // 存盘处理

extern int gl_DigitShowMode; // 数字窗口显示模式
// 数字窗口显示模式(gl_DigitShowMode使用的选项)
#define SHOW_MODE_DEC   0 // 十进制显示
#define SHOW_MODE_HEX   1 // 十六进制显示
#define SHOW_MODE_VOLT  2 // 电压值显示

extern HANDLE gl_hEvent;  // 采集线程与绘制线程的同步信号
extern HANDLE gl_hFileObject;
extern BOOL gl_bCloseFile;
extern HANDLE gl_hExitEvent;

extern ULONGLONG gl_FileLenghtWords;
extern int gl_Offset;     // 当前缓冲段内偏移
extern int gl_nDrawIndex;         // 绘图段索引

extern BOOL gl_bDataProcessing;
extern CString g_strFileFullName;

extern LONGLONG gl_nTriggerPos;		// 触发点位置

extern CString strMsg;

#define MsgBox AfxMessageBox

//////////////////////////////////////////////////
#define MAX_OFFSET 8192


//#############################################################
// external declare
extern BOOL gl_bCollected;
extern BOOL gl_bTileWave;
extern BOOL gl_FirstScreenStop;
extern ULONG gl_TrigCnt;
extern int gl_nReadIndex;
