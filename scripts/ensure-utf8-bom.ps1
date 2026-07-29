$file = $args[0]
$path = Resolve-Path $file
$bytes = [System.IO.File]::ReadAllBytes($path.ProviderPath)
# UTF-8 BOM = EF BB BF
if ($bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF) {
    Write-Host "Already has UTF-8 BOM"
    exit 0
}
$bom = [byte[]](0xEF, 0xBB, 0xBF)
$out = New-Object byte[] ($bytes.Length + 3)
[Array]::Copy($bom, 0, $out, 0, 3)
[Array]::Copy($bytes, 0, $out, 3, $bytes.Length)
[System.IO.File]::WriteAllBytes($path.ProviderPath, $out)
Write-Host "UTF-8 BOM added"
