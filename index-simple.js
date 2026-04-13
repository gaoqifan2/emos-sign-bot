const http = require('http');
const url = require('url');
const querystring = require('querystring');

// 存储用户签到信息
const users = [];
// 存储定时任务
const tasks = {};
// 存储用户的chat_id
const chatIds = new Set();

// 配置信息
const CONFIG = {
  PORT: 3000,
  API_BASE_URL: 'https://emos.best',
  TELEGRAM_BOT_TOKEN: '8754758110:AAGscR-51usqNuB6hkEld7ovO_eQm5w-zCs',
  TELEGRAM_API_URL: 'https://api.telegram.org/bot'
};

// 注册用户
function registerUser(token, time, random = false) {
  if (!token) {
    return { success: false, message: 'Token is required' };
  }
  
  // 检查是否已存在
  const existingUserIndex = users.findIndex(user => user.token === token);
  
  if (existingUserIndex !== -1) {
    // 更新现有用户
    users[existingUserIndex] = { ...users[existingUserIndex], time, random };
    // 取消旧的定时任务
    if (tasks[token]) {
      clearInterval(tasks[token]);
    }
  } else {
    // 添加新用户
    users.push({ token, time, random });
  }
  
  // 设置定时任务
  setCheckinTask(token, time, random);
  
  return { success: true, message: '用户注册成功' };
}

// 删除用户
function removeUser(token) {
  if (!token) {
    return { success: false, message: 'Token is required' };
  }
  
  const userIndex = users.findIndex(user => user.token === token);
  
  if (userIndex === -1) {
    return { success: false, message: '用户不存在' };
  }
  
  // 取消定时任务
  if (tasks[token]) {
    clearInterval(tasks[token]);
    delete tasks[token];
  }
  
  // 删除用户
  users.splice(userIndex, 1);
  
  return { success: true, message: '用户删除成功' };
}

// 设置签到任务
function setCheckinTask(token, time, random) {
  if (tasks[token]) {
    clearInterval(tasks[token]);
  }
  
  if (random) {
    // 随机时间，每小时检查一次
    tasks[token] = setInterval(() => {
      // 随机执行，概率约为1/24
      if (Math.random() < 1/24) {
        performCheckin(token);
      }
    }, 60 * 60 * 1000); // 每小时检查一次
  } else {
    // 固定时间
    const [hour, minute] = time.split(':').map(Number);
    // 每天检查一次
    tasks[token] = setInterval(() => {
      const now = new Date();
      if (now.getHours() === hour && now.getMinutes() === minute) {
        performCheckin(token);
      }
    }, 60 * 1000); // 每分钟检查一次
  }
  
  console.log(`设置签到任务: ${token} ${random ? '随机时间' : time}`);
}

// 执行签到
async function performCheckin(token) {
  try {
    // 获取用户信息
    const userInfo = await getUserInfo(token);
    if (!userInfo) {
      console.error(`获取用户信息失败: ${token}`);
      return;
    }
    
    // 执行签到
    const checkinResult = await checkin(token);
    if (!checkinResult) {
      console.error(`签到失败: ${token}`);
      return;
    }
    
    // 发送通知
    await sendNotification(userInfo.username, checkinResult);
    
    console.log(`签到成功: ${userInfo.username}`);
  } catch (error) {
    console.error(`签到过程出错: ${token}`, error);
  }
}

// 获取用户信息
function getUserInfo(token) {
  return new Promise((resolve, reject) => {
    const options = {
      hostname: url.parse(CONFIG.API_BASE_URL).hostname,
      path: '/api/user',
      method: 'GET',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      }
    };
    
    const req = http.request(options, (res) => {
      let data = '';
      res.on('data', (chunk) => {
        data += chunk;
      });
      res.on('end', () => {
        try {
          resolve(JSON.parse(data));
        } catch (error) {
          reject(error);
        }
      });
    });
    
    req.on('error', (error) => {
      reject(error);
    });
    
    req.end();
  });
}

// 执行签到
function checkin(token) {
  return new Promise((resolve, reject) => {
    const options = {
      hostname: url.parse(CONFIG.API_BASE_URL).hostname,
      path: '/api/user/sign',
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      }
    };
    
    const req = http.request(options, (res) => {
      let data = '';
      res.on('data', (chunk) => {
        data += chunk;
      });
      res.on('end', () => {
        try {
          resolve(JSON.parse(data));
        } catch (error) {
          reject(error);
        }
      });
    });
    
    req.on('error', (error) => {
      reject(error);
    });
    
    req.end();
  });
}

