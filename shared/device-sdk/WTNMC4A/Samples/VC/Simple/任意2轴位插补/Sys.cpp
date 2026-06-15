/***************************************************************************************/
#include "stdafx.h"
#include <stdio.h>
#include <conio.h>
#include "WTNMC4A.h"
int main(int argc, char* argv[])
{
	LONG A,LPX,LPY,speed;
	WTNMC4A_PARA_RR0  RR0;		// 状态寄存器RR0
	USHORT nBitData[24] = {             // 位插补的16进制数据
		0x0000, 0xFFFF, 0x0000, 0x0000, 
		0x0000, 0xFFFF, 0xFFFF, 0x0000,
		0xFFFF, 0x0000, 0xFFFF, 0x0000,
		0xFFFF, 0x0000, 0x0000, 0x0000,		
		0xFFFF, 0x0000, 0x0000, 0xFFFF,
		0x0000, 0xFFFF, 0x0000, 0xFFFF,
	};
	HANDLE hDevice =  WTNMC4A_DEV_CreateA("192.168.1.4");
	WTNMC4A_PARA_InterpolationAxis IA;		// 插补轴
	WTNMC4A_PARA_DataList DL;		// 公用参数
	IA.Axis1 = WTNMC4A_XAXIS;		// X轴
	IA.Axis2 = WTNMC4A_YAXIS;		// Y轴
	DL.Multiple =1;
	DL.Acceleration = 1250;			// 加速度(125~1000000)
	DL.StartSpeed = 1;				// 初始速度(1~8000)
	DL.DriveSpeed = 1;				// 驱动速度(1~8000)
	DL.DecIncRate=150;
	WTNMC4A_InitBitInterpolation_2D(						// 初始化位插补参数
		hDevice,
		&IA,							// 插补轴结构体指针
		&DL);							// 公共参数结构体指针

	WTNMC4A_AutoBitInterpolation_2D(hDevice, nBitData, 24); // 启动位插补子线程

	while(!_kbhit())
	{
		speed= (WTNMC4A_ReadCV(hDevice, IA.Axis1));		// 读当前速度
		A = (WTNMC4A_ReadCA(hDevice, IA.Axis1));	// 读当前加速度
		LPX = WTNMC4A_ReadLP(hDevice, IA.Axis1);		// 读逻辑计数器
		LPY = WTNMC4A_ReadLP(hDevice, IA.Axis2);
		WTNMC4A_GetRR0Status(hDevice, &RR0);			// 获得主状态寄存器RR0的位状态
		//		printf("%d轴speed= %d",IA.Axis1, speed);
		//		printf("%d轴A= %d ",IA.Axis1, A);
		printf("%d轴LP = %ld ",IA.Axis1, LPX);
		printf("%d轴LP = %ld \n", IA.Axis2, LPY);
		//		printf("SC1 = %ld ", RR0.BPSC1);
		//		printf("SC0 = %ld \n", RR0.BPSC0);
	} 
	WTNMC4A_DecStop( hDevice,  WTNMC4A_ALLAXIS); // 减速停止
	WTNMC4A_DEV_Release(hDevice);
	_getch(); 
	return 0;
}

