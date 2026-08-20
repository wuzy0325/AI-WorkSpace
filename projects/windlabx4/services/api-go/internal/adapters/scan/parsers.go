package scan

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"windlabx4/services/api-go/internal/core/device"
)

// deviceDispatcher 根据响应内容分发到对应的设备解析函数。
// remoteAddr 格式为 "host:port"，入口处统一提取纯 IP 供下游使用。
func deviceDispatcher(data []byte, remoteAddr string) *device.ScanResult {
	remoteHost := remoteHostFromAddr(remoteAddr)
	msg := strings.TrimSpace(string(data))

	// JSON 响应：根据 model/type 字段区分设备类型，默认按 T1603 处理
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(msg), &jsonData); err == nil {
		return dispatchJsonResponse(jsonData, remoteHost)
	}

	// 二进制响应：1604Pre
	if len(data) >= 36 && !isASCIIPrintable(data) {
		if result := parseDaqP1604PreResponse(data, remoteAddr); result != nil {
			return result
		}
	}

	// CSV / 短文本响应
	return parseDaqP1604Response(data, remoteAddr)
}

// dispatchJsonResponse 根据 JSON 中的 model 字段分发到对应的解析函数
func dispatchJsonResponse(jsonData map[string]interface{}, remoteHost string) *device.ScanResult {
	model := getJSONString(jsonData, "model")
	// 如果 model 包含 P1604 标识，按 P1604 处理
	if strings.Contains(strings.ToUpper(model), "P1604") {
		return parseDaqP1604Json(jsonData, remoteHost)
	}
	// 默认按 T1603 处理（保持向后兼容）
	return parseDaqT1603Json(jsonData, remoteHost)
}

func isASCIIPrintable(data []byte) bool {
	n := len(data)
	if n > 5 {
		n = 5
	}
	for _, b := range data[:n] {
		if b < 0x20 || b > 0x7E {
			return false
		}
	}
	return true
}

func parseDaqP1604Response(data []byte, remoteAddr string) *device.ScanResult {
	remoteHost := remoteHostFromAddr(remoteAddr)
	msg := strings.TrimSpace(string(data))
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(msg), &jsonData); err == nil {
		return parseDaqP1604Json(jsonData, remoteHost)
	}

	parts := strings.Split(msg, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	// CSV 响应：根据 model 字段（parts[3]）区分设备类型
	// P1604 响应格式：IP,MAC,0,序列号,firmware,...（parts[3] 为序列号，不含 T1603）
	// T1603 响应格式：IP,MAC,0,T1603,firmware,...（parts[3] 为 model "T1603"）
	// 注意：两者 parts[2] 都可能是 "0"，不能用 parts[2] 区分
	if len(parts) >= 6 {
		model := strings.ToUpper(safeGet(parts, 3))
		if strings.Contains(model, "T1603") {
			// model 包含 T1603，按 T1603 解析
			return parseDaqT1603Csv(parts, remoteHost)
		}
		// model 不含 T1603，按 P1604 解析（P1604 的 parts[3] 是序列号）
		return parseDaqP1604Csv(parts)
	}

	if strings.HasPrefix(msg, "DAQP1604") {
		return &device.ScanResult{
			ID:        scanResultID(scanDaqP1604Prefix, remoteHost, daqP1604DefaultPort, ""),
			Name:      "Discovered DAQ-P-1604",
			Type:      device.DeviceDAQP1604,
			Available: true,
			Address:   remoteHost,
			Port:      daqP1604DefaultPort,
		}
	}
	if strings.HasPrefix(msg, "DAQT1603") {
		return &device.ScanResult{
			ID:        scanResultID(scanDaqT1603Prefix, remoteHost, daqT1603DefaultPort, ""),
			Name:      "Discovered DAQ-T-1603",
			Type:      device.DeviceDaqT1603,
			Available: true,
			Address:   remoteHost,
			Port:      daqT1603DefaultPort,
		}
	}

	return nil
}

func parseDaqP1604Csv(parts []string) *device.ScanResult {
	if len(parts) < 6 {
		return nil
	}

	address := parts[0]
	if address == "" {
		return nil
	}
	port := daqP1604DefaultPort
	if p, err := parseInt(safeGet(parts, 7)); err == nil && p > 0 {
		port = p
	}

	mac := safeGet(parts, 1)
	result := &device.ScanResult{
		ID:         scanResultID(scanDaqP1604Prefix, address, port, mac),
		Name:       "Discovered DAQ-P-1604",
		Type:       device.DeviceDAQP1604,
		Available:  true,
		Address:    address,
		Port:       port,
		MacAddress: mac,
	}

	result.SerialNumber = omitZero(safeGet(parts, 3))
	result.FirmwareVersion = safeGet(parts, 4)
	result.SubnetMask = safeGet(parts, 8)
	result.Gateway = safeGet(parts, 9)

	return result
}

