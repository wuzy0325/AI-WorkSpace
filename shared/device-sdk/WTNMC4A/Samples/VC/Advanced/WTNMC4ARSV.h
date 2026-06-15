// 保留的头文件, 这是为用户和厂商提供的一种特殊的、增值的,额外的服务。
// 绝大多数情况下,我们建议您优先使用WTNMC4A.h中的接口函数。(RSV=Reserve)
// 本头文件中的内容,我们不提供额外的技术支持

#ifndef _WTNMC4ARSV_DEVICE_
#define _WTNMC4ARSV_DEVICE_

#include "WTNMC4A.h"

// 函数FILE_Create()的参数nOptMode所用的文件操作方式(支持"或"指令实现多种方式并行操作)
#define WTNMC4A_FILE_OPTMODE_CREATE_NEW			1	// 创建文件,如果文件存在则会出错
#define WTNMC4A_FILE_OPTMODE_CREATE_ALWAYS		2	// 不管文件是否存在,总是要被创建(即可能改写前一个文件)
#define WTNMC4A_FILE_OPTMODE_OPEN_EXISTING		3	// 打开必须已经存在的文件
#define WTNMC4A_FILE_OPTMODE_OPEN_ALWAYS			4	// 打开文件,若该文件不在,则创建它

// 函数FILE_SetOffset()的参数nBaseMode所用的文件指针移动参考基点
#define WTNMC4A_FILE_BASEMODE_BEGIN			0	// 以文件起点作为参考点往右偏移
#define WTNMC4A_FILE_BASEMODE_CURRENT			1	// 以文件的当前位置作为参考点往左或往右偏移(nOffsetBytes<0时往左偏移,>0时往右偏移)
#define WTNMC4A_FILE_BASEMODE_END				2	// 以文件的尾部作为参考点往左偏移

// 函数AUX_GetCPUTime()的参数nUnitType所用的返回值单位类型
#define WTNMC4A_UNIT_TYPE_NS	0	// 返回纳秒时间
#define WTNMC4A_UNIT_TYPE_US	1	// 返回微秒时间
#define WTNMC4A_UNIT_TYPE_MS	2	// 返回毫秒时间
#define WTNMC4A_UNIT_TYPE_S	3	// 返回秒时间
#define WTNMC4A_UNIT_TYPE_M	4	// 返回分时间
#define WTNMC4A_UNIT_TYPE_H	5	// 返回小时时间
#define WTNMC4A_UNIT_TYPE_D	6	// 返回天时间
#define WTNMC4A_UNIT_TYPE_AUTO	7   // 自动单位(如果大于等于1天,则以天为单位,依此类推)

// ################################ 保留的设备驱动接口申明 ################################
#ifndef _DEFINE
#define DEVLIB __declspec(dllimport)
#else
#define DEVLIB __declspec(dllexport)
#endif

