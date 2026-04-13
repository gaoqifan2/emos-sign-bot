const http = require('http');
const url = require('url');

console.log('=== 自动签到系统测试版 ===');
console.log('启动时间:', new Date().toISOString());

// 配置信息
const CONFIG = {
  PORT: 3000,
  API_BASE_URL: 'https://emos.best',
  TELEGRAM_BOT_TOKEN: '8754758110:AAGscR-51usqNuB6hkEld7ovO_eQm5w-zCs',
  TELEGRAM_API_URL: 'https://api.telegram.org/bot'
};

// 存储用户签到信息
const users = [];
// 存储用户的chat_id
const chatIds = new Set();

// 处理HTTP请求
function handleRequest(req, res) {
  const parsedUrl = url.parse(req.url, true);
  const path = parsedUrl.pathname;
  
  console.log('接收到请求:', path, new Date().toISOString());
  
  // 处理API请求
  if (path === '/api/register') {
    let body = '';
    req.on('data', (chunk) => {
      body += chunk;
    });
    req.on('end', () => {
      try {
        const data = JSON.parse(body);
        console.log('注册用户请求:', data);
        
        if (!data.token) {
          res.writeHead(400, { 'Content-Type': 'application/json' });
          res.end(JSON.stringify({ success: false, message: 'Token is required' }));
          return;
        }
        
        // 检查是否已存在
        const existingUserIndex = users.findIndex(user => user.token === user.token);
        
        if (existingUserIndex !== -1) {
          // 更新现有用户
          users[existingUserIndex] = { ...users[existingUserIndex], time: data.time, random: data.random };
          console.log('更新用户:', data.token);
        } else {
          // 添加新用户
          users.push({ token: data.token, time: data.time, random: data.random });
          console.log('添加新用户:', data.token);
        }
        
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ success: true, message: '用户注册成功' }));
      } catch (error) {
        console.error('注册用户出错:', error);
        res.writeHead(400, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ success: false, message: 'Invalid request' }));
      }
    });
  } else if (path === '/api/remove') {
    let body = '';
    req.on('data', (chunk) => {
      body += chunk;
    });
    req.on('end', () => {
      try {
        const data = JSON.parse(body);
        console.log('删除用户请求:', data);
        
        if (!data.token) {
          res.writeHead(400, { 'Content-Type': 'application/json' });
          res.end(JSON.stringify({ success: false, message: 'Token is required' }));
          return;
        }
        
        const userIndex = users.findIndex(user => user.token === data.token);
        
        if (userIndex === -1) {
          res.writeHead(404, { 'Content-Type': 'application/json' });
          res.end(JSON.stringify({ success: false, message: '用户不存在' }));
          return;
        }
        
        // 删除用户
        users.splice(userIndex, 1);
        console.log('删除用户:', data.token);
        
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ success: true, message: '用户删除成功' }));
      } catch (error) {
        console.error('删除用户出错:', error);
        res.writeHead(400, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ success: false, message: 'Invalid request' }));
      }
    });
  } else if (path === '/api/users') {
    console.log('获取用户列表请求');
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify(users));
  } else if (path.startsWith(`/webhook/${CONFIG.TELEGRAM_BOT_TOKEN}`)) {
    console.log('接收到Telegram webhook请求');
    let body = '';
    req.on('data', (chunk) => {
      body += chunk;
    });
    req.on('end', () => {
      try {
        const update = JSON.parse(body);
        console.log('Telegram更新:', JSON.stringify(update, null, 2));
        
        if (update.message) {
          const chatId = update.message.chat.id;
          const text = update.message.text;
          
          // 存储chat_id
          chatIds.add(chatId);
          console.log('存储chat_id:', chatId);
          
          // 处理命令
          if (text.startsWith('/add')) {
            console.log('处理/add命令:', text);
            const parts = text.split(' ');
            if (parts.length >= 3) {
              const token = parts[1];
              const time = parts[2];
              const random = parts[3] === 'random';
              
              // 注册用户
              const existingUserIndex = users.findIndex(user => user.token === token);
              
              if (existingUserIndex !== -1) {
                users[existingUserIndex] = { ...users[existingUserIndex], time, random };
                console.log('更新用户:', token);
              } else {
                users.push({ token, time, random });
                console.log('添加新用户:', token);
              }
              
              // 回复用户
              sendTelegramMessage(chatId, '用户添加成功！');
            } else {
              // 回复用户
              sendTelegramMessage(chatId, '格式错误，请使用: /add token time [random]');
            }
          } else if (text.startsWith('/remove')) {
            console.log('处理/remove命令:', text);
            const parts = text.split(' ');
            if (parts.length === 2) {
              const token = parts[1];
              
              const userIndex = users.findIndex(user => user.token === token);
              
              if (userIndex !== -1) {
                users.splice(userIndex, 1);
                console.log('删除用户:', token);
                // 回复用户
                sendTelegramMessage(chatId, '用户删除成功！');
              } else {
                // 回复用户
                sendTelegramMessage(chatId, '用户不存在！');
              }
            } else {
              // 回复用户
              sendTelegramMessage(chatId, '格式错误，请使用: /remove token');
            }
          } else if (text === '/list') {
            console.log('处理/list命令');
            // 显示用户列表
            let message = '当前签到用户列表:\n';
            users.forEach((user, index) => {
              message += `${index + 1}. Token: ${user.token.substring(0, 20)}...\n`;
              message += `   时间: ${user.random ? '随机' : user.time}\n`;
            });
            
            if (users.length === 0) {
              message = '当前没有签到用户！';
            }
            
            // 回复用户
            sendTelegramMessage(chatId, message);
          } else if (text === '/help') {
            console.log('处理/help命令');
            // 显示帮助信息
            const helpMessage = `使用说明:\n\n` +
              `/add token time [random] - 添加签到用户\n` +
              `  token: 签到所需的Bearer Token\n` +
              `  time: 签到时间，格式为 HH:MM\n` +
              `  random: 可选，添加后为随机时间签到\n\n` +
              `/remove token - 删除签到用户\n` +
              `/list - 查看当前签到用户列表\n` +
              `/help - 显示帮助信息`;
            
            // 回复用户
            sendTelegramMessage(chatId, helpMessage);
          } else {
            console.log('接收到普通消息:', text);
            sendTelegramMessage(chatId, '请使用 /help 查看可用命令');
          }
        }
        
        res.writeHead(200, { 'Content-Type': 'text/plain' });
        res.end('OK');
      } catch (error) {
        console.error('处理webhook出错:', error);
        res.writeHead(200, { 'Content-Type': 'text/plain' });
        res.end('OK');
      }
    });
  } else {
    console.log('未知路径:', path);
    res.writeHead(404, { 'Content-Type': 'text/plain' });
    res.end('Not Found');
  }
}