// 发送通知到Telegram
function sendNotification(username, checkinResult) {
  return new Promise((resolve, reject) => {
    const content = `📅 签到通知\n\n用户名: ${username}\n签到状态: 成功\n连续签到: ${checkinResult.continuous_days} 天\n获得萝卜: ${checkinResult.earn_point} 个\n签到索引: ${checkinResult.sign_index}`;
    
    // 发送到所有注册的chat_id
    const promises = Array.from(chatIds).map(chatId => {
      return new Promise((resolve, reject) => {
        const postData = JSON.stringify({
          chat_id: chatId,
          text: content
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
          resolve();
        });
        
        req.on('error', (error) => {
          reject(error);
        });
        
        req.write(postData);
        req.end();
      });
    });
    
    Promise.all(promises).then(() => {
      resolve();
    }).catch((error) => {
      reject(error);
    });
  });
}

// 发送消息到Telegram
function sendTelegramMessage(chatId, text) {
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
      resolve();
    });
    
    req.on('error', (error) => {
      reject(error);
    });
    
    req.write(postData);
    req.end();
  });
}

// 设置Telegram Bot webhook
function setWebhook() {
  return new Promise((resolve, reject) => {
    // 注意：需要将your-vps-ip替换为实际的VPS IP地址
    // 并且VPS需要开放80或443端口，或者使用反向代理
    const webhookUrl = `https://your-vps-ip:${CONFIG.PORT}/webhook/${CONFIG.TELEGRAM_BOT_TOKEN}`;
    const postData = JSON.stringify({
      url: webhookUrl
    });
    
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
        console.log('Webhook设置成功:', data);
        resolve();
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

// 处理HTTP请求
function handleRequest(req, res) {
  const parsedUrl = url.parse(req.url, true);
  const path = parsedUrl.pathname;
  
  // 处理API请求
  if (path === '/api/register') {
    let body = '';
    req.on('data', (chunk) => {
      body += chunk;
    });
    req.on('end', () => {
      try {
        const data = JSON.parse(body);
        const result = registerUser(data.token, data.time, data.random);
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify(result));
      } catch (error) {
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
        const result = removeUser(data.token);
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify(result));
      } catch (error) {
        res.writeHead(400, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ success: false, message: 'Invalid request' }));
      }
    });
  } else if (path === '/api/users') {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify(users));
  } else if (path.startsWith(`/webhook/${CONFIG.TELEGRAM_BOT_TOKEN}`)) {
    // 处理Telegram Bot webhook
    let body = '';
    req.on('data', (chunk) => {
      body += chunk;
    });
    req.on('end', () => {
      try {
        const update = JSON.parse(body);
        
        if (update.message) {
          const chatId = update.message.chat.id;
          const text = update.message.text;
          
          // 存储chat_id
          chatIds.add(chatId);
          
          // 处理命令
          if (text.startsWith('/add')) {
            // 格式: /add token time [random]
            const parts = text.split(' ');
            if (parts.length >= 3) {
              const token = parts[1];
              const time = parts[2];
              const random = parts[3] === 'random';
              
              // 注册用户
              const result = registerUser(token, time, random);
              
              // 回复用户
              sendTelegramMessage(chatId, result.message);
            } else {
              // 回复用户
              sendTelegramMessage(chatId, '格式错误，请使用: /add token time [random]');
            }
          } else if (text.startsWith('/remove')) {
            // 格式: /remove token
            const parts = text.split(' ');
            if (parts.length === 2) {
              const token = parts[1];
              
              const result = removeUser(token);
              
              // 回复用户
              sendTelegramMessage(chatId, result.message);
            } else {
              // 回复用户
              sendTelegramMessage(chatId, '格式错误，请使用: /remove token');
            }
          } else if (text === '/list') {
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
          }
        }
        
        res.writeHead(200, { 'Content-Type': 'text/plain' });
        res.end('OK');
      } catch (error) {
        res.writeHead(200, { 'Content-Type': 'text/plain' });
        res.end('OK');
      }
    });
  } else {
    res.writeHead(404, { 'Content-Type': 'text/plain' });
    res.end('Not Found');
  }
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
  setWebhook().then(() => {
    console.log('Webhook设置完成');
  }).catch((error) => {
    console.error('Webhook设置失败:', error);
  });
  
  // 定期输出系统状态
  setInterval(() => {
    console.log(`系统运行中 - 当前用户数: ${users.length}`);
  }, 60000); // 每分钟输出一次
});