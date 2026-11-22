#Requires -Version 5.1
<#
.SYNOPSIS
  在 Windows 上一条命令启动 LuckyTV 本地开发环境（Go 后端 + Vite 前端，可选云沙箱）。

.DESCRIPTION
  不依赖 Git Bash / openssl。默认：
    - 后端 http://127.0.0.1:8080，数据目录 .local\project-workbench-debug
    - 前端 http://127.0.0.1:3000，/api 代理到后端
  端口被占用时会打印占用进程（PID / 进程名 / 命令行），加 -StopExisting 可直接结束它。

.EXAMPLE
  .\scripts\dev-windows.ps1
  .\scripts\dev-windows.ps1 -Desktop
  .\scripts\dev-windows.ps1 -Desktop -Delve
  .\scripts\dev-windows.ps1 -Clean
  .\scripts\dev-windows.ps1 -Cloud
  .\scripts\dev-windows.ps1 -StopExisting
  .\scripts\dev-windows.ps1 -BackendPort 8081 -WebPort 3001
#>
[CmdletBinding()]
param(
    [int]$BackendPort = 8080,
    [int]$WebPort = 3000,
    [string]$DataDir = "../.local/project-workbench-debug",
    [switch]$Clean,
    [switch]$Cloud,
    [int]$CloudPort = 8081,
    [switch]$NoWeb,
    [switch]$NoBackend,
    [switch]$StopExisting,
    # 打开真正的桌面客户端（Wails 窗口 + 内嵌 Go 服务），前端仍由 Vite 提供，可热更新。
    [switch]$Desktop,
    # 配合 -Desktop：用 Delve 启动客户端，等待 127.0.0.1:2345 上的调试器连接。
    [switch]$Delve,
    [int]$DelvePort = 2345
)

$ErrorActionPreference = "Stop"
# Windows PowerShell 5.1 默认按系统代码页输出，中文提示会变成 ???；统一成 UTF-8。
try { [Console]::OutputEncoding = [System.Text.Encoding]::UTF8 } catch { }
$root = Split-Path -Parent $PSScriptRoot
$backendDir = Join-Path $root "backend"
$webDir = Join-Path $root "web"
$started = @()
$startedPorts = @()

function Write-Step($message) { Write-Host "==> $message" -ForegroundColor Cyan }

function Assert-Command($name) {
    if (-not (Get-Command $name -ErrorAction SilentlyContinue)) {
        throw "找不到命令 '$name'。请先安装并加入 PATH。"
    }
}

function New-HexToken([int]$byteCount = 32) {
    $buffer = New-Object byte[] $byteCount
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($buffer) } finally { $rng.Dispose() }
    return -join ($buffer | ForEach-Object { $_.ToString("x2") })
}

function Get-PortOwner($port) {
    $listener = Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $listener) { return $null }
    $process = Get-Process -Id $listener.OwningProcess -ErrorAction SilentlyContinue
    $commandLine = ""
    try {
        $commandLine = (Get-CimInstance Win32_Process -Filter "ProcessId = $($listener.OwningProcess)" -ErrorAction Stop).CommandLine
    } catch { }
    return [pscustomobject]@{
        Port        = $port
        PID         = $listener.OwningProcess
        Name        = if ($process) { $process.ProcessName } else { "unknown" }
        CommandLine = $commandLine
    }
}

function Wait-Http($url, $seconds, $headers = $null) {
    $deadline = (Get-Date).AddSeconds($seconds)
    while ((Get-Date) -lt $deadline) {
        try {
            if ($headers) {
                $response = Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 3 -Headers $headers
            } else {
                $response = Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 3
            }
            if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 500) { return $true }
        } catch {
            # Windows PowerShell 5.1 把 4xx/5xx 抛成异常，但服务已经起来了也算就绪。
            $status = 0
            if ($_.Exception.Response -and $_.Exception.Response.StatusCode) {
                $status = [int]$_.Exception.Response.StatusCode
            }
            if ($status -ge 200 -and $status -lt 500) { return $true }
            Start-Sleep -Milliseconds 500
        }
    }
    return $false
}

function Stop-Listeners($ports) {
    foreach ($port in $ports) {
        $listeners = Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue
        foreach ($listener in $listeners) {
            try { Stop-Process -Id $listener.OwningProcess -Force -ErrorAction SilentlyContinue } catch { }
        }
    }
}

