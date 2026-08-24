// Command device-scan 是 WindLabX4 的独立局域网设备扫描工具。
//
// 它把后端 DeviceManager.ScanDevices() 的扫描能力从整套桌面应用中剥离出来，
// 复用 internal/adapters/scan.NetworkScanner，一次性探测局域网内的 DAQ 设备
// （DAQ-P-1604 / DAQ-T-1603 / DAQ-P-1604Pre），以人类可读表格或 JSON 输出。
//
// 用法：
//
//	device-scan.exe                 一次性扫描并打印表格
//	device-scan.exe -timeout 5s     自定义扫描超时
//	device-scan.exe -json           以 JSON 数组输出（便于脚本消费）
//	device-scan.exe -out result.json 导出到文件（与 -json 共用格式）
//
// 退出码：发现设备=0；扫描出错=1；无设备但正常=0。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"golang.org/x/sys/windows"

	"windlabx4/services/api-go/internal/adapters/scan"
	"windlabx4/services/api-go/internal/core/device"
)

// setupConsole 在 Windows 下将控制台输出切换到 UTF-8 代码页，保证中文正常显示。
func setupConsole() {
	_ = windows.SetConsoleOutputCP(65001) // CP_UTF8
}

func main() {
	timeout := flag.Duration("timeout", 3*time.Second, "扫描超时时间，例如 5s / 3000ms")
	asJSON := flag.Bool("json", false, "以 JSON 数组形式输出结果")
	outFile := flag.String("out", "", "将结果写入指定文件（与 -json 共用格式）")
	iface := flag.String("iface", "", "仅扫描指定网卡（按名称，如 Ethernet0）；与 -subnet 互斥")
	subnet := flag.String("subnet", "", "仅扫描指定子网（CIDR，如 192.168.1.0/24）；与 -iface 互斥")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "WindLabX4 独立设备扫描工具\n\n用法: device-scan.exe [选项]\n\n选项:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	setupConsole()

	// 默认全网卡广播；若指定 -iface/-subnet 则限定扫描范围。
	targets, err := scan.ScopedDiscoveryTargets(*iface, *subnet)
	if err != nil {
		fmt.Fprintf(os.Stderr, "范围参数无效: %v\n", err)
		os.Exit(1)
	}
	if *iface != "" && *subnet != "" {
		fmt.Fprintln(os.Stderr, "-iface 与 -subnet 互斥，只能指定其一")
		os.Exit(1)
	}
	scopeDesc := "全网卡广播"
	if *iface != "" {
		scopeDesc = "网卡 " + *iface
	} else if *subnet != "" {
		scopeDesc = "子网 " + *subnet
	}

	// 复用现有 NetworkScanner，可覆盖超时与扫描目标。
	opts := []scan.NetworkScannerOption{scan.WithTimeout(*timeout)}
	if len(targets) > 0 {
		opts = append(opts, scan.WithTargets(targets...))
	}
	scanner := scan.NewNetworkScanner(opts...)
	results, err := scanner.Scan()
	if err != nil {
		fmt.Fprintf(os.Stderr, "扫描出错: %v\n", err)
		os.Exit(1)
	}

	if *asJSON || *outFile != "" {
		if err := outputJSON(results, *outFile); err != nil {
			fmt.Fprintf(os.Stderr, "输出 JSON 失败: %v\n", err)
			os.Exit(1)
		}
		return
	}

	printTable(results, scopeDesc, *timeout)
}

// printTable 以人类可读表格形式打印扫描结果。
func printTable(results []device.ScanResult, scope string, timeout time.Duration) {
	fmt.Println("========================================")
	fmt.Printf(" WindLabX4 设备扫描 (范围=%s, timeout=%s)\n", scope, timeout)
	fmt.Println("========================================")

	if len(results) == 0 {
		fmt.Println(" 未发现设备")
		fmt.Println("----------------------------------------")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, " 类型\tIP\t端口\tMAC\t固件\t型号\t子网掩码\t网关")
	for _, r := range results {
		fmt.Fprintf(w, " %s\t%s\t%d\t%s\t%s\t%s\t%s\t%s\n",
			r.Type, r.Address, r.Port, r.MacAddress, r.FirmwareVersion, r.Model, r.SubnetMask, r.Gateway)
	}
	w.Flush()

	fmt.Println("----------------------------------------")
	fmt.Printf(" 共发现 %d 台设备\n", len(results))
}

// outputJSON 以 JSON 数组输出结果；若指定 outFile 则同时写入文件。
func outputJSON(results []device.ScanResult, outFile string) error {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	if outFile != "" {
		if err := os.WriteFile(outFile, data, 0644); err != nil {
			return err
		}
		fmt.Printf("已写入 %s（%d 台设备）\n", outFile, len(results))
		return nil
	}
	fmt.Println(string(data))
	return nil
}