#ifdef __cplusplus
extern "C" {
#endif

	// ################################ 设备信息函数 ################################
	BOOL DEVLIB WINAPI WTNMC4A_DEV_GetPhysIdx(					// 获得物理序号, 成功时返回TRUE,否则返回FALSE,可调用GetLastError()分析错误原因
										HANDLE hDevice,			// 设备对象句柄,它由DEV_Create()函数创建
										U32* pPhysIdx);			// 返回的物理序号

	BOOL DEVLIB WINAPI WTNMC4A_DEV_SetPhysIdx(					// 设置物理序号, 成功时返回TRUE,否则返回FALSE,可调用GetLastError()分析错误原因
										HANDLE hDevice,			// 设备对象句柄,它由DEV_Create()函数创建
										U32 nPhysIdx);			// 物理序号
	
	BOOL DEVLIB WINAPI WTNMC4A_DEV_GetVersion(					// 获得设备版本信息, 成功时返回TRUE,否则返回FALSE,可调用GetLastError()分析错误原因
										HANDLE hDevice,			// 设备对象句柄,它由DEV_Create()函数创建
										U32* pDllVer,			// 返回的动态库(.dll)版本号
										U32* pDriverVer,		// 返回的驱动(.sys)版本号
										U32* pFirmwareVer);		// 返回的固件版本号

	BOOL DEVLIB WINAPI WTNMC4A_DEV_GetSerialNum(				// 获得序列号, 成功时返回TRUE,否则返回FALSE,可调用GetLastError()分析错误原因
										HANDLE hDevice,			// 设备对象句柄,它由DEV_Create()函数创建
										U32* pSerialNum);		// 返回的序列号

	BOOL DEVLIB WINAPI WTNMC4A_DEV_GetUserPID(					// 获得用户产品ID号(User Product Identification), 成功时返回TRUE,否则返回FALSE,可调用GetLastError()分析错误原因
										HANDLE hDevice,			// 设备对象句柄,它由DEV_Create()函数创建
										U32* pUserPID);			// 返回的用户产品ID

	BOOL DEVLIB WINAPI WTNMC4A_DEV_SetUserPID(					// 设置用户产品ID号(User Product Identification), 成功时返回TRUE,否则返回FALSE,可调用GetLastError()分析错误原因
										HANDLE hDevice,			// 设备对象句柄,它由DEV_Create()函数创建
										U32 nUserPID);			// 用户产品ID

	// ############################# 文件函数(支持大于4GB文件读写) ##############################
	HANDLE DEVLIB WINAPI WTNMC4A_FILE_Create(					// 根据指定文件名创建文件句柄(hFile),如果失败,则返回值为INVALID_HANDLE_VALUE(-1),可调用GetLastError()分析错误原因
										const char* strFileName,// 路径及文件名,如"C:\\ART\\SampleData.dat" 
										I32 nOptMode);			// 文件操作模式,见上面相关常量定义

	U32 DEVLIB WINAPI WTNMC4A_FILE_Read(						// 从指定文件中读取数据,返回实际读取的字节数, 成功时返回值大于0,否则返回值等于0,可调用GetLastError()分析错误原因
										HANDLE hFile,			// 文件句柄,由FILE_Create()函数创建
										PVOID pDataBuffer,		// 数据缓冲区,存放从文件读取的数据
										U32 nSizeBytes);		// 请求读取数据的字节数

	U32 DEVLIB WINAPI WTNMC4A_FILE_Write(						// 向指定文件写入数据,返回实际写入的字节数, 成功时返回值大于0,否则返回值等于0,可调用GetLastError()分析错误原因
										HANDLE hFile,			// 文件句柄,由FILE_Create()函数创建
										PVOID pDataBuffer,		// 数据缓冲区,存放要写入文件的数据
										U32 nSizeBytes);		// 请求写入数据的字节数

	U64 DEVLIB WINAPI WTNMC4A_FILE_GetLength(HANDLE hFile);		// 获取指定文件的长度(字节数), 成功时返回值大于0,否则返回值等于0,可调用GetLastError()分析错误原因

	BOOL DEVLIB WINAPI WTNMC4A_FILE_SetOffset(					// 设置读写文件的偏移位置, 成功时返回TRUE,否则返回FALSE,可调用GetLastError()分析错误原因
										HANDLE hFile,			// 文件句柄,由FILE_Create()函数创建
										I64 nOffsetBytes,		// 偏移位置(字节)
										I32 nBaseMode);			// 参考基点模式,具体请参考上面的相关常量定义

	U64 DEVLIB WINAPI WTNMC4A_FILE_GetDiskFreeBytes(			// 获取指定磁盘的剩余空间（字节数）,成功时返回值大于0,否则返回值等于0,可调用GetLastError()分析错误原因
										const char* strDiskName); // 磁盘名称,如"C:\\", "D:\\"

	BOOL DEVLIB WINAPI WTNMC4A_FILE_Release(HANDLE hFile);		// 释放文件句柄


#ifdef __cplusplus
}
#endif

#ifndef _CVIDEF_H
	// 自动包含驱动函数导入库
	#ifndef _DEFINE
		#ifndef LOAD_WTNMC4A_LIB // 如果没有加载LIB库
		#define LOAD_WTNMC4A_LIB
			#ifndef _WIN64
				#pragma comment(lib, "WTNMC4A.lib")
				#pragma message("======== Welcome to use our art company's products!")
				#pragma message("======== Automatically linking with WTNMC4A.dll...")
				#pragma message("======== Successfully linked with WTNMC4A.dll")
			#else
				#pragma comment(lib, "WTNMC4A_64.lib")
				#pragma message("======== Welcome to use our art company's products!")
				#pragma message("======== Automatically linking with WTNMC4A_64.dll...")
				#pragma message("======== Successfully linked with WTNMC4A_64.dll")
			#endif
		#endif // LOAD_WTNMC4A_LIB
	#endif // _DEFINE
#endif // _CVIDEF_H


#endif // _WTNMC4ARSV_DEVICE_