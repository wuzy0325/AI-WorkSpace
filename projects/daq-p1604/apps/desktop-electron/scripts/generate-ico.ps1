# 从 appicon.png 生成多尺寸 ICO 文件（满足 electron-builder 至少 256x256 要求）
# ICO 格式：ICONDIR(6B) + N * ICONDIRENTRY(16B) + N * PNG_DATA
# Windows Vista+ 支持 ICO 内嵌 PNG 数据，无需 BMP 转换
# 用法：powershell -File generate-ico.ps1

param(
    [string]$SourcePng = "..\..\desktop-wails\appicon.png",
    [string]$OutputIco = "..\..\desktop-wails\appicon.ico",
    [int[]]$Sizes = @(256, 128, 64, 48, 32, 16)
)

$ErrorActionPreference = "Stop"

# 解析为绝对路径（相对路径基于 $PSScriptRoot，并规范化 .. 避免 Bitmap.FromFile 拒绝）
if ([System.IO.Path]::IsPathRooted($SourcePng)) {
    $sourcePath = $SourcePng
} else {
    $sourcePath = Join-Path $PSScriptRoot $SourcePng
}
if ([System.IO.Path]::IsPathRooted($OutputIco)) {
    $outputPath = $OutputIco
} else {
    $outputPath = Join-Path $PSScriptRoot $OutputIco
}
# 用 .NET GetFullPath 规范化 .. 和 . （Bitmap.FromFile 不接受含 .. 的路径）
$sourcePath = [System.IO.Path]::GetFullPath($sourcePath)
$outputPath = [System.IO.Path]::GetFullPath($outputPath)

Write-Host "源 PNG: $sourcePath"
Write-Host "输出 ICO: $outputPath"
Write-Host "尺寸集合: $($Sizes -join ', ')"

Add-Type -AssemblyName System.Drawing
Add-Type -AssemblyName System.IO

# 读取源 PNG（用 FromFile 静态方法避免构造函数歧义）
$sourceBitmap = [System.Drawing.Bitmap]::FromFile($sourcePath)
if ($sourceBitmap.Width -ne $sourceBitmap.Height) {
    $sourceBitmap.Dispose()
    throw "源 PNG 非正方形: $($sourceBitmap.Width)x$($sourceBitmap.Height)"
}
Write-Host "源 PNG 尺寸: $($sourceBitmap.Width)x$($sourceBitmap.Height)"

# 为每个尺寸生成 PNG 字节数据
$pngBytesList = @()
$sizeList = @()
foreach ($size in $Sizes) {
    # 缩放到目标尺寸（高质量双三次插值）
    $resized = [System.Drawing.Bitmap]::new($size, $size, [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
    $graphics = [System.Drawing.Graphics]::FromImage($resized)
    $graphics.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
    $graphics.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::HighQuality
    $graphics.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
    $graphics.DrawImage($sourceBitmap, 0, 0, $size, $size)
    $graphics.Dispose()

    # 保存为 PNG 字节
    $ms = [System.IO.MemoryStream]::new()
    $resized.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
    $resized.Dispose()
    $pngBytes = $ms.ToArray()
    $ms.Dispose()

    $pngBytesList += , $pngBytes
    $sizeList += $size
    Write-Host "  尺寸 ${size}x${size}: $($pngBytes.Length) bytes (PNG)"
}

$sourceBitmap.Dispose()

# 构建 ICO 文件
# ICONDIR: Reserved(2B=0) + Type(2B=1) + Count(2B)
$count = $sizeList.Count
$headerSize = 6
$dirEntrySize = 16
$dataOffset = $headerSize + $dirEntrySize * $count

$totalSize = $dataOffset
foreach ($bytes in $pngBytesList) { $totalSize += $bytes.Length }

$fs = [System.IO.File]::Create($outputPath)
$bw = [System.IO.BinaryWriter]::new($fs)

try {
    # ICONDIR
    $bw.Write([uint16]0)        # Reserved
    $bw.Write([uint16]1)        # Type = ICO
    $bw.Write([uint16]$count)   # Image count

    # ICONDIRENTRY (每个 16 字节)
    $currentOffset = $dataOffset
    for ($i = 0; $i -lt $count; $i++) {
        $size = $sizeList[$i]
        $pngBytes = $pngBytesList[$i]

        # Width(1B, 0=256) + Height(1B, 0=256) + ColorCount(1B,0) + Reserved(1B,0)
        $widthByte = if ($size -eq 256) { [byte]0 } else { [byte]$size }
        $heightByte = if ($size -eq 256) { [byte]0 } else { [byte]$size }
        $bw.Write([byte]$widthByte)
        $bw.Write([byte]$heightByte)
        $bw.Write([byte]0)         # ColorCount (0 for >=8bpp)
        $bw.Write([byte]0)         # Reserved
        # ColorPlanes(2B=1) + BitsPerPixel(2B=32)
        $bw.Write([uint16]1)       # ColorPlanes
        $bw.Write([uint16]32)      # BitsPerPixel
        # SizeInBytes(4B) + Offset(4B)
        $bw.Write([uint32]$pngBytes.Length)
        $bw.Write([uint32]$currentOffset)

        $currentOffset += $pngBytes.Length
    }

    # PNG 数据
    for ($i = 0; $i -lt $count; $i++) {
        $bw.Write($pngBytesList[$i])
    }

    Write-Host ""
    Write-Host "ICO 生成完成: $outputPath ($totalSize bytes)"
}
finally {
    $bw.Dispose()
    $fs.Dispose()
}
