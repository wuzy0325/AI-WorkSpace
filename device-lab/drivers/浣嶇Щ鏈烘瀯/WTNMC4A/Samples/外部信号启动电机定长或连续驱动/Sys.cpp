/***************************************************************************************/
#include "stdafx.h"
#include <stdio.h>
#include <conio.h>
#include "WTNMC4A.h"
int main(int argc, char* argv[])
{
	LONG A,LP,speed;
	HANDLE hDevice = WTNMC4A_DEV_CreateA("192.168.1.4");
	WTNMC4A_PARA_DataList DL;		// 公用参数
	WTNMC4A_PARA_LCData LC;			// 直线和S曲线参数
	DL.Multiple =1;
	DL.Acceleration = 125;			// 加速度(125~1000000)
	DL.Deceleration = 125;
	DL.DecIncRate = 1000;
	DL.AccIncRate = 1000;			// 加速度变化率(954~62500000)
	DL.StartSpeed = 100;			// 初始速度(1~8000)
	DL.DriveSpeed = 3000;			// 驱动速度(1~8000)

	LC.AxisNum = WTNMC4A_XAXIS;		// 轴号 (X轴 | Y轴 | X、Y轴)
	LC.LV_DV = 0;
	LC.DecMode = 0;
	LC.PLSLogLever =  0;
	LC.DIRLogLever = 0;
	LC.Line_Curve = WTNMC4A_LINE;	// 运动方式	(直线 | 曲线)
	LC.Direction = 0;
	LC.nPulseNum = 100000;			// 定量输出脉冲数(0~268435455)
	LC.PulseMode = WTNMC4A_CWCCW;	// 脉冲方式 (CW/CCW方式 | CP/DIR方式)

	WTNMC4A_InitLVDV(				// 初始化连续,定长脉冲驱动
		hDevice,		// 设备句柄
		&DL,			// 公共参数结构体指针
		&LC);			// 直线S曲线参数结构体指针
	/*	WTNMC4A_SetOutEnableDV(			// 设置外部使能定量驱动(下降沿有效)
	hDevice,		// 设备句柄
	LC.AxisNum);	// 轴号(WTNMC4A_XAXIS:X轴,WTNMC4A_YAXIS:Y轴, WTNMC4A_ZAXIS:Z轴,WTNMC4A_UAXIS:U轴) */
	WTNMC4A_SetOutEnableLV(			// 设置外部使能连续驱动(保持低电平有效)
		hDevice,		// 设备句柄
		LC.AxisNum);	// 轴号(WTNMC4A_XAXIS:X轴,WTNMC4A_YAXIS:Y轴, WTNMC4A_ZAXIS:Z轴,WTNMC4A_UAXIS:U轴) */
	while(!_kbhit())
	{
		speed= (WTNMC4A_ReadCV(hDevice, LC.AxisNum));		// 读当前速度
		A = (WTNMC4A_ReadCA(hDevice, LC.AxisNum));		// 读当前加速度
		LP = WTNMC4A_ReadLP(hDevice, LC.AxisNum);						// 读逻辑计数器
		printf("%d轴速度 = %d    ",LC.AxisNum,speed);
		printf("%d轴加速度 = %d  ", LC.AxisNum,A);
		printf("%d轴逻辑位置计数器 = %ld \n", LC.AxisNum,LP);
	}	         

	_getch();
	WTNMC4A_DecStop(hDevice, WTNMC4A_ALLAXIS);		// 减速停止
	WTNMC4A_DEV_Release(hDevice);
	return 0;
}