Write-Step "检查依赖"
if ($Desktop) {
    if ($NoWeb) { throw "-Desktop 需要 Vite 提供实时前端，不能和 -NoWeb 一起使用。" }
    if ($Delve -and -not (Get-Command "dlv" -ErrorAction SilentlyContinue)) {
        throw "找不到 dlv。执行 go install github.com/go-delve/delve/cmd/dlv@latest 后再用 -Delve。"
    }
    # 桌面客户端自带内嵌 Go 服务，不再单独启动 cmd/server。
    $NoBackend = $true
}
Assert-Command "go"
Assert-Command "node"
if (-not $NoWeb) { Assert-Command "bun" }
Write-Host ("    go   : " + (go version))
Write-Host ("    node : " + (node -v))
if (-not $NoWeb) { Write-Host ("    bun  : " + (bun -v)) }

if ($Clean) {
    $DataDir = "../.local/accept-" + (Get-Date -Format "yyyyMMdd-HHmmss")
    Write-Step "使用全新数据目录 $DataDir"
}
$dataDirFull = [System.IO.Path]::GetFullPath((Join-Path $backendDir $DataDir))
Write-Host ("    数据: " + $dataDirFull)

$wantedPorts = @()
if (-not $NoBackend -or $Desktop) { $wantedPorts += $BackendPort }
if (-not $NoWeb) { $wantedPorts += $WebPort }
if ($Cloud) { $wantedPorts += $CloudPort }

$blocked = @()
foreach ($port in $wantedPorts) {
    $owner = Get-PortOwner $port
    if ($owner) { $blocked += $owner }
}

if ($blocked.Count -gt 0) {
    Write-Host ""
    Write-Host "以下端口已被占用：" -ForegroundColor Yellow
    foreach ($item in $blocked) {
        Write-Host ("    {0}  pid={1}  进程={2}" -f $item.Port, $item.PID, $item.Name)
        if ($item.CommandLine) {
            $command = $item.CommandLine
            if ($command.Length -gt 160) { $command = $command.Substring(0, 160) + " ..." }
            Write-Host ("            " + $command) -ForegroundColor DarkGray
        }
    }
    Write-Host ""
    if (-not $StopExisting) {
        throw "端口被占用。加 -StopExisting 结束上面的进程，或用 -BackendPort / -WebPort 换端口。"
    }
    Write-Step "停止占用进程 (-StopExisting)"
    foreach ($item in $blocked) {
        try { Stop-Process -Id $item.PID -Force -ErrorAction Stop; Write-Host ("    已停止 pid={0} ({1})" -f $item.PID, $item.Name) }
        catch { throw ("无法停止 pid={0} ({1})：{2}" -f $item.PID, $item.Name, $_.Exception.Message) }
    }
    Start-Sleep -Milliseconds 800
    $stillBusy = @()
    foreach ($port in $wantedPorts) {
        $owner = Get-PortOwner $port
        if ($owner) { $stillBusy += $owner }
    }
    if ($stillBusy.Count -gt 0) {
        throw ("端口仍被占用：{0}。请手动结束这些进程后重试。" -f (($stillBusy | ForEach-Object { "$($_.Port)(pid=$($_.PID))" }) -join ", "))
    }
}

$launchToken = $null
if ($Desktop) { $launchToken = New-HexToken }