// parseDaqP1604Json 解析 P1604 设备的 JSON 格式响应
func parseDaqP1604Json(jsonData map[string]interface{}, remoteHost string) *device.ScanResult {
	address := remoteHost
	if ip, ok := jsonData["ip"].(string); ok && ip != "" {
		address = ip
	}
	port := daqP1604DefaultPort
	if p, ok := jsonData["port"].(float64); ok && p > 0 {
		port = int(p)
	}

	mac, _ := jsonData["mac"].(string)
	result := &device.ScanResult{
		ID:         scanResultID(scanDaqP1604Prefix, address, port, mac),
		Name:       "Discovered DAQ-P-1604",
		Type:       device.DeviceDAQP1604,
		Available:  true,
		Address:    address,
		Port:       port,
		MacAddress: mac,
	}

	result.SerialNumber = getJSONString(jsonData, "serialNumber")
	result.FirmwareVersion = getJSONString(jsonData, "firmwareVersion")
	result.SubnetMask = getJSONString(jsonData, "subnetMask")
	result.Gateway = getJSONString(jsonData, "gateway")

	return result
}

func parseDaqT1603Response(data []byte, remoteAddr string) *device.ScanResult {
	remoteHost := remoteHostFromAddr(remoteAddr)
	msg := strings.TrimSpace(string(data))

	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(msg), &jsonData); err == nil {
		return parseDaqT1603Json(jsonData, remoteHost)
	}

	parts := strings.Split(msg, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	if len(parts) >= 8 {
		return parseDaqT1603Csv(parts, remoteHost)
	}
	if strings.HasPrefix(msg, "DAQT1603") {
		return &device.ScanResult{
			ID:        scanResultID(scanDaqT1603Prefix, remoteHost, daqT1603DefaultPort, ""),
			Name:      "Discovered DAQ-T-1603",
			Type:      device.DeviceDaqT1603,
			Available: true,
			Address:   remoteHost,
			Port:      daqT1603DefaultPort,
		}
	}

	return nil
}

func remoteHostFromAddr(remoteAddr string) string {
	host := remoteAddr
	if splitHost, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = splitHost
	}
	return host
}

func parseDaqT1603Json(jsonData map[string]interface{}, remoteHost string) *device.ScanResult {
	address := remoteHost
	if ip, ok := jsonData["ip"].(string); ok && ip != "" {
		address = ip
	}
	port := daqT1603DefaultPort
	if p, ok := jsonData["port"].(float64); ok && p > 0 {
		port = int(p)
	}

	mac, _ := jsonData["mac"].(string)
	result := &device.ScanResult{
		ID:         scanResultID(scanDaqT1603Prefix, address, port, mac),
		Name:       "Discovered DAQ-T-1603",
		Type:       device.DeviceDaqT1603,
		Available:  true,
		Address:    address,
		Port:       port,
		MacAddress: mac,
	}

	result.SerialNumber = getJSONString(jsonData, "serialNumber")
	result.FirmwareVersion = getJSONString(jsonData, "firmwareVersion")
	result.Model = getJSONString(jsonData, "model")
	result.SubnetMask = getJSONString(jsonData, "subnetMask")
	result.Gateway = getJSONString(jsonData, "gateway")
	if mode, ok := jsonData["ipMode"].(string); ok {
		result.IpMode = mode
	}
	if tc, ok := jsonData["tcpConnected"].(bool); ok {
		result.TcpConnected = tc
	}
	if ia, ok := jsonData["ipAssigned"].(bool); ok {
		result.IpAssigned = ia
	}

	return result
}

func parseDaqT1603Csv(parts []string, remoteHost string) *device.ScanResult {
	if len(parts) < 8 {
		return nil
	}

	address := parts[0]
	if address == "" {
		address = remoteHost
	}
	port := daqT1603DefaultPort
	if p, err := parseInt(parts[7]); err == nil && p > 0 {
		port = p
	}

	mac := ""
	if len(parts) > 1 && parts[1] != "" {
		mac = parts[1]
	}
	result := &device.ScanResult{
		ID:         scanResultID(scanDaqT1603Prefix, address, port, mac),
		Name:       "Discovered DAQ-T-1603",
		Type:       device.DeviceDaqT1603,
		Available:  true,
		Address:    address,
		Port:       port,
		MacAddress: mac,
	}

	result.SerialNumber = omitZero(safeGet(parts, 2))
	result.Model = safeGet(parts, 3)
	result.FirmwareVersion = safeGet(parts, 4)
	if tc := safeGet(parts, 5); tc == "1" {
		result.TcpConnected = true
	}
	if ia := safeGet(parts, 6); ia == "1" {
		result.IpAssigned = true
	}
	result.SubnetMask = safeGet(parts, 8)

	return result
}

func parseDaqP1604PreResponse(data []byte, remoteAddr string) *device.ScanResult {
	if len(data) < 36 {
		return nil
	}

	ip := fmt.Sprintf("%d.%d.%d.%d", data[5], data[6], data[7], data[8])
	mac := fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
		data[9], data[10], data[11], data[12], data[13], data[14])

	return &device.ScanResult{
		ID:         fmt.Sprintf("scan-daq-p-1604pre-%s-%d", ip, daqP1064PreDefaultPort),
		Name:       "Discovered DAQ-P-1604Pre",
		Type:       device.DeviceDAQP1604Pre,
		Available:  true,
		Address:    ip,
		Port:       daqP1064PreDefaultPort,
		MacAddress: mac,
	}
}
