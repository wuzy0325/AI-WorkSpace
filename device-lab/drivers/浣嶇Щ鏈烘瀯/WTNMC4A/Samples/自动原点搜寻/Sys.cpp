/***************************************************************************************/
#include "stdafx.h"
#include <stdio.h>
#include <conio.h>
#include "WTNMC4A.h"
int main(int argc, char* argv[])
{

	HANDLE hDevice =  WTNMC4A_DEV_CreateA("192.168.1.4");
	LONG A[4],LP[4],speed[4];								
	int i;
	WTNMC4A_PARA_ExpMode Para1;						// 设置其他参数
	WTNMC4A_PARA_AutoHomeSearch Para2;				// 自动原点搜寻设置
	WTNMC4A_SetLP(hDevice, WTNMC4A_ALLAXIS, 0);		// 设置逻辑位置计数器
	WTNMC4A_SetEP(hDevice, WTNMC4A_ALLAXIS,	0);		// 设置实位计数器 		 	
	for(i=0; i<4; i++)
	{
		WTNMC4A_SetInEnable(			// 设置自动原点搜寻第一、第二、第三步外部触发信号IN0-2的有效电平
			hDevice,			// 设备号
			i,					// 轴号(WTNMC4A_XAXIS:X轴,WTNMC4A_YAXIS:Y轴, WTNMC4A_ZAXIS:Z轴,WTNMC4A_UAXIS:U轴)	
			i,					// 停止号
			0);					// 有效电平
	}
	Para1.FE0 = 1;                  // 设置IN0，1滤波器有效
	Para1.FE1 = 1;					// 设置IN2滤波器有效
	Para1.FE4 = 1;					// 设置IN3滤波器有效
	WTNMC4A_ExtMode(				// 设置其他模式
		hDevice,			// 设备句柄
		WTNMC4A_XAXIS,		// 轴号(WTNMC4A_XAXIS:X轴,WTNMC4A_YAXIS:Y轴, WTNMC4A_ZAXIS:Z轴,WTNMC4A_UAXIS:U轴) 
		&Para1);			// 其他参数结构体指针
	Para2.LIMIT = 0;				// 1：利用硬件限位信号(nLMTP或nLMPM)进行原点搜寻 0：无效
	Para2.SAND  = 0;				// 1：原点信号和Z相信号有效时停止第三步操作 0：无效
	Para2.ST4E = 0;					// 1：第四步使能 0：无效
	Para2.ST4D = 0;					// 第四步的搜寻运转方向 0：正方向  1：负方向
	Para2.ST3E = 0;					// 1：第三步使能 0：无效
	Para2.ST3D = 0;					// 第三步的搜寻运转方向 0：正方向  1：负方向
	Para2.ST2E = 1;					// 1：第二步使能 0：无效
	Para2.ST2D = 1;					// 第二步的搜寻运转方向 0：正方向  1：负方向
	Para2.ST1E = 1;					// 1：第一步使能 0：无效
	Para2.ST1D = 0 ;				// 第一步的搜寻运转方向 0：正方向  1：负方向
	WTNMC4A_SetAutoHomeSearch(		// 设置自动原点搜寻参数
		hDevice,			// 设备句柄
		WTNMC4A_XAXIS,		// 轴号(WTNMC4A_XAXIS:X轴,WTNMC4A_YAXIS:Y轴, WTNMC4A_ZAXIS:Z轴,WTNMC4A_UAXIS:U轴)
		&Para2);
	WTNMC4A_SetA(hDevice, WTNMC4A_XAXIS, 5000);	  // 加速度		
	WTNMC4A_SetSV(hDevice, WTNMC4A_XAXIS, 100);	  // 初始速度							 
	WTNMC4A_SetV(hDevice, WTNMC4A_XAXIS, 2000);	  // 驱动速度(高速原点搜寻速度)				
	WTNMC4A_SetHV(hDevice, WTNMC4A_XAXIS, 50);    // 低速原点搜寻速度
	WTNMC4A_SetP(hDevice, WTNMC4A_XAXIS, 350000); // 定长脉冲数
	WTNMC4A_StartAutoHomeSearch(		// 启动自动原点搜寻
		hDevice,			// 设备句柄		
		WTNMC4A_XAXIS);
	while(!_kbhit())
	{
		for(i=0; i<3; i++)
		{
			speed[i]= (WTNMC4A_ReadCV(hDevice, i))*10;		// 读当前速度
			A[i] = (WTNMC4A_ReadCA(hDevice, i))*1250;		// 读当前加速度
			LP[i] = WTNMC4A_ReadLP(hDevice, i);				// 读逻辑计数器
			speed[i]= (WTNMC4A_ReadCV(hDevice, i))*10;
			LP[i] = WTNMC4A_ReadLP(hDevice, i);
		}
		printf("%dSV=%d   ",WTNMC4A_XAXIS,speed[0]);
		printf("%dSV=%d   ",WTNMC4A_YAXIS,speed[1]);

		printf("%dLP=%ld   ", WTNMC4A_XAXIS,LP[0]);
		printf("%dLP=%ld  \n", WTNMC4A_YAXIS,LP[1]);		

	}	         

	WTNMC4A_DecStop(					// 减速停止
		hDevice,				
		WTNMC4A_ALLAXIS);	

	WTNMC4A_DEV_Release(hDevice);
	_getch();
	return 1;
}

