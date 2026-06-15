/***************************************************************************************/
#include "stdafx.h"
#include <stdio.h>
#include <conio.h>
#include "WTNMC4A.h"
int main(int argc, char* argv[])
{
	WTNMC4A_PARA_DO Para;	// IO输出
	HANDLE hDevice =  WTNMC4A_DEV_CreateA("192.168.1.4");
if (hDevice ==INVALID_HANDLE_VALUE)
{
	printf("WTNMC4A_DEV_CreateA error");
	_getch();
	return 0;
}


	printf("Press any key to set DO\n");
	Para.OUT0 = 0;
	Para.OUT1 = 1;
	Para.OUT2 = 0;
	Para.OUT3 = 1;
	Para.OUT4 = 0;
	Para.OUT5 = 1;
	Para.OUT6 = 0;
	Para.OUT7 = 1;

  
	_getch();
	WTNMC4A_SetDeviceDO(			// 开关量输出
		hDevice,	 		// 设备号
		WTNMC4A_XAXIS,		// 轴号
		&Para);	


	WTNMC4A_SetDeviceDO(			// 开关量输出
		hDevice,	 		// 设备号
		WTNMC4A_YAXIS,		// 轴号
		&Para);	


	WTNMC4A_SetDeviceDO(			// 开关量输出
		hDevice,	 		// 设备号
		WTNMC4A_ZAXIS,		// 轴号
		&Para);

	WTNMC4A_SetDeviceDO(			// 开关量输出
		hDevice,	 		// 设备号
		WTNMC4A_UAXIS,		// 轴号
		&Para);

    printf("Para.OUT0 = 0\n");
	printf("Para.OUT1 = 1\n");
	printf("Para.OUT2 = 0\n");
	printf("Para.OUT3 = 1\n");
	printf("Para.OUT4 = 0\n");
	printf("Para.OUT5 = 1\n");
	printf("Para.OUT6 = 0\n");
	printf("Para.OUT7 = 1\n");
	_getch();
	WTNMC4A_DEV_Release(hDevice);
	_getch();

	return 0;
}

