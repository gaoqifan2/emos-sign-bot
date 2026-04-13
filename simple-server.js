const http = require('http');
const url = require('url');

console.log('=== 自动签到系统简化版 ===');
console.log('启动时间:', new Date().toISOString());

// 配置信息
const CONFIG = {
  PORT: 3000,
  API_BASE_URL: 'https://emos.best'
};

// 存储用户签到信息
const users = [];

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
        const existingUserIndex = users.findIndex(user => user.token === data.token);
        
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
  } else {
    console.log('未知路径:', path);
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
  
  console.log('系统启动成功，等待接收命令...');
  
  // 定期输出系统状态
  setInterval(() => {
    console.log(`系统运行中 - 当前用户数: ${users.length}`);
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