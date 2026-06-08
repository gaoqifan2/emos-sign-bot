# 自动签到系统

这是一个部署在VPS上的自动签到脚本，支持通过Telegram Bot管理多个账号的签到任务。

## 功能特点

- 支持多个账号定时签到
- 支持自定义签到时间或随机签到时间
- 通过Telegram Bot管理签到账号
- 签到成功后发送通知到Telegram
- 支持查看当前签到用户列表

## 技术栈

- Node.js
- Express
- node-cron
- axios

## 部署步骤

1. **安装依赖**
   ```bash
   npm install
   ```

2. **配置修改**
   - 编辑 `index.js` 文件，将 `your-vps-ip` 替换为实际的VPS IP地址
   - 确保VPS开放了相应的端口（默认3000）

3. **启动服务**
   ```bash
   node index.js
   ```
   或使用启动脚本：
   ```bash
   ./start.bat
   ```

4. **设置Telegram Bot**
   - 已创建Bot：@EmosCheckinBot
   - Token：通过环境变量 `TELEGRAM_BOT_TOKEN` 配置

## Telegram Bot命令

- `/add token time [random]` - 添加签到用户
  - `token`：签到所需的Bearer Token
  - `time`：签到时间，格式为 HH:MM
  - `random`：可选，添加后为随机时间签到

- `/remove token` - 删除签到用户

- `/list` - 查看当前签到用户列表

- `/help` - 显示帮助信息

## API接口

- `POST /api/register` - 注册用户
  - 参数：`token`、`time`、`random`

- `POST /api/remove` - 删除用户
  - 参数：`token`

- `GET /api/users` - 获取用户列表

- `POST /webhook/{token}` - Telegram Bot webhook

## 签到流程

1. 用户通过Telegram Bot添加签到账号
2. 系统根据设置的时间定时执行签到
3. 签到成功后获取用户信息
4. 将签到结果发送到Telegram Bot

## 注意事项

- 确保VPS有稳定的网络连接
- 确保签到API的Bearer Token有效
- 如需使用HTTPS，需要配置SSL证书
- 定期检查系统运行状态

## 故障排查

- 查看控制台日志了解运行状态
- 检查网络连接是否正常
- 验证Bearer Token是否有效
- 确认Telegram Bot webhook设置是否正确
