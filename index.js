const express = require('express');
const cron = require('node-cron');
const axios = require('axios');

const app = express();
app.use(express.json());

// 存储用户签到信息
const users = [];
// 存储定时任务
const tasks = {};

// 配置信息
const CONFIG = {
  PORT: 3000,
  API_BASE_URL: 'https://emos.best',
  TELEGRAM_BOT_TOKEN: process.env.TELEGRAM_BOT_TOKEN || '',
  TELEGRAM_API_URL: 'https://api.telegram.org/bot'
};

// 存储用户的chat_id
const chatIds = new Set();

// 注册用户
app.post('/api/register', (req, res) => {
  const { token, time, random = false } = req.body;
  
  if (!token) {
    return res.status(400).json({ error: 'Token is required' });
  }
  
  // 检查是否已存在
  const existingUserIndex = users.findIndex(user => user.token === token);
  
  if (existingUserIndex !== -1) {
    // 更新现有用户
    users[existingUserIndex] = { ...users[existingUserIndex], time, random };
    // 取消旧的定时任务
    if (tasks[token]) {
      tasks[token].stop();
    }
  } else {
    // 添加新用户
    users.push({ token, time, random });
  }
  
  // 设置定时任务
  setCheckinTask(token, time, random);
  
  res.json({ success: true, message: '用户注册成功' });
});

// 删除用户
app.post('/api/remove', (req, res) => {
  const { token } = req.body;
  
  if (!token) {
    return res.status(400).json({ error: 'Token is required' });
  }
  
  const userIndex = users.findIndex(user => user.token === token);
  
  if (userIndex === -1) {
    return res.status(404).json({ error: '用户不存在' });
  }
  
  // 取消定时任务
  if (tasks[token]) {
    tasks[token].stop();
    delete tasks[token];
  }
  
  // 删除用户
  users.splice(userIndex, 1);
  
  res.json({ success: true, message: '用户删除成功' });
});

// 获取用户列表
app.get('/api/users', (req, res) => {
  res.json(users);
});

