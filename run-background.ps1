# 设置控制台编码为UTF-8
$OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
[System.Console]::InputEncoding = [System.Text.Encoding]::UTF8

# 设置PowerShell的文化设置为中文
[System.Threading.Thread]::CurrentThread.CurrentCulture = [System.Globalization.CultureInfo]::CreateSpecificCulture('zh-CN')
[System.Threading.Thread]::CurrentThread.CurrentUICulture = [System.Globalization.CultureInfo]::CreateSpecificCulture('zh-CN')

# 设置代理
$env:HTTP_PROXY = "http://127.0.0.1:7897"
$env:HTTPS_PROXY = "http://127.0.0.1:7897"

Write-Host "启动自动签到系统（后台运行）..."

# 无限循环运行
while ($true) {
    Write-Host "[$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')] 启动自动签到系统..."
    try {
        # 运行bot.exe
        ./bot.exe
        
        # 如果bot.exe退出，等待5秒后重新启动
        Write-Host "[$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')] 系统退出，等待5秒后重新启动..."
        Start-Sleep -Seconds 5
    } catch {
        # 如果出现错误，等待5秒后重新启动
        Write-Host "[$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')] 运行出错: $($_.Exception.Message)"
        Write-Host "等待5秒后重新启动..."
        Start-Sleep -Seconds 5
    }
}