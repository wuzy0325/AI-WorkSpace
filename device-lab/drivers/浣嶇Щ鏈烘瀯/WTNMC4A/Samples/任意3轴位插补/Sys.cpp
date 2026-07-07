/***************************************************************************************/
#include "stdafx.h"
#include <stdio.h>
#include <conio.h>
#include "WTNMC4A.h"
int main(int argc, char* argv[])
{

	LONG A,LPX,LPY,LPZ,speed;	
	USHORT nBitData[36] = {
		0x0000, 0xFFFF, 0x0000, 0x0000, 0xFFFF, 0x0000,
		0x0000, 0xFFFF, 0xFFFF, 0x0000, 0x0000, 0x0000,
		0xFFFF, 0x0000, 0xFFFF, 0x0000, 0x0000, 0x0000,
		0xFFFF, 0x0000, 0x0000, 0x0000,	0x0000, 0x0000,	
		0xFFFF, 0x0000, 0x0000, 0xFFFF, 0x0000, 0x0000,
		0x0000, 0xFFFF, 0x0000, 0xFFFF, 0x0000, 0x0000,
	};

	HANDLE hDevice =  WTNMC4A_DEV_CreateA("192.168.1.4");
	WTNMC4A_PARA_InterpolationAxis IA;				// 插补轴
	WTNMC4A_PARA_DataList DL;		// 公用参数
	IA.Axis1 = WTNMC4A_XAXIS;		// X轴
	IA.Axis2 = WTNMC4A_YAXIS;		// Y轴
	IA.Axis3 = WTNMC4A_ZAXIS;		// Z轴
	DL.Multiple =1;
	DL.Acceleration = 1250;			// 加速度(125~1000000)
	DL.StartSpeed = 1;				// 初始速度(1~8000)
	DL.DriveSpeed = 1;				// 驱动速度(1~8000)
	WTNMC4A_InitBitInterpolation_3D(	// 初始化任意3轴位插补参数
		hDevice,			// 设备句柄
		&IA,				// 插补轴结构体指针
		&DL);				// 公共参数结构体指针
	WTNMC4A_AutoBitInterpolation_3D(hDevice, (PSHORT)nBitData, 36);	// 启动任意3轴位插补子线程	
	while(!_kbhit())
	{

		speed= (WTNMC4A_ReadCV(hDevice, IA.Axis1));		// 读当前速度
		A = (WTNMC4A_ReadCA(hDevice, IA.Axis1));	// 读当前加速度
		LPX = WTNMC4A_ReadLP(hDevice, IA.Axis1);		// 读逻辑计数器
		LPY = WTNMC4A_ReadLP(hDevice, IA.Axis2);
		LPZ = WTNMC4A_ReadLP(hDevice, IA.Axis3);
		printf("X轴speed= %d ",speed);
		printf("X轴A= %d ", A);
		printf("X轴LP = %ld ", LPX);
		printf("Y轴LP = %ld ", LPY);	
		printf("Z轴LP = %ld \n", LPZ);
	}

	WTNMC4A_DecStop(hDevice, WTNMC4A_ALLAXIS);		// 减速停止
	WTNMC4A_DEV_Release(hDevice);
	_getch();
	return 0;
}

