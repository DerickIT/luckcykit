#Requires -Version 5.1
<#
.SYNOPSIS
  在 Windows 上一条命令启动 LuckyTV 本地开发环境（Go 后端 + Vite 前端，可选云沙箱）。

.DESCRIPTION
  不依赖 Git Bash / openssl。默认：
    - 后端 http://127.0.0.1:8080，数据目录 .local\project-workbench-debug
    - 前端 http://127.0.0.1:3000，/api 代理到后端
  后端与前端都在本窗口输出日志，Ctrl+C 结束时一起停止。

.EXAMPLE
  .\scripts\dev-windows.ps1

.EXAMPLE
  .\scripts\dev-windows.ps1 -Clean                 # 用全新的验收数据目录
  .\scripts\dev-windows.ps1 -Cloud                 # 同时启动云服务沙箱（8081）
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
    [switch]$NoBackend
)

$ErrorActionPreference = "Stop"
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

function Test-PortBusy($port) {
    $listener = Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue
    return [bool]$listener
}

function Wait-Http($url, $seconds) {
    $deadline = (Get-Date).AddSeconds($seconds)
    while ((Get-Date) -lt $deadline) {
        try {
            $response = Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 3
            if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 500) { return $true }
        } catch {
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

if (-not $NoBackend -and (Test-PortBusy $BackendPort)) {
    throw "端口 $BackendPort 已被占用。先停止旧进程，或用 -BackendPort 换端口。"
}
if (-not $NoWeb -and (Test-PortBusy $WebPort)) {
    throw "端口 $WebPort 已被占用。先停止旧进程，或用 -WebPort 换端口。"
}

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
        if (Test-PortBusy $CloudPort) { throw "端口 $CloudPort 已被占用。" }
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
        $web = Start-Process -FilePath "bun" -ArgumentList "run", "dev", "--", "--host", "127.0.0.1", "--port", "$WebPort" `
            -WorkingDirectory $webDir -NoNewWindow -PassThru
        $started += $web
        $startedPorts += $WebPort
    }

    if (-not $NoBackend) {
        Write-Step "等待后端就绪"
        if (-not (Wait-Http "http://127.0.0.1:$BackendPort/api/health/ready" 120)) {
            throw "后端 120 秒内未就绪，请看上方日志。"
        }
        Write-Host "    后端就绪" -ForegroundColor Green
    }
    if (-not $NoWeb) {
        if (-not (Wait-Http "http://127.0.0.1:$WebPort/" 90)) {
            Write-Host "    前端尚未响应，Vite 可能仍在编译；日志见上方。" -ForegroundColor Yellow
        } else {
            Write-Host "    前端就绪" -ForegroundColor Green
        }
    }

    Write-Host ""
    Write-Host "可用地址：" -ForegroundColor Green
    if (-not $NoWeb) { Write-Host "  应用    http://localhost:$WebPort" }
    if (-not $NoBackend) {
        Write-Host "  后端    http://127.0.0.1:$BackendPort/api/health/ready"
        Write-Host "  Bootstrap http://127.0.0.1:$BackendPort/api/workspace/bootstrap"
    }
    if ($Cloud) { Write-Host "  云沙箱  http://127.0.0.1:$CloudPort/api/v1/health" }
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
