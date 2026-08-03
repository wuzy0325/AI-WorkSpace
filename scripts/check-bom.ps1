$file = $args[0]
$bytes = [System.IO.File]::ReadAllBytes($file)
$head = $bytes[0..2] | ForEach-Object { '{0:X2}' -f $_ }
Write-Host ($head -join ' ')