// 发送消息到Telegram
function sendTelegramMessage(chatId, text) {
  console.log('发送消息到Telegram:', chatId, text);
  
  return new Promise((resolve, reject) => {
    const postData = JSON.stringify({
      chat_id: chatId,
      text: text
    });
    
    const options = {
      hostname: url.parse(CONFIG.TELEGRAM_API_URL).hostname,
      path: `/${CONFIG.TELEGRAM_BOT_TOKEN}/sendMessage`,
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Content-Length': Buffer.byteLength(postData)
      }
    };
    
    const req = http.request(options, (res) => {
      let data = '';
      res.on('data', (chunk) => {
        data += chunk;
      });
      res.on('end', () => {
        console.log('Telegram消息发送成功:', data);
        resolve();
      });
    });
    
    req.on('error', (error) => {
      console.error('Telegram消息发送失败:', error);
      reject(error);
    });
    
    req.write(postData);
    req.end();
  });
}

// 设置Telegram Bot webhook
function setWebhook() {
  console.log('开始设置webhook...');
  
  return new Promise((resolve, reject) => {
    // 注意：需要将your-vps-ip替换为实际的VPS IP地址
    const webhookUrl = `https://your-vps-ip:${CONFIG.PORT}/webhook/${CONFIG.TELEGRAM_BOT_TOKEN}`;
    const postData = JSON.stringify({
      url: webhookUrl
    });
    
    console.log('设置webhook URL:', webhookUrl);
    
    const options = {
      hostname: url.parse(CONFIG.TELEGRAM_API_URL).hostname,
      path: `/${CONFIG.TELEGRAM_BOT_TOKEN}/setWebhook`,
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Content-Length': Buffer.byteLength(postData)
      }
    };
    
    const req = http.request(options, (res) => {
      let data = '';
      res.on('data', (chunk) => {
        data += chunk;
      });
      res.on('end', () => {
        console.log('Webhook设置响应:', data);
        resolve(data);
      });
    });
    
    req.on('error', (error) => {
      console.error('Webhook设置失败:', error);
      reject(error);
    });
    
    req.write(postData);
    req.end();
  });
}

// 创建HTTP服务器
const server = http.createServer(handleRequest);

// 启动服务器
server.listen(CONFIG.PORT, () => {
  console.log(`服务器运行在 http://localhost:${CONFIG.PORT}`);
  console.log('API接口:');
  console.log('  POST /api/register - 注册用户 (token, time, random)');
  console.log('  POST /api/remove - 删除用户 (token)');
  console.log('  GET /api/users - 获取用户列表');
  console.log('  POST /webhook/{token} - Telegram Bot webhook');
  
  console.log('系统启动成功，等待接收命令...');
  
  // 设置webhook
  setWebhook().then((response) => {
    console.log('Webhook设置完成:', response);
  }).catch((error) => {
    console.error('Webhook设置失败:', error);
  });
  
  // 定期输出系统状态
  setInterval(() => {
    console.log(`系统运行中 - 当前用户数: ${users.length}, 当前chat_id数: ${chatIds.size}`);
  }, 60000); // 每分钟输出一次
});

// 处理未捕获的错误
process.on('uncaughtException', (error) => {
  console.error('未捕获的错误:', error);
});

// 处理未处理的Promise拒绝
process.on('unhandledRejection', (reason, promise) => {
  console.error('未处理的Promise拒绝:', reason);
});

console.log('系统初始化完成，等待启动...');