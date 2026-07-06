/***************************************************************************************/
#include "stdafx.h"
#include <stdio.h>
#include <conio.h>
#include "WTNMC4A.h"
int main(int argc, char* argv[])
{

	HANDLE hDevice =  WTNMC4A_DEV_CreateA("192.168.1.4");

	int nAxisNum = WTNMC4A_XAXIS;
	WTNMC4A_PARA_DataList   m_DataList;		// 公用参数 

	WTNMC4A_PARA_LCData     m_LCData;		// 直线和S曲线参数

	//	WTNMC4A_PARA_Interrupt m_Interrupt;  // 中断位参数
	//	HANDLE      gl_hEventInt;
	m_DataList.Multiple =1;
	m_DataList.StartSpeed = 100;		// 初始速度(1~8000)
	m_DataList.DriveSpeed = 4000;		// 驱动速度(1~8000)
	m_DataList.Acceleration = 140;		// 加速度(125~1000000)	
	m_DataList.Deceleration = 125;		// 减速度(125~1000000)
	m_DataList.AccIncRate = 1000;		// 加速度变化率(954~62500000)
	m_DataList.DecIncRate = 1000;		// 减速度变化率(954~62500000)


	m_LCData.AxisNum = nAxisNum;		// 轴号 (X轴 | Y轴 | X、Y轴)
	m_LCData.LV_DV = 0;					// 驱动方式  (连续 | 定长 )
	m_LCData.DecMode = 0;				// 减速方式  (自动减速 | 手动减速)
	m_LCData.PulseMode = 0;				// 脉冲方式 (CW/CCW方式 | CP/DIR方式)	
	m_LCData.PLSLogLever = 0;			// 设定驱动脉冲的方向（默认正方向）
	m_LCData.DIRLogLever = 0;			// 设定方向信号输出的逻辑电平（0：低电平为正向，1：高电平为正向）
	m_LCData.Line_Curve = 0;			// 运动方式	(直线 | 曲线)
	m_LCData.Direction = 1;				// 运动方向 (正方向 | 反方向)
	m_LCData.nPulseNum = 5000;			// 定量输出脉冲数(0~268435455)


	//	memset(&m_Interrupt, 0, sizeof(m_Interrupt));


	Sleep(1);

	WTNMC4A_SetLP(hDevice, nAxisNum, 0); // 逻辑位置计数器清零
	WTNMC4A_SetEP(hDevice, nAxisNum, 0); // 实位计数器清零	

	if(!WTNMC4A_InitLVDV(hDevice,      // 初始化当前轴
		&m_DataList,
		&m_LCData))
	{
		printf("初始化单轴直线动失败！\0");
	}



	// 	WTNMC4A_SetInterruptBit(    // 设置中断位		
	// 		hDevice,				// 设备句柄
	// 		nAxisNum,					// 轴号
	// 		&m_Interrupt);	// 中断位结构体指针

	// 	
	// 	gl_hEventInt = CreateEvent(NULL, FALSE, FALSE, NULL); // 信号量
	// 	WTNMC4A_InitDeviceInt(hDevice, gl_hEventInt);       // 初始化INT


	if(!WTNMC4A_StartLVDV(hDevice,    //	启动单轴
		nAxisNum))
	{
		printf("单轴启动失败\n");
		goto ExitRead;
	}

	ULONG nRV;		// 当前速度
	LONG  nRa;			// 当前加速度
	ULONG nLPV;		// 逻辑计数器计数
	LONG  nFv;			// 实位计数器值


	WTNMC4A_PARA_RR1 RP1;
	while (!_kbhit())
	{

		// 读取X轴当前速度----------------------------------------------------------
		nRV = WTNMC4A_ReadCV(hDevice, nAxisNum); 

		// 读取X轴当前加速度--------------------------------------------------------
		nRa = WTNMC4A_ReadCA(hDevice, nAxisNum);   
		// 读取X轴逻辑计数器---------------------------------------------------------
		nLPV = WTNMC4A_ReadLP(hDevice, nAxisNum);	

		// 读取X轴实位计数器--------------------------------------------------------------
		nFv = WTNMC4A_ReadEP(hDevice, nAxisNum);  

		WTNMC4A_GetRR1Status(hDevice, nAxisNum, &RP1);

		if (RP1.DSND == 1)
		{
			nRa = nRa*-1;
		}

		printf("当前速度:%d, 加速度:%d, 逻辑计数器%u, 实位计数器%d\n", nRV, nRa, nLPV, nFv);
		Sleep(20);
	}





	//复位

	WTNMC4A_SetLP(hDevice, nAxisNum, 0); // 逻辑位置计数器清零
	WTNMC4A_SetEP(hDevice, nAxisNum, 0); // 实位计数器清零

	WTNMC4A_Reset(hDevice);

	if(!WTNMC4A_ClearSoftwareLimit(hDevice, nAxisNum))
	{
		printf("清除软件限位失败！");
		goto ExitRead;
	}

ExitRead:
	WTNMC4A_DEV_Release(hDevice);

	_getch();

	return 0;
}