// 设置签到任务
function setCheckinTask(token, time, random) {
  if (tasks[token]) {
    tasks[token].stop();
  }
  
  let cronExpression;
  
  if (random) {
    // 随机时间，每天0-23点之间
    cronExpression = '0 0-23 * * *';
    tasks[token] = cron.schedule(cronExpression, () => {
      // 随机执行，概率约为1/24
      if (Math.random() < 1/24) {
        performCheckin(token);
      }
    });
  } else {
    // 固定时间
    const [hour, minute] = time.split(':').map(Number);
    cronExpression = `${minute} ${hour} * * *`;
    tasks[token] = cron.schedule(cronExpression, () => {
      performCheckin(token);
    });
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
async function getUserInfo(token) {
  try {
    const response = await axios.get(`${CONFIG.API_BASE_URL}/api/user`, {
      headers: {
        'Authorization': `Bearer ${token}`
      }
    });
    return response.data;
  } catch (error) {
    console.error('获取用户信息出错:', error);
    return null;
  }
}

// 执行签到
async function checkin(token) {
  try {
    const response = await axios.post(`${CONFIG.API_BASE_URL}/api/user/sign`, {}, {
      headers: {
        'Authorization': `Bearer ${token}`
      }
    });
    return response.data;
  } catch (error) {
    console.error('签到出错:', error);
    return null;
  }
}

// 发送通知到Telegram
async function sendNotification(username, checkinResult) {
  try {
    const content = `📅 签到通知\n\n用户名: ${username}\n签到状态: 成功\n连续签到: ${checkinResult.continuous_days} 天\n获得萝卜: ${checkinResult.earn_point} 个\n签到索引: ${checkinResult.sign_index}`;
    
    // 发送到所有注册的chat_id
    for (const chatId of chatIds) {
      await axios.post(`${CONFIG.TELEGRAM_API_URL}${CONFIG.TELEGRAM_BOT_TOKEN}/sendMessage`, {
        chat_id: chatId,
        text: content
      });
    }
  } catch (error) {
    console.error('发送通知出错:', error);
  }
}

// Telegram Bot webhook
app.post(`/webhook/${CONFIG.TELEGRAM_BOT_TOKEN}`, (req, res) => {
  const update = req.body;
  
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
        const existingUserIndex = users.findIndex(user => user.token === token);
        
        if (existingUserIndex !== -1) {
          users[existingUserIndex] = { ...users[existingUserIndex], time, random };
          if (tasks[token]) {
            tasks[token].stop();
          }
        } else {
          users.push({ token, time, random });
        }
        
        setCheckinTask(token, time, random);
        
        // 回复用户
        axios.post(`${CONFIG.TELEGRAM_API_URL}${CONFIG.TELEGRAM_BOT_TOKEN}/sendMessage`, {
          chat_id: chatId,
          text: '用户添加成功！'
        });
      } else {
        // 回复用户
        axios.post(`${CONFIG.TELEGRAM_API_URL}${CONFIG.TELEGRAM_BOT_TOKEN}/sendMessage`, {
          chat_id: chatId,
          text: '格式错误，请使用: /add token time [random]'
        });
      }
    } else if (text.startsWith('/remove')) {
      // 格式: /remove token
      const parts = text.split(' ');
      if (parts.length === 2) {
        const token = parts[1];
        
        const userIndex = users.findIndex(user => user.token === token);
        
        if (userIndex !== -1) {
          if (tasks[token]) {
            tasks[token].stop();
            delete tasks[token];
          }
          users.splice(userIndex, 1);
          
          // 回复用户
          axios.post(`${CONFIG.TELEGRAM_API_URL}${CONFIG.TELEGRAM_BOT_TOKEN}/sendMessage`, {
            chat_id: chatId,
            text: '用户删除成功！'
          });
        } else {
          // 回复用户
          axios.post(`${CONFIG.TELEGRAM_API_URL}${CONFIG.TELEGRAM_BOT_TOKEN}/sendMessage`, {
            chat_id: chatId,
            text: '用户不存在！'
          });
        }
      } else {
        // 回复用户
        axios.post(`${CONFIG.TELEGRAM_API_URL}${CONFIG.TELEGRAM_BOT_TOKEN}/sendMessage`, {
          chat_id: chatId,
          text: '格式错误，请使用: /remove token'
        });
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
      axios.post(`${CONFIG.TELEGRAM_API_URL}${CONFIG.TELEGRAM_BOT_TOKEN}/sendMessage`, {
        chat_id: chatId,
        text: message
      });
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
      axios.post(`${CONFIG.TELEGRAM_API_URL}${CONFIG.TELEGRAM_BOT_TOKEN}/sendMessage`, {
        chat_id: chatId,
        text: helpMessage
      });
    }
  }
  
  res.status(200).send('OK');
});

// 设置Telegram Bot webhook
async function setWebhook() {
  try {
    // 注意：需要将your-vps-ip替换为实际的VPS IP地址
    // 并且VPS需要开放80或443端口，或者使用反向代理
    const url = `https://your-vps-ip:${CONFIG.PORT}/webhook/${CONFIG.TELEGRAM_BOT_TOKEN}`;
    await axios.post(`${CONFIG.TELEGRAM_API_URL}${CONFIG.TELEGRAM_BOT_TOKEN}/setWebhook`, {
      url: url
    });
    console.log('Webhook设置成功');
  } catch (error) {
    console.error('Webhook设置失败:', error);
  }
}

// 启动服务器
app.listen(CONFIG.PORT, () => {
  console.log(`服务器运行在 http://localhost:${CONFIG.PORT}`);
  console.log('API接口:');
  console.log('  POST /api/register - 注册用户 (token, time, random)');
  console.log('  POST /api/remove - 删除用户 (token)');
  console.log('  GET /api/users - 获取用户列表');
  console.log('  POST /webhook/{token} - Telegram Bot webhook');
  
  // 设置webhook
  setWebhook();
});
