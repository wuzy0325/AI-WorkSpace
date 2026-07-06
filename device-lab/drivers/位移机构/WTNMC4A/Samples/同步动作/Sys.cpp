/***************************************************************************************/
#include "stdafx.h"
#include <stdio.h>
#include <conio.h>
#include "WTNMC4A.h"
int main(int argc, char* argv[])
{

	HANDLE hDevice =  WTNMC4A_DEV_CreateA("192.168.1.4");
	LONG A[4],LP[4],speed[4],BR[4],EP[4];								
	int i=0;
	WTNMC4A_PARA_DataList DL[4];
	WTNMC4A_PARA_LCData LC[4];

	WTNMC4A_PARA_SynchronActionOwnAxis Para1[3];	// 设置同步参数     
	WTNMC4A_PARA_SynchronActionOtherAxis Para2[3];	// 设置同步参数
	//	WTNMC4A_PARA_AutoHomeSearch Para3;
	WTNMC4A_SetLP(hDevice, WTNMC4A_ALLAXIS, 0); // 设置逻辑位置计数器
	WTNMC4A_SetEP(hDevice, WTNMC4A_ALLAXIS,	0);	// 设置实位计数器 		 	
	for(i=0; i<3; i++)							// 初始化X，Y，Z轴
	{
		DL[i].Multiple=1;
		DL[i].StartSpeed = 10;				// 初始速度(1~8000)
		DL[i].DriveSpeed = 8000;			// 驱动速度(1~8000)
		DL[i].Acceleration =5000;			// 加速度(125~1000000)
		DL[i].Deceleration = 2500;			// 减速度(125~1000000)
		LC[i].AxisNum = i;					// 轴号 (X轴 | Y轴 | X、Y轴)
		LC[i].PulseMode = WTNMC4A_CWCCW;	// 脉冲方式 (CW/CCW方式 | CP/DIR方式)
		LC[i].Line_Curve = WTNMC4A_LINE;	// 运动方式	(直线 | 曲线)
		LC[i].LV_DV = WTNMC4A_DV;			// 驱动方式  (定长)
		LC[i].nPulseNum = 100000;			// 定量输出脉冲数(0~268435455)
		LC[i].Direction = WTNMC4A_PDIRECTION;	// 运动方向 (正方向)
		WTNMC4A_InitLVDV(						// 初始化连续,定长脉冲驱动
			hDevice,				// 设备句柄
			&DL[i],				// 公共参数结构体指针
			&LC[i]);
	}
	Para1[0].AXIS1 = 1;						// 选择Y，Z轴作为X轴的同步轴
	Para1[0].AXIS2 = 1;	
	Para1[0].DEND= 1;						// 当X轴停止时驱动时启动同步操作
	WTNMC4A_SetSynchronAction(
		hDevice,				// 设备句柄
		WTNMC4A_XAXIS,			// 轴号(WTNMC4A_XAXIS:X轴,WTNMC4A_YAXIS:Y轴, WTNMC4A_ZAXIS:Z轴,WTNMC4A_UAXIS:U轴) 
		&Para1[0],				// 自己轴的参数结构体指针
		&Para2[0]);				// 其它轴的参数结构体指针

	Para2[1].FDRVP = 1;						// 启动同步操作时启动Y，Z轴正方向定长驱动
	WTNMC4A_SetSynchronAction(
		hDevice,				// 设备句柄
		WTNMC4A_YAXIS,			// 轴号(WTNMC4A_XAXIS:X轴,WTNMC4A_YAXIS:Y轴, WTNMC4A_ZAXIS:Z轴,WTNMC4A_UAXIS:U轴) 
		&Para1[1],				// 自己轴的参数结构体指针
		&Para2[1]);
	Para2[2].FDRVP = 1;						// 启动同步操作时启动Y，Z轴正方向定长驱动
	WTNMC4A_SetSynchronAction(
		hDevice,				// 设备句柄
		WTNMC4A_ZAXIS,			// 轴号(WTNMC4A_XAXIS:X轴,WTNMC4A_YAXIS:Y轴, WTNMC4A_ZAXIS:Z轴,WTNMC4A_UAXIS:U轴) 
		&Para1[2],				// 自己轴的参数结构体指针
		&Para2[2]);

	WTNMC4A_StartLVDV(						// 启动连续,定长脉冲驱动
		hDevice,				// 设备句柄
		WTNMC4A_XAXIS); 
	while(!_kbhit())
	{
		for(i=0; i<3; i++)
		{
			speed[i]= (WTNMC4A_ReadCV(hDevice, i));	// 读当前速度
			A[i] = (WTNMC4A_ReadCA(hDevice, i));	// 读当前加速度
			LP[i] = WTNMC4A_ReadLP(hDevice, i);			// 读逻辑计数器
			EP[i] = WTNMC4A_ReadEP(hDevice, i);
			BR[i] = WTNMC4A_ReadBR(hDevice, i);
			speed[i]= (WTNMC4A_ReadCV(hDevice, i));
			LP[i] = WTNMC4A_ReadLP(hDevice, i);
		}
		//		printf("%dSV=%d ",WTNMC4A_XAXIS,speed[0]);
		//		printf("%dSV=%d ",WTNMC4A_YAXIS,speed[1]);
		//		printf("%dSV=%d ",WTNMC4A_ZAXIS,speed[2]);
		printf("%dLP=%ld ", WTNMC4A_XAXIS,LP[0]);
		//		printf("%dBR =%d   ",WTNMC4A_XAXIS,BR[0]);	
		printf("%dLP=%ld ", WTNMC4A_YAXIS,LP[1]);
		//		printf("%dBR =%d   ",WTNMC4A_YAXIS,BR[1]);
		printf("%dLP=%ld  \n", WTNMC4A_ZAXIS,LP[2]);
		//		printf("%dBR =%d  \n",WTNMC4A_ZAXIS,BR[2]);	
	}	 
	_getch();		
	while(!_kbhit())
	{
		for(i=0; i<3; i++)
		{
			BR[i] = WTNMC4A_ReadBR(hDevice, i);		// 读同步缓冲寄存器
		}
		printf("%dBR =%d   ",WTNMC4A_XAXIS,BR[0]);
		printf("%dBR =%d   ",WTNMC4A_YAXIS,BR[1]);
		printf("%dBR =%d  \n",WTNMC4A_ZAXIS,BR[2]);		
	}
	WTNMC4A_DecStop(hDevice, WTNMC4A_ALLAXIS);		// 减速停止
	WTNMC4A_DEV_Release(hDevice);
	_getch();
	return 1;
}

