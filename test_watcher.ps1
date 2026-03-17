$ErrorActionPreference = 'SilentlyContinue'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

Register-CimIndicationEvent -ClassName 'Win32_ProcessStartTrace' -SourceIdentifier 'ProcStart'

Write-Host "Waiting for processes..."

$ev = Wait-Event -SourceIdentifier 'ProcStart' -Timeout 10
if ($ev -ne $null) {
    $d = $ev.SourceEventArgs.NewEvent
    $obj = @{
        pid = [uint32]$d.ProcessID
        name = $d.ProcessName
    }
    $json = $obj | ConvertTo-Json -Compress
    [Console]::WriteLine($json)
}