try {
    if (-not $NoBackend) {
        Write-Step "启动后端 (127.0.0.1:$BackendPort)"
        $env:CANVAS_BACKEND_DATA_DIR = $dataDirFull
        $env:CANVAS_BACKEND_ADDR = "127.0.0.1:$BackendPort"
        $backend = Start-Process -FilePath "go" -ArgumentList "run", "./cmd/server" `
            -WorkingDirectory $backendDir -NoNewWindow -PassThru
        $started += $backend
        $startedPorts += $BackendPort
    }

    if ($Cloud) {
        Write-Step "启动云服务沙箱 (127.0.0.1:$CloudPort)"
        $env:LUCKYTV_CLOUD_SANDBOX = "true"
        $env:LUCKYTV_CLOUD_ADDR = "127.0.0.1:$CloudPort"
        $env:LUCKYTV_CLOUD_DATA_DIR = [System.IO.Path]::GetFullPath((Join-Path $backendDir "../.local/cloud-service-sandbox"))
        $cloud = Start-Process -FilePath "go" -ArgumentList "run", "./cmd/cloud" `
            -WorkingDirectory $backendDir -NoNewWindow -PassThru
        $started += $cloud
        $startedPorts += $CloudPort
    }

    if (-not $NoWeb) {
        Write-Step "启动前端 (127.0.0.1:$WebPort，/api 代理到 $BackendPort)"
        $env:VITE_API_PROXY_TARGET = "http://127.0.0.1:$BackendPort"
        if ($Desktop) {
            # 桌面窗口走 wails.localhost 同源直连内嵌服务；这个 token 只用于在浏览器里调试同一份前端。
            $env:VITE_DESKTOP_LAUNCH_TOKEN = $launchToken
        } else {
            Remove-Item Env:VITE_DESKTOP_LAUNCH_TOKEN -ErrorAction SilentlyContinue
        }
        $webArgs = @("run", "dev")
        if ($WebPort -ne 3000) { $webArgs += @("--", "--port", "$WebPort") }
        $web = Start-Process -FilePath "bun" -ArgumentList $webArgs `
            -WorkingDirectory $webDir -NoNewWindow -PassThru
        $started += $web
        $startedPorts += $WebPort
    }

    if ($Desktop) {
        Write-Step "启动桌面客户端 (窗口内前端 = http://127.0.0.1:$WebPort)"
        if (-not (Wait-Http "http://127.0.0.1:$WebPort/" 90)) {
            Write-Host "    Vite 尚未响应；客户端会自己重试等待，稍后窗口才会出画面。" -ForegroundColor Yellow
        }
        $env:CANVAS_DESKTOP_DATA_DIR = $dataDirFull
        $env:CANVAS_DESKTOP_BACKEND_ADDR = "127.0.0.1:$BackendPort"
        $env:CANVAS_DESKTOP_LAUNCH_TOKEN = $launchToken
        $env:FRONTEND_DEVSERVER_URL = "http://127.0.0.1:$WebPort"
        $desktopProcess = $null
        if ($Delve) {
            $delveArgs = @(
                "debug", "./cmd/desktop",
                "--headless", "--listen=127.0.0.1:$DelvePort",
                "--api-version=2", "--accept-multiclient", "--continue"
            )
            $desktopProcess = Start-Process -FilePath "dlv" -ArgumentList $delveArgs `
                -WorkingDirectory $backendDir -NoNewWindow -PassThru
        } else {
            $desktopProcess = Start-Process -FilePath "go" -ArgumentList "run", "./cmd/desktop" `
                -WorkingDirectory $backendDir -NoNewWindow -PassThru
        }
        $started += $desktopProcess
        $startedPorts += $BackendPort
    }

    if (-not $NoBackend -or $Desktop) {
        Write-Step "等待内嵌服务就绪"
        # 桌面 profile 的 API 需要启动令牌，就绪探测也要带上，否则只会拿到 403。
        $readyHeaders = $null
        if ($Desktop) { $readyHeaders = @{ "X-Desktop-Token" = $launchToken } }
        if (-not (Wait-Http "http://127.0.0.1:$BackendPort/api/health/ready" 180 $readyHeaders)) {
            throw "内嵌服务 180 秒内未就绪，请看上方日志。"
        }
        Write-Host "    后端就绪" -ForegroundColor Green
    }
    if (-not $NoWeb) {
        if (Wait-Http "http://127.0.0.1:$WebPort/" 90) { Write-Host "    前端就绪" -ForegroundColor Green }
        else { Write-Host "    前端尚未响应，Vite 可能仍在编译；日志见上方。" -ForegroundColor Yellow }
    }

    Write-Host ""
    Write-Host "可用地址：" -ForegroundColor Green
    if ($Desktop) { Write-Host "  客户端      已打开的 LuckyTV 窗口（前端热更新，F12 打开 WebView2 开发者工具）" }
    if (-not $NoWeb) { Write-Host "  网页         http://localhost:$WebPort" }
    if (-not $NoBackend -or $Desktop) {
        Write-Host "  后端健康     http://127.0.0.1:$BackendPort/api/health/ready"
        Write-Host "  Bootstrap    http://127.0.0.1:$BackendPort/api/workspace/bootstrap"
    }
    if ($Delve) { Write-Host "  Delve 调试  127.0.0.1:$DelvePort（等待调试器接入）" }
    if ($Cloud) { Write-Host "  云沙箱       http://127.0.0.1:$CloudPort/api/v1/health" }
    Write-Host ""
    Write-Host "按 Ctrl+C 停止全部服务。" -ForegroundColor Yellow

    while ($true) {
        $alive = @($started | Where-Object { -not $_.HasExited })
        if ($alive.Count -eq 0) { break }
        Start-Sleep -Seconds 1
    }
    Write-Host "子进程已退出。" -ForegroundColor Yellow
} finally {
    Write-Step "停止服务"
    foreach ($process in $started) {
        if (-not $process.HasExited) {
            try { Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue } catch { }
        }
    }
    # go run 会再派生真正的服务进程，按端口兜底清理。
    Stop-Listeners $startedPorts
    Write-Host "已停止。" -ForegroundColor Yellow
}
