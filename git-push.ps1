Write-Host "开始推送代码到GitHub..."

git remote -v

git add -A
git commit -m "feat: 添加Token更换提醒功能，每30分钟通知用户更新Token"

Write-Host "正在推送到GitHub..."
git push -u origin master
