# 检查 PE 文件的 SubsystemVersion(决定 Win7 兼容性)
# 6.01 = Win7, 6.02 = Win8+, 6.03 = Win8.1
# 参考: PE32+ Optional Header 结构 (微软 PE/COFF 规范)
$exe = "c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\projects\daq-t1603-win7-python\dist\DAQ-T-1603-Win7.exe"
$bytes = [System.IO.File]::ReadAllBytes($exe)

# PE 头偏移在 DOS 头偏移 0x3C 处
$peOffset = [BitConverter]::ToInt32($bytes, 0x3C)
# Optional Header 起始位置 = PE 标记(4) + COFF header(20)
$optHeader = $peOffset + 4 + 20

# 读 Magic 判断 PE32 还是 PE32+
$magic = [BitConverter]::ToUInt16($bytes, $optHeader)
$isPE32Plus = ($magic -eq 0x20B)
Write-Output ("PE Magic: 0x{0:X3} ({1})" -f $magic, $(if ($isPE32Plus) {"PE32+ 64-bit"} else {"PE32 32-bit"}))

# SubsystemVersion 在 Optional Header 中的偏移
# PE32:    +40 (因为 ImageBase 是 4 字节)
# PE32+:   +48 (因为 ImageBase 是 8 字节)
if ($isPE32Plus) {
    $svOffset = $optHeader + 48
    $osvOffset = $optHeader + 40
} else {
    $svOffset = $optHeader + 40
    $osvOffset = $optHeader + 32
}

$major = [BitConverter]::ToUInt16($bytes, $svOffset)
$minor = [BitConverter]::ToUInt16($bytes, $svOffset + 2)
$osMajor = [BitConverter]::ToUInt16($bytes, $osvOffset)
$osMinor = [BitConverter]::ToUInt16($bytes, $osvOffset + 2)

Write-Output ("PE SubsystemVersion: {0}.{1:00}" -f $major, $minor)
Write-Output ("PE OSVersion:        {0}.{1:00}" -f $osMajor, $osMinor)

# 判定 Win7 兼容性
# SubsystemVersion <= 6.01 → Win7 SP1 可运行
if ($major -lt 6 -or ($major -eq 6 -and $minor -le 1)) {
    Write-Output "结论: WIN7_COMPATIBLE - 可在 Win7 SP1+ 运行"
} elseif ($major -eq 6 -and $minor -eq 2) {
    Write-Output "结论: WIN8_REQUIRED - 需要 Win8+"
} else {
    Write-Output "结论: WIN81_REQUIRED - 需要 Win8.1+"
}
