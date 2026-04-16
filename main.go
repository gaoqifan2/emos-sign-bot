package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 移除init函数，使用中文输出

// 配置信息
type Config struct {
	Port             int    `json:"port"`
	ApiBaseURL       string `json:"api_base_url"`
	TelegramBotToken string `json:"telegram_bot_token"`
	TelegramApiURL   string `json:"telegram_api_url"`
	ProxyURL         string `json:"proxy_url"` // 代理URL
	EnableTelegram   bool   `json:"enable_telegram"` // 是否启用Telegram
	DataFile         string `json:"data_file"` // 数据文件路径
}

// 用户信息
type User struct {
	Token      string `json:"token"`
	Time       string `json:"time"`
	Random     bool   `json:"random"`
	RandomTime string `json:"random_time"` // 当天的随机签到时间
	Remark     string `json:"remark"`      // 用户备注
	Username   string `json:"username"`    // 用户名
}

// 签到结果
type CheckinResult struct {
	SignIndex       int `json:"sign_index"`
	EarnPoint       int `json:"earn_point"`
	ContinuousDays  int `json:"continuous_days"`
}

// 用户信息
type UserInfo struct {
	Username string `json:"username"`
}

// 数据存储结构
type DataStorage struct {
	Users   []User          `json:"users"`
	ChatIds map[int64]bool  `json:"chat_ids"`
}

var (
	config     Config
	users      []User
	chatIds    = make(map[int64]bool)
	usersMutex sync.Mutex
	beijingLoc *time.Location // 北京时间时区
	// 用户状态管理
	userStates = make(map[int64]UserState)
	stateMutex sync.Mutex
)

// 用户状态类型
type StateType string

const (
	StateIdle          StateType = "idle"
	StateWaitToken     StateType = "wait_token"
	StateWaitMode      StateType = "wait_mode"
	StateWaitTime      StateType = "wait_time"
	StateWaitRemark    StateType = "wait_remark"
	StateWaitRemoveOpt StateType = "wait_remove_opt"
	StateWaitRemoveUser StateType = "wait_remove_user"
)

// 用户状态
type UserState struct {
	Type       StateType
	Data       map[string]string
	CreateTime time.Time
}

// 初始化日志文件
func initLog() {
	// 创建或清空日志文件
	logFile, err := os.Create("log.txt")
	if err != nil {
		// 日志文件创建失败，尝试输出错误信息
		os.Stderr.Write([]byte(fmt.Sprintf("创建日志文件失败: %v\n", err)))
		return
	}
	defer logFile.Close()
	
	// 写入初始化信息
	initMessage := fmt.Sprintf("日志文件初始化 - %s\n", time.Now().Format(time.RFC3339))
	_, err = logFile.Write([]byte(initMessage))
	if err != nil {
		// 日志写入失败，尝试输出错误信息
		os.Stderr.Write([]byte(fmt.Sprintf("写入日志文件失败: %v\n", err)))
	}
}

// 写入日志到文件
func writeLog(s string) {
	// 确保字符串以换行符结尾
	if len(s) > 0 && s[len(s)-1] != '\n' {
		s += "\n"
	}
	
	// 打开或创建日志文件
	logFile, err := os.OpenFile("log.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		// 日志文件创建失败，尝试输出错误信息
		os.Stderr.Write([]byte(fmt.Sprintf("创建日志文件失败: %v\n", err)))
		return
	}
	defer logFile.Close()
	
	// 写入日志
	_, err = logFile.Write([]byte(s))
	if err != nil {
		// 日志写入失败，尝试输出错误信息
		os.Stderr.Write([]byte(fmt.Sprintf("写入日志文件失败: %v\n", err)))
	}
}

// 直接输出UTF-8编码的字符串
func printlnUTF8(s string) {
	defer func() {
		if r := recover(); r != nil {
			// 捕获panic，确保程序不会崩溃
			// 尝试使用更简单的方式输出错误信息
			os.Stderr.Write([]byte(fmt.Sprintf("printlnUTF8 panic: %v\n", r)))
		}
	}()
	
	n := len(s)
	if n > 0 && s[n-1] != '\n' {
		s += "\n"
	}
	n = len(s)
	
	// 同时输出到日志文件
	defer func() {
		if r := recover(); r != nil {
			// 捕获writeLog的panic
			os.Stderr.Write([]byte(fmt.Sprintf("writeLog panic: %v\n", r)))
		}
	}()
	writeLog(s)
	
	// 直接使用fmt.Println函数
	fmt.Println(s)
}

func main() {
	// 初始化日志文件
	initLog()
	
	// 初始化随机种子
	rand.Seed(time.Now().UnixNano())
	
	// 初始化北京时间时区
	var err error
	beijingLoc, err = time.LoadLocation("Asia/Shanghai")
	if err != nil {
		printlnUTF8(fmt.Sprintf("加载时区失败: %v，使用本地时区", err))
		beijingLoc = time.Local
	} else {
		printlnUTF8("成功加载北京时间时区 (Asia/Shanghai)")
	}

	// 初始化配置
	initConfig()
	
	// 网络设置：强制使用IPv4并支持代理
	setupNetwork()
	
	// 加载数据
	loadData()
	
	// 测试日志写入
	printlnUTF8("测试日志写入功能")
	printlnUTF8("这是一条中文测试消息")
	printlnUTF8("This is an English test message")

	// 打印系统信息
	printlnUTF8("=== 自动签到系统 (Go版本) ===")
	printlnUTF8(fmt.Sprintf("启动时间: %s", time.Now().In(beijingLoc).Format(time.RFC3339)))
	printlnUTF8(fmt.Sprintf("服务器运行在: http://localhost:%d", config.Port))
	printlnUTF8("API接口:")
	printlnUTF8("  POST /api/register - 注册用户 (token, time, random)")
	printlnUTF8("  POST /api/remove - 删除用户 (token)")
	printlnUTF8("  GET /api/users - 获取用户列表")
	printlnUTF8("  POST /webhook - Telegram Bot webhook")
	printlnUTF8("系统启动成功，等待命令...")
	printlnUTF8(fmt.Sprintf("已加载 %d 个用户，%d 个聊天ID", len(users), len(chatIds)))

	// 设置路由
	http.HandleFunc("/api/register", registerUser)
	http.HandleFunc("/api/remove", removeUser)
	http.HandleFunc("/api/users", getUsers)
	http.HandleFunc("/webhook", handleWebhook)

	// 暂时禁用服务器功能，避免端口冲突
	// go func() {
	// 	if err := http.ListenAndServe(fmt.Sprintf(":%d", config.Port), nil); err != nil {
	// 		log.Printf("服务器启动失败: %v", err)
	// 	}
	// }()

	// 启动轮询获取Telegram消息（如果启用）
	if config.EnableTelegram {
		go func() {
			printlnUTF8("Telegram轮询线程已启动")
			for {
				printlnUTF8("启动Telegram轮询...")
				// 捕获panic
				defer func() {
					if r := recover(); r != nil {
						printlnUTF8(fmt.Sprintf("Telegram轮询崩溃: %v", r))
					}
				}()
				telegramPolling()
				printlnUTF8("Telegram轮询退出，5秒后重新启动...")
				time.Sleep(5 * time.Second)
			}
		}()
		printlnUTF8("Telegram轮询已启用")
	} else {
		printlnUTF8("Telegram轮询已禁用")
	}

	// 定期检查签到
	go func() {
		defer func() {
			if r := recover(); r != nil {
				printlnUTF8(fmt.Sprintf("签到调度器崩溃: %v", r))
			}
		}()
		checkinScheduler()
	}()

	// 定期输出系统状态
	go func() {
		defer func() {
			if r := recover(); r != nil {
				printlnUTF8(fmt.Sprintf("系统状态监控崩溃: %v", r))
			}
		}()
		statusScheduler()
	}()

	// 保持程序运行
	select {}
}

// 初始化配置
func initConfig() {
	config = Config{
		Port:             3001,
		ApiBaseURL:       "https://emos.best",
		TelegramBotToken: "8754758110:AAGscR-51usqNuB6hkEld7ovO_eQm5w-zCs",
		TelegramApiURL:   "https://api.telegram.org/bot",
		ProxyURL:         "http://127.0.0.1:7897", // 代理URL，根据实际情况修改
		EnableTelegram:   true, // 是否启用Telegram
		DataFile:         "data.json", // 数据文件路径
	}
}

// 加载数据
func loadData() {
	usersMutex.Lock()
	defer usersMutex.Unlock()
	
	// 读取数据文件
	data, err := os.ReadFile(config.DataFile)
	if err != nil {
		if os.IsNotExist(err) {
			printlnUTF8("数据文件不存在，使用空数据")
			return
		}
		printlnUTF8(fmt.Sprintf("读取数据文件失败: %v，使用空数据", err))
		return
	}
	
	// 解析数据
	var storage DataStorage
	if err := json.Unmarshal(data, &storage); err != nil {
		printlnUTF8(fmt.Sprintf("解析数据文件失败: %v，使用空数据", err))
		return
	}
	
	// 加载数据
	users = storage.Users
	if storage.ChatIds != nil {
		chatIds = storage.ChatIds
	}
	
	// 重置所有随机签到用户的随机时间，以便在程序启动时重新生成
	resetCount := 0
	for i, user := range users {
		if user.Random {
			users[i].RandomTime = ""
			resetCount++
		}
	}
	
	// 只有当有用户数据并且重置了随机时间时，才保存数据
	if len(users) > 0 && resetCount > 0 {
		saveData()
		printlnUTF8(fmt.Sprintf("已重置 %d 个随机签到用户的随机时间，将在启动后重新生成", resetCount))
	}
	
	printlnUTF8(fmt.Sprintf("成功加载数据: %d 个用户，%d 个聊天ID", len(users), len(chatIds)))
}

// 保存数据
func saveData() {
	printlnUTF8(fmt.Sprintf("开始保存数据: %d 个用户，%d 个聊天ID", len(users), len(chatIds)))
	
	// 准备数据
	storage := DataStorage{
		Users:   users,
		ChatIds: chatIds,
	}
	
	// 序列化数据
	data, err := json.MarshalIndent(storage, "", "  ")
	if err != nil {
		printlnUTF8(fmt.Sprintf("序列化数据失败: %v", err))
		return
	}
	
	printlnUTF8(fmt.Sprintf("序列化成功，数据长度: %d 字节", len(data)))
	
	// 写入文件
	if err := os.WriteFile(config.DataFile, data, 0644); err != nil {
		printlnUTF8(fmt.Sprintf("写入数据文件失败: %v", err))
		return
	}
	
	printlnUTF8(fmt.Sprintf("成功保存数据到 %s: %d 个用户，%d 个聊天ID", config.DataFile, len(users), len(chatIds)))
}

// 设置网络配置
func setupNetwork() {
	// 强制使用IPv4解析
	net.DefaultResolver = &net.Resolver{
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			// 只使用IPv4
			d := net.Dialer{}
			return d.DialContext(ctx, "tcp4", address)
		},
	}
	
	// 配置代理：优先使用系统环境变量中的代理设置
	proxyFunc := http.ProxyFromEnvironment
	
	// 如果配置了代理URL，则使用配置的代理
	if config.ProxyURL != "" {
		proxyURL, err := url.Parse(config.ProxyURL)
		if err == nil {
			proxyFunc = http.ProxyURL(proxyURL)
			printlnUTF8(fmt.Sprintf("使用配置的代理: %s", config.ProxyURL))
		} else {
			printlnUTF8(fmt.Sprintf("无效的代理URL，使用系统代理: %v", err))
			proxyFunc = http.ProxyFromEnvironment
		}
	} else {
		printlnUTF8("使用系统环境变量中的代理")
	}
	
	// 支持代理设置，强制使用IPv4
	http.DefaultTransport = &http.Transport{
		Proxy: proxyFunc,
		Dial: func(network, addr string) (net.Conn, error) {
			// 强制使用IPv4
			return net.Dial("tcp4", addr)
		},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// 强制使用IPv4
			d := net.Dialer{}
			return d.DialContext(ctx, "tcp4", addr)
		},
		TLSHandshakeTimeout: 10 * time.Second,
	}
	
	// 测试代理连接
	testProxyConnection()
	
	printlnUTF8("网络设置完成: 使用IPv4和代理支持")
}

// 测试代理连接
func testProxyConnection() {
	printlnUTF8("测试代理连接...")
	
	// 创建一个带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// 测试连接到Telegram API
	url := "https://api.telegram.org/bot" + config.TelegramBotToken + "/getMe"
	
	// 创建请求
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		printlnUTF8(fmt.Sprintf("代理测试失败: %v", err))
		printlnUTF8("请检查您的代理设置")
		printlnUTF8("系统将继续运行，但Telegram功能可能受限")
		return
	}
	
	// 使用配置的Transport
	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: http.DefaultTransport,
	}
	
	resp, err := client.Do(req)
	if err != nil {
		printlnUTF8(fmt.Sprintf("代理测试失败: %v", err))
		printlnUTF8("请检查您的代理设置")
		printlnUTF8("系统将继续运行，但Telegram功能可能受限")
	} else {
		defer resp.Body.Close()
		printlnUTF8(fmt.Sprintf("代理测试成功! 状态码: %d", resp.StatusCode))
	}
}

// 注册用户
func registerUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Token  string `json:"token"`
		Time   string `json:"time"`
		Random bool   `json:"random"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Token == "" {
		http.Error(w, "Token is required", http.StatusBadRequest)
		return
	}

	usersMutex.Lock()
	defer usersMutex.Unlock()

	// 检查是否已存在
	found := false
	for i, user := range users {
		if user.Token == req.Token {
			// 更新现有用户
			users[i] = User{
				Token:  req.Token,
				Time:   req.Time,
				Random: req.Random,
			}
			found = true
			printlnUTF8(fmt.Sprintf("更新用户: %s", req.Token))
			break
		}
	}

	if !found {
		// 添加新用户
		users = append(users, User{
			Token:  req.Token,
			Time:   req.Time,
			Random: req.Random,
		})
		printlnUTF8(fmt.Sprintf("添加新用户: %s", req.Token))
	}
	
	// 保存数据
	saveData()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "用户注册成功",
	})
}

// 删除用户
func removeUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Token string `json:"token"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Token == "" {
		http.Error(w, "Token is required", http.StatusBadRequest)
		return
	}

	usersMutex.Lock()
	defer usersMutex.Unlock()

	// 查找并删除用户
	found := false
	for i, user := range users {
		if user.Token == req.Token {
			// 删除用户
			users = append(users[:i], users[i+1:]...)
			found = true
			printlnUTF8(fmt.Sprintf("删除用户: %s", req.Token))
			break
		}
	}

	if !found {
		http.Error(w, "用户不存在", http.StatusNotFound)
		return
	}
	
	// 保存数据
	saveData()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "用户删除成功",
	})
}

// 获取用户列表
func getUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	usersMutex.Lock()
	defer usersMutex.Unlock()

	printlnUTF8("获取用户列表请求")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

// 处理Telegram webhook
func handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var update struct {
		Message struct {
			Chat struct {
				ID int64 `json:"id"`
			} `json:"chat"`
			Text string `json:"text"`
		} `json:"message"`
	}

	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if update.Message.Text != "" {
		chatID := update.Message.Chat.ID
		text := update.Message.Text

		// 存储chat_id
		chatIds[chatID] = true
		printlnUTF8(fmt.Sprintf("存储chat_id: %d", chatID))

		// 检查用户状态
		stateMutex.Lock()
		state, exists := userStates[chatID]
		stateMutex.Unlock()

		// 如果用户有状态，处理状态相关的输入
		if exists {
			handleUserState(chatID, text, state)
			return
		}

		// 处理命令
		trimmedText := strings.TrimSpace(text)
		// 调试日志：打印处理命令前的信息
		printlnUTF8("=== 处理命令 ===")
		printlnUTF8(fmt.Sprintf("原始文本: '%s'", text))
		printlnUTF8(fmt.Sprintf("去除空格后: '%s'", trimmedText))
		
		// 处理 /add 命令
		if strings.HasPrefix(trimmedText, "/add") {
			// 检查是否是单独的 /add 命令
			if trimmedText == "/add" {
				// 开始一步一步添加用户
				printlnUTF8("调用 startAddUser")
				startAddUser(chatID)
			} else {
				// 兼容旧格式
				printlnUTF8("调用 handleAddCommand (旧格式)")
				handleAddCommand(chatID, text)
			}
		} else if strings.HasPrefix(trimmedText, "/remove") {
			// 检查是否是单独的 /remove 命令
			if trimmedText == "/remove" {
				// 开始一步一步删除用户
				startRemoveUser(chatID)
			} else {
				// 兼容旧格式
				handleRemoveCommand(chatID, text)
			}
		} else if trimmedText == "/list" {
			handleListCommand(chatID)
		} else if trimmedText == "/help" {
			handleHelpCommand(chatID)
		} else {
			sendTelegramMessage(chatID, "请使用 /help 查看可用命令")
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// 开始添加用户流程
func startAddUser(chatID int64) {
	// 设置用户状态为等待输入token
	stateMutex.Lock()
	userStates[chatID] = UserState{
		Type:       StateWaitToken,
		Data:       make(map[string]string),
		CreateTime: time.Now(),
	}
	stateMutex.Unlock()

	// 发送提示消息
	sendTelegramMessage(chatID, "请输入签到用的Bearer Token:")
}

// 开始删除用户流程
func startRemoveUser(chatID int64) {
	// 设置用户状态为等待选择删除方式
	stateMutex.Lock()
	userStates[chatID] = UserState{
		Type:       StateWaitRemoveOpt,
		Data:       make(map[string]string),
		CreateTime: time.Now(),
	}
	stateMutex.Unlock()

	// 发送提示消息
	sendTelegramMessage(chatID, "请选择删除方式:\n1. 根据Token删除\n2. 根据用户名删除")
}

// 处理用户状态
func handleUserState(chatID int64, text string, state UserState) {
	switch state.Type {
	case StateWaitToken:
		handleWaitToken(chatID, text)
	case StateWaitMode:
		handleWaitMode(chatID, text, state.Data)
	case StateWaitTime:
		handleWaitTime(chatID, text, state.Data)
	case StateWaitRemark:
		handleWaitRemark(chatID, text, state.Data)
	case StateWaitRemoveOpt:
		handleWaitRemoveOpt(chatID, text)
	case StateWaitRemoveUser:
		handleWaitRemoveUser(chatID, text, state.Data)
	}
}

// 处理等待token状态
func handleWaitToken(chatID int64, text string) {
	// 检查是否输入0退出
	if text == "0" {
		// 清除用户状态
		stateMutex.Lock()
		delete(userStates, chatID)
		stateMutex.Unlock()
		sendTelegramMessage(chatID, "已取消添加用户")
		return
	}

	// 检查token是否为空
	if strings.TrimSpace(text) == "" {
		sendTelegramMessage(chatID, "Token不能为空，请重新输入:")
		return
	}

	// 尝试获取用户信息，检查token是否有效
	userInfo, err := getUserInfo(text)
	if err != nil {
		sendTelegramMessage(chatID, "获取用户信息失败，Token无效，请重新输入:")
		return
	}

	// 保存token和username并进入下一步
	stateMutex.Lock()
	userStates[chatID] = UserState{
		Type: StateWaitMode,
		Data: map[string]string{
			"token":     text,
			"username": userInfo.Username,
		},
		CreateTime: time.Now(),
	}
	stateMutex.Unlock()

	// 发送提示消息
	sendTelegramMessage(chatID, "请选择签到模式:\n1. 固定时间签到\n2. 随机时间签到\n输入0取消")
}

// 处理等待模式状态
func handleWaitMode(chatID int64, text string, data map[string]string) {
	// 检查是否输入0退出
	if text == "0" {
		// 清除用户状态
		stateMutex.Lock()
		delete(userStates, chatID)
		stateMutex.Unlock()
		sendTelegramMessage(chatID, "已取消添加用户")
		return
	}

	// 检查输入是否有效
	if text != "1" && text != "2" {
		sendTelegramMessage(chatID, "请输入正确的选项(1或2):")
		return
	}

	// 保存模式并进入下一步
	random := text == "2"
	data["random"] = fmt.Sprintf("%v", random)

	stateMutex.Lock()
	userStates[chatID] = UserState{
		Type:       StateWaitTime,
		Data:       data,
		CreateTime: time.Now(),
	}
	stateMutex.Unlock()

	// 发送提示消息
	if random {
		sendTelegramMessage(chatID, "已选择随机时间签到，请输入一个参考时间(格式: HH:MM:SS，例如: 08:30:00):\n输入0取消")
	} else {
		sendTelegramMessage(chatID, "请输入固定签到时间(格式: HH:MM:SS，例如: 08:30:00):\n输入0取消")
	}
}

// 处理等待时间状态
func handleWaitTime(chatID int64, text string, data map[string]string) {
	// 检查是否输入0退出
	if text == "0" {
		// 清除用户状态
		stateMutex.Lock()
		delete(userStates, chatID)
		stateMutex.Unlock()
		sendTelegramMessage(chatID, "已取消添加用户")
		return
	}

	// 替换中文冒号为英文冒号
	timeStr := strings.ReplaceAll(text, "：", ":")

	// 验证时间格式
	parts := strings.Split(timeStr, ":")
	if len(parts) < 2 {
		sendTelegramMessage(chatID, "时间格式错误，请使用 HH:MM:SS 格式:")
		return
	}

	// 验证小时
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		sendTelegramMessage(chatID, "小时格式错误，应为 00-23:")
		return
	}

	// 验证分钟
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		sendTelegramMessage(chatID, "分钟格式错误，应为 00-59:")
		return
	}

	// 验证秒（如果有）
	if len(parts) == 3 {
		second, err := strconv.Atoi(parts[2])
		if err != nil || second < 0 || second > 59 {
			sendTelegramMessage(chatID, "秒格式错误，应为 00-59:")
			return
		}
	}

	// 保存时间并进入下一步
	data["time"] = timeStr

	stateMutex.Lock()
	userStates[chatID] = UserState{
		Type:       StateWaitRemark,
		Data:       data,
		CreateTime: time.Now(),
	}
	stateMutex.Unlock()

	// 发送提示消息
	sendTelegramMessage(chatID, "请输入备注信息(如不需要备注，输入0):\n输入0取消")
}

// 处理等待备注状态
func handleWaitRemark(chatID int64, text string, data map[string]string) {
	// 检查是否输入0退出
	if text == "0" {
		// 清除用户状态
		stateMutex.Lock()
		delete(userStates, chatID)
		stateMutex.Unlock()
		sendTelegramMessage(chatID, "已取消添加用户")
		return
	}

	// 处理备注
	remark := text

	// 保存备注
	data["remark"] = remark

	// 提取数据
	token := data["token"]
	timeStr := data["time"]
	random := data["random"] == "true"

	// 处理random参数
	randomParam := ""
	if random {
		randomParam = "random"
	}

	// 调用添加用户函数
	handleAddCommand(chatID, fmt.Sprintf("/add %s %s %s %s", token, timeStr, randomParam, remark))

	// 清除用户状态
	stateMutex.Lock()
	delete(userStates, chatID)
	stateMutex.Unlock()
}

// 处理等待删除方式状态
func handleWaitRemoveOpt(chatID int64, text string) {
	// 检查是否输入0退出
	if text == "0" {
		// 清除用户状态
		stateMutex.Lock()
		delete(userStates, chatID)
		stateMutex.Unlock()
		sendTelegramMessage(chatID, "已取消删除用户")
		return
	}

	// 检查输入是否有效
	if text != "1" && text != "2" {
		sendTelegramMessage(chatID, "请输入正确的选项(1或2):")
		return
	}

	// 保存删除方式并进入下一步
	stateMutex.Lock()
	userStates[chatID] = UserState{
		Type: StateWaitRemoveUser,
		Data: map[string]string{
			"removeOpt": text,
		},
		CreateTime: time.Now(),
	}
	stateMutex.Unlock()

	// 发送提示消息
	if text == "1" {
		sendTelegramMessage(chatID, "请输入要删除的用户Token:\n输入0取消")
	} else {
		sendTelegramMessage(chatID, "请输入要删除的用户名:\n输入0取消")
	}
}

// 处理等待删除用户状态
func handleWaitRemoveUser(chatID int64, text string, data map[string]string) {
	// 检查是否输入0退出
	if text == "0" {
		// 清除用户状态
		stateMutex.Lock()
		delete(userStates, chatID)
		stateMutex.Unlock()
		sendTelegramMessage(chatID, "已取消删除用户")
		return
	}

	// 提取删除方式
	removeOpt := data["removeOpt"]

	// 调用删除用户函数
	if removeOpt == "1" {
		// 根据Token删除
		handleRemoveCommand(chatID, fmt.Sprintf("/remove %s", text))
	} else {
		// 根据用户名删除
		handleRemoveByUsername(chatID, text)
	}

	// 清除用户状态
	stateMutex.Lock()
	delete(userStates, chatID)
	stateMutex.Unlock()
}

// 根据用户名删除用户
func handleRemoveByUsername(chatID int64, username string) {
	usersMutex.Lock()
	defer usersMutex.Unlock()

	// 查找并删除用户
	found := false
	for i, user := range users {
		if user.Username == username {
			// 删除用户
			users = append(users[:i], users[i+1:]...)
			found = true
			printlnUTF8(fmt.Sprintf("删除用户: %s", username))
			break
		}
	}

	if found {
		// 保存数据
		saveData()
		sendTelegramMessage(chatID, "用户删除成功!")
	} else {
		sendTelegramMessage(chatID, "用户不存在!")
	}
}

// 处理添加用户命令
func handleAddCommand(chatID int64, text string) {
	// 调试日志：打印完整的文本
	printlnUTF8("=== handleAddCommand called ===")
	printlnUTF8(fmt.Sprintf("Full text: '%s'", text))
	printlnUTF8(fmt.Sprintf("Text length: %d", len(text)))
	
	parts := strings.Split(text, " ")
	
	// 调试日志：打印分割后的部分
	printlnUTF8(fmt.Sprintf("Split parts: %v", parts))
	printlnUTF8(fmt.Sprintf("Parts count: %d", len(parts)))
	
	// 过滤空字符串元素
	var filteredParts []string
	for i, part := range parts {
		printlnUTF8(fmt.Sprintf("Part %d: '%s' (empty: %t)", i, part, part == ""))
		if part != "" {
			filteredParts = append(filteredParts, part)
		}
	}
	
	// 调试日志：打印过滤后的部分
	printlnUTF8(fmt.Sprintf("Filtered parts: %v", filteredParts))
	printlnUTF8(fmt.Sprintf("Filtered parts count: %d", len(filteredParts)))
	
	if len(filteredParts) < 3 {
		sendTelegramMessage(chatID, "格式错误，请使用: /add token time [random] [remark]")
		return
	}

	token := filteredParts[1]
	timeStr := filteredParts[2]
	// 替换中文冒号为英文冒号
	timeStr = strings.ReplaceAll(timeStr, "：", ":")
	random := false
	remark := ""
	
	// 检查是否有random参数
	if len(filteredParts) > 3 {
		if filteredParts[3] == "random" {
			random = true
			// 检查是否有备注参数
			if len(filteredParts) > 4 {
				// 合并剩余的所有部分作为备注
				remark = strings.Join(filteredParts[4:], " ")
			}
		} else {
			// 剩余的所有部分作为备注
			remark = strings.Join(filteredParts[3:], " ")
		}
	}
	
	// 调试日志：打印解析后的参数
	printlnUTF8(fmt.Sprintf("Parsed token: '%s'", truncateToken(token)))
	printlnUTF8(fmt.Sprintf("Parsed time: '%s'", timeStr))
	printlnUTF8(fmt.Sprintf("Parsed random: %v", random))
	printlnUTF8(fmt.Sprintf("Parsed remark: '%s'", remark))

	usersMutex.Lock()
	defer usersMutex.Unlock()

	// 生成随机时间（如果是随机签到）
	randomTime := ""
	if random {
		// 生成当前时间之后的随机时间
		now := time.Now()
		hour := now.Hour()
		minute := now.Minute()
		
		// 生成当前小时或之后的小时
		hourRange := 23 - hour
		if hourRange > 0 {
			randomHour := hour + rand.Intn(hourRange + 1)
			// 如果是当前小时，生成当前分钟之后的分钟
			if randomHour == hour {
				minuteRange := 59 - minute
				if minuteRange > 0 {
					randomMinute := minute + 1 + rand.Intn(minuteRange)
					randomSecond := rand.Intn(60)
					randomTime = fmt.Sprintf("%02d:%02d:%02d", randomHour, randomMinute, randomSecond)
				} else {
					// 当前时间是23:59，只能选择23:59:59
					randomTime = "23:59:59"
				}
			} else {
				// 其他小时，生成任意分钟和秒
				randomMinute := rand.Intn(60)
				randomSecond := rand.Intn(60)
				randomTime = fmt.Sprintf("%02d:%02d:%02d", randomHour, randomMinute, randomSecond)
			}
		} else {
			// 当前时间是23点，只能选择23点
			minuteRange := 59 - minute
			if minuteRange > 0 {
				randomMinute := minute + 1 + rand.Intn(minuteRange)
				randomSecond := rand.Intn(60)
				randomTime = fmt.Sprintf("23:%02d:%02d", randomMinute, randomSecond)
			} else {
				// 当前时间是23:59，只能选择23:59:59
				randomTime = "23:59:59"
			}
		}
		printlnUTF8(fmt.Sprintf("生成随机时间: %s", randomTime))
	}
	
	// 获取用户信息（优先使用状态中保存的username）
	username := ""
	// 检查是否在状态中保存了username
	stateMutex.Lock()
	if state, exists := userStates[chatID]; exists && state.Data != nil {
		if savedUsername, ok := state.Data["username"]; ok {
			username = savedUsername
			printlnUTF8(fmt.Sprintf("使用状态中保存的用户名: %s", username))
		}
	}
	stateMutex.Unlock()
	
	// 如果没有保存的username，尝试获取
	if username == "" {
		userInfo, err := getUserInfo(token)
		if err != nil {
			printlnUTF8(fmt.Sprintf("获取用户信息失败: %v，使用空用户名", err))
		} else {
			username = userInfo.Username
			printlnUTF8(fmt.Sprintf("获取用户信息成功: %s", username))
		}
	}

	// 检查是否已存在
	found := false
	for i, user := range users {
		if user.Token == token {
			// 更新现有用户
			users[i] = User{
				Token:      token,
				Time:       timeStr,
				Random:     random,
				RandomTime: randomTime,
				Remark:     remark,
				Username:   username,
			}
			found = true
			printlnUTF8(fmt.Sprintf("更新用户: %s, 用户名: %s, 备注: %s", truncateToken(token), username, remark))
			break
		}
	}

	if !found {
		// 添加新用户
		users = append(users, User{
			Token:      token,
			Time:       timeStr,
			Random:     random,
			RandomTime: randomTime,
			Remark:     remark,
			Username:   username,
		})
		printlnUTF8(fmt.Sprintf("添加新用户: %s, 用户名: %s, 备注: %s", truncateToken(token), username, remark))
	}
	
	// 保存数据
	saveData()
	
	// 发送成功消息
	sendTelegramMessage(chatID, "用户添加成功！")
	
	// 立即检查新添加的用户是否需要签到
	go checkinUsers()
}

// 处理删除用户命令
func handleRemoveCommand(chatID int64, text string) {
	parts := strings.Split(text, " ")
	if len(parts) != 2 {
		sendTelegramMessage(chatID, "格式错误，请使用: /remove token")
		return
	}

	token := parts[1]

	usersMutex.Lock()
	defer usersMutex.Unlock()

	// 查找并删除用户
	found := false
	for i, user := range users {
		if user.Token == token {
			// 删除用户
			users = append(users[:i], users[i+1:]...)
			found = true
			printlnUTF8(fmt.Sprintf("删除用户: %s", token))
			break
		}
	}

	if found {
		// 保存数据
		saveData()
		sendTelegramMessage(chatID, "用户删除成功!")
	} else {
		sendTelegramMessage(chatID, "用户不存在!")
	}
}

// 处理列出用户命令
func handleListCommand(chatID int64) {
	usersMutex.Lock()
	defer usersMutex.Unlock()

	if len(users) == 0 {
		sendTelegramMessage(chatID, "当前没有签到用户!")
		return
	}

	message := "当前签到用户列表:\n"
	for i, user := range users {
		message += fmt.Sprintf("%d. Token: %s\n", i+1, truncateToken(user.Token))
		if user.Username != "" {
			// 确保用户名中不包含Markdown特殊字符
			safeUsername := strings.ReplaceAll(user.Username, "*", "")
			message += fmt.Sprintf("   用户名: *%s*\n", safeUsername)
		}
		message += fmt.Sprintf("   时间: %s\n", func() string {
			if user.Random {
				return "随机"
			}
			return user.Time
		}())
		if user.Random {
			if user.RandomTime != "" {
				message += fmt.Sprintf("   今日随机时间: %s\n", user.RandomTime)
			} else {
				message += "   今日随机时间: 未生成\n"
			}
		}
		if user.Remark != "" {
			// 确保备注中不包含Markdown特殊字符
			safeRemark := strings.ReplaceAll(user.Remark, "*", "")
			message += fmt.Sprintf("   备注: %s\n", safeRemark)
		}
	}

	sendTelegramMessage(chatID, message)
}

// 处理帮助命令
func handleHelpCommand(chatID int64) {
	message := "使用说明:\n\n"
	message += "/add token time [random] [remark] - 添加签到用户\n"
	message += "  token: 签到用的Bearer Token\n"
	message += "  time: 签到时间，格式 HH:MM\n"
	message += "  random: 可选，随机时间签到\n"
	message += "  remark: 可选，用户备注信息\n\n"
	message += "/remove token - 删除签到用户\n"
	message += "/list - 查看当前签到用户列表\n"
	message += "/help - 显示帮助信息"

	sendTelegramMessage(chatID, message)
}

// 截断token
func truncateToken(token string) string {
	// 只显示前面的数字加_后面的两个字符串，其他以*******显示
	parts := strings.Split(token, "_")
	if len(parts) >= 2 {
		prefix := parts[0]
		suffix := parts[1]
		if len(suffix) >= 2 {
			return prefix + "_" + suffix[:2] + "*******"
		}
	}
	// 如果格式不符合预期，返回前20个字符
	if len(token) > 20 {
		return token[:20]
	}
	return token
}

// 发送消息到Telegram
func sendTelegramMessage(chatID int64, text string) {
	printlnUTF8(fmt.Sprintf("发送消息到Telegram: %d, %s", chatID, text))

	payload := map[string]interface{}{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		printlnUTF8(fmt.Sprintf("JSON编码失败: %v", err))
		return
	}

	apiURL := fmt.Sprintf("%s%s/sendMessage", config.TelegramApiURL, config.TelegramBotToken)
	printlnUTF8(fmt.Sprintf("发送消息URL: %s", apiURL))
	printlnUTF8(fmt.Sprintf("发送消息数据: %s", string(data)))
	
	// 配置代理
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: http.DefaultTransport,
	}
	
	resp, err := client.Post(apiURL, "application/json", strings.NewReader(string(data)))
	if err != nil {
		printlnUTF8(fmt.Sprintf("发送消息失败: %v", err))
		return
	}
	defer resp.Body.Close()

	// 读取响应体
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		printlnUTF8(fmt.Sprintf("读取响应失败: %v", err))
		return
	}
	
	printlnUTF8(fmt.Sprintf("发送消息响应状态码: %d", resp.StatusCode))
	printlnUTF8(fmt.Sprintf("发送消息响应体: %s", string(responseBody)))

	if resp.StatusCode != http.StatusOK {
		printlnUTF8(fmt.Sprintf("发送消息失败，状态码: %d", resp.StatusCode))
	} else {
		printlnUTF8("发送消息成功")
	}
}

// 签到调度器
func checkinScheduler() {
	// 启动时立即执行一次
	checkinUsers()
	
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		<-ticker.C
		checkinUsers()
	}
}

// 检查用户签到
func checkinUsers() {
	usersMutex.Lock()
	// 检查是否是新的一天，如果是，重置所有用户的随机时间
	now := time.Now().In(beijingLoc)
	currentHour := now.Hour()
	currentMinute := now.Minute()
	currentSecond := now.Second()
	
	// 如果是每天的00:00:00，重置所有用户的随机时间
	if currentHour == 0 && currentMinute == 0 && currentSecond == 0 {
		for i, user := range users {
			if user.Random {
				users[i].RandomTime = ""
			}
		}
		saveData()
		printlnUTF8("=== 新的一天开始，重置所有用户的随机时间 ===")
	}
	
	userCopy := make([]User, len(users))
	copy(userCopy, users)
	usersMutex.Unlock()

	// 调试日志：打印当前时间
	printlnUTF8("=== 签到调度器运行 ===")
	printlnUTF8(fmt.Sprintf("当前时间(北京时间): %02d:%02d:%02d", currentHour, currentMinute, currentSecond))
	printlnUTF8(fmt.Sprintf("用户数量: %d", len(userCopy)))

	for i, user := range userCopy {
		// 调试日志：打印用户信息
		printlnUTF8(fmt.Sprintf("用户 %d: Token=%s, Time=%s, Random=%v", i+1, truncateToken(user.Token), user.Time, user.Random))
		
		if user.Random {
			// 随机签到，每天随机选择一个时间
			// 检查是否已经生成今天的随机时间
			randomHour := 0
			randomMinute := 0
			randomSecond := 0
			needUpdate := false
			
			if user.RandomTime == "" {
				// 生成今天的随机时间，确保是当前时间之后的时间
				// 使用外部的currentHour和currentMinute，确保时间一致性
				hourRange := 23 - currentHour
				if hourRange > 0 {
					randomHour = currentHour + rand.Intn(hourRange + 1)
				} else {
					// 当前时间是23点，只能选择23点
					randomHour = 23
				}
				
				// 如果是当前小时，生成当前分钟之后的分钟
				if randomHour == currentHour {
					minuteRange := 59 - currentMinute
					if minuteRange > 0 {
						randomMinute = currentMinute + 1 + rand.Intn(minuteRange)
					} else {
						// 当前时间是23:59，只能选择23:59
						randomMinute = 59
					}
				} else {
					// 其他小时，生成任意分钟
					randomMinute = rand.Intn(60)
				}
				
				// 生成任意秒数
				randomSecond = rand.Intn(60)
				
				user.RandomTime = fmt.Sprintf("%02d:%02d:%02d", randomHour, randomMinute, randomSecond)
				needUpdate = true
				printlnUTF8(fmt.Sprintf("生成随机时间: %s", user.RandomTime))
			} else {
				// 使用已生成的随机时间
				parts := strings.Split(user.RandomTime, ":")
				if len(parts) >= 2 {
					randomHour, _ = strconv.Atoi(parts[0])
					randomMinute, _ = strconv.Atoi(parts[1])
					if len(parts) == 3 {
						randomSecond, _ = strconv.Atoi(parts[2])
					} else {
						randomSecond = 0
					}
				}
			}
			
			printlnUTF8(fmt.Sprintf("随机签到: user=%s, 随机时间=%s, 当前时间=%02d:%02d:%02d", truncateToken(user.Token), user.RandomTime, currentHour, currentMinute, currentSecond))
			
			if randomHour == currentHour && randomMinute == currentMinute && randomSecond == currentSecond {
				printlnUTF8(fmt.Sprintf("开始随机签到用户: %s", truncateToken(user.Token)))
				go performCheckin(user.Token)
				// 签到后清空随机时间，并且当天不再重新生成
				user.RandomTime = "checked"
				needUpdate = true
			}
			
			// 跳过已经签到的用户，不再生成随机时间
			if user.RandomTime == "checked" {
				continue
			}
			
			// 如果需要更新用户信息，保存到数据中
			if needUpdate {
				usersMutex.Lock()
				for j, u := range users {
					if u.Token == user.Token {
						users[j] = user
						break
					}
				}
				usersMutex.Unlock()
				saveData()
			}
		} else {
			// 固定时间签到
			parts := strings.Split(user.Time, ":")
			printlnUTF8(fmt.Sprintf("固定时间签到: user=%s, time=%s, split parts=%v", truncateToken(user.Token), user.Time, parts))
			if len(parts) >= 2 {
				hour, err := strconv.Atoi(parts[0])
				if err != nil {
					printlnUTF8(fmt.Sprintf("用户 %s 的小时格式无效: %v", truncateToken(user.Token), err))
					continue
				}
				minute, err := strconv.Atoi(parts[1])
				if err != nil {
					printlnUTF8(fmt.Sprintf("用户 %s 的分钟格式无效: %v", truncateToken(user.Token), err))
					continue
				}
				second := 0
				if len(parts) == 3 {
					second, _ = strconv.Atoi(parts[2])
				}
				printlnUTF8(fmt.Sprintf("检查用户 %s: 计划时间=%02d:%02d:%02d, 当前时间=%02d:%02d:%02d", truncateToken(user.Token), hour, minute, second, currentHour, currentMinute, currentSecond))
				if hour == currentHour && minute == currentMinute && second == currentSecond {
					printlnUTF8(fmt.Sprintf("开始签到用户: %s", truncateToken(user.Token)))
					go performCheckin(user.Token)
				}
			} else {
				printlnUTF8(fmt.Sprintf("用户 %s 的时间格式无效: %s", truncateToken(user.Token), user.Time))
			}
		}
	}
}

// 执行签到
func performCheckin(token string) {
	// 获取用户信息
	userInfo, err := getUserInfo(token)
	if err != nil {
		printlnUTF8(fmt.Sprintf("获取用户信息失败: %v", err))
		return
	}

	// 执行签到
	result, err, statusText := checkin(token)
	if err != nil {
		printlnUTF8(fmt.Sprintf("签到失败: %v", err))
		// 发送失败通知
		message := fmt.Sprintf("📅 签到通知\n\n用户名: %s\n签到状态: %s\n错误信息: %v", userInfo.Username, statusText, err)
		for chatID := range chatIds {
			sendTelegramMessage(chatID, message)
		}
		return
	}

	// 发送通知
	if statusText == "签到成功" {
		sendCheckinNotification(userInfo.Username, result, statusText)
	} else {
		message := fmt.Sprintf("📅 签到通知\n\n用户名: %s\n签到状态: %s", userInfo.Username, statusText)
		for chatID := range chatIds {
			sendTelegramMessage(chatID, message)
		}
	}
	printlnUTF8(fmt.Sprintf("签到结果: %s - %s", userInfo.Username, statusText))
}

// 获取用户信息
func getUserInfo(token string) (UserInfo, error) {
	var userInfo UserInfo

	url := fmt.Sprintf("%s/api/user", config.ApiBaseURL)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return userInfo, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return userInfo, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return userInfo, fmt.Errorf("获取用户信息失败，状态码: %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return userInfo, err
	}

	return userInfo, nil
}

// 执行签到
func checkin(token string) (CheckinResult, error, string) {
	var result CheckinResult
	var statusText string

	// 准备多种签到内容，随机选择一个
	checkinContents := []string{
		"1",
		"2",
		"3",
		"4",
		"5",
		"6",
		"7",
		"8",
	}
	
	// 随机选择一个签到内容
	randomIndex := rand.Intn(len(checkinContents))
	content := checkinContents[randomIndex]
	
	// 将content作为查询参数添加到URL中
	url := fmt.Sprintf("%s/api/user/sign?content=%s", config.ApiBaseURL, url.QueryEscape(content))
	
	// 打印请求信息
	printlnUTF8("=== 签到请求 ===")
	printlnUTF8(fmt.Sprintf("URL: %s", url))
	printlnUTF8("Method: PUT")
	printlnUTF8(fmt.Sprintf("Token: %s", token))
	
	// 使用PUT方法，请求体为nil
	req, err := http.NewRequest(http.MethodPut, url, nil)
	if err != nil {
		return result, err, "网络请求失败"
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	// 添加User-Agent头
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return result, err, "网络连接失败"
	}
	defer resp.Body.Close()

	// 打印响应状态和头信息
	printlnUTF8("=== 签到响应 ===")
	printlnUTF8(fmt.Sprintf("响应状态: %d", resp.StatusCode))
	printlnUTF8(fmt.Sprintf("响应头: %v", resp.Header))

	// 读取响应体
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return result, err, "读取响应失败"
	}
	
	// 打印响应体
	printlnUTF8(fmt.Sprintf("响应体: %s", string(responseBody)))

	// 检查响应状态码
	if resp.StatusCode == http.StatusOK {
		// 解析响应体
		if err := json.Unmarshal(responseBody, &result); err != nil {
			return result, err, "解析响应失败"
		}
		statusText = "签到成功"
	} else if resp.StatusCode == http.StatusConflict {
		// 409 Conflict - 可能表示已经签到过
		statusText = "今日已签到"
	} else if resp.StatusCode == 429 {
		// 429 Too Many Requests - 可能表示重复签到
		statusText = "今日已签到"
	} else if resp.StatusCode == http.StatusBadRequest {
		// 400 Bad Request - 可能表示参数错误
		statusText = "参数错误"
	} else if resp.StatusCode == http.StatusUnauthorized {
		// 401 Unauthorized - 可能表示token无效
		statusText = "Token无效"
	} else {
		// 其他错误
		statusText = fmt.Sprintf("签到失败，状态码: %d", resp.StatusCode)
	}

	return result, nil, statusText
}

// 发送签到通知
func sendCheckinNotification(username string, result CheckinResult, statusText string) {
	message := fmt.Sprintf("📅 签到通知\n\n用户名: %s\n签到状态: %s\n累计签到: %d 天\n获得萝卜: %d 个\n今日签到排名: %d", 
		username, statusText, result.ContinuousDays, result.EarnPoint, result.SignIndex)
	for chatID := range chatIds {
		sendTelegramMessage(chatID, message)
	}
}

// 状态调度器
func statusScheduler() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C
		usersMutex.Lock()
		userCount := len(users)
		usersMutex.Unlock()
		printlnUTF8(fmt.Sprintf("[%s] 系统运行中 - 当前用户: %d, 当前chat_ids: %d", time.Now().In(beijingLoc).Format("15:04:05"), userCount, len(chatIds)))
	}
}

// Telegram消息轮询
func telegramPolling() {
	printlnUTF8("开始Telegram轮询...")
	
	// 存储最后处理的消息ID
	var offset int64 = 0
	
	for {
		// 捕获循环中的panic
		defer func() {
			if r := recover(); r != nil {
				printlnUTF8(fmt.Sprintf("Telegram轮询循环崩溃: %v", r))
			}
		}()
		
		// 调用getUpdates API
		apiURL := fmt.Sprintf("%s%s/getUpdates?offset=%d&timeout=30", config.TelegramApiURL, config.TelegramBotToken, offset)
		printlnUTF8(fmt.Sprintf("获取Telegram更新: %s", apiURL))
		
		// 创建一个带超时的上下文
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		
		// 创建请求
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			printlnUTF8(fmt.Sprintf("创建请求失败: %v", err))
			cancel()
			time.Sleep(5 * time.Second)
			continue
		}
		
		// 发送请求
		printlnUTF8("发送Telegram请求...")
		// 使用与setupNetwork函数中相同的代理设置
		client := &http.Client{
			Timeout: 30 * time.Second,
		}
		resp, err := client.Do(req)
		if err != nil {
			printlnUTF8(fmt.Sprintf("获取更新失败: %v", err))
			cancel()
			time.Sleep(5 * time.Second)
			continue
		}
		
		// 确保响应体被关闭
		defer resp.Body.Close()
		defer cancel()
		
		printlnUTF8(fmt.Sprintf("获取更新成功，状态码: %d", resp.StatusCode))
		
		// 读取响应体
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			printlnUTF8(fmt.Sprintf("读取响应体失败: %v", err))
			time.Sleep(5 * time.Second)
			continue
		}
		
		// 打印响应体
		printlnUTF8(fmt.Sprintf("响应体: %s", string(responseBody)))
		
		// 解析响应
		var result struct {
			Ok     bool `json:"ok"`
			Result []struct {
				UpdateID int64 `json:"update_id"`
				Message  struct {
					Chat struct {
						ID int64 `json:"id"`
					} `json:"chat"`
					Text string `json:"text"`
				} `json:"message"`
			} `json:"result"`
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		
		if err := json.Unmarshal(responseBody, &result); err != nil {
			printlnUTF8(fmt.Sprintf("解析更新失败: %v", err))
			time.Sleep(5 * time.Second)
			continue
		}
		
		// 检查是否有错误
		if !result.Ok {
			printlnUTF8(fmt.Sprintf("Telegram API错误: %d - %s", result.Error.Code, result.Error.Message))
			time.Sleep(5 * time.Second)
			continue
		}
		
		// 打印响应
		printlnUTF8(fmt.Sprintf("获取到 %d 条更新", len(result.Result)))
		
		// 处理消息
		for _, update := range result.Result {
			// 更新offset
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			
			// 处理消息
			if update.Message.Text != "" {
				chatID := update.Message.Chat.ID
				text := update.Message.Text
				
				// 存储chat_id
				chatIds[chatID] = true
				printlnUTF8(fmt.Sprintf("存储chat_id: %d", chatID))
				
				// 检查用户状态
				stateMutex.Lock()
				state, exists := userStates[chatID]
				stateMutex.Unlock()
				
				// 如果用户有状态，处理状态相关的输入
				if exists {
					handleUserState(chatID, text, state)
					continue
				}
				
				// 处理命令
				trimmedText := strings.TrimSpace(text)
				// 调试日志：打印处理命令前的信息
				printlnUTF8("=== 处理命令 ===")
				printlnUTF8(fmt.Sprintf("原始文本: '%s'", text))
				printlnUTF8(fmt.Sprintf("去除空格后: '%s'", trimmedText))
				
				// 处理 /add 命令
				if strings.HasPrefix(trimmedText, "/add") {
					// 检查是否是单独的 /add 命令
					if trimmedText == "/add" {
						// 开始一步一步添加用户
						printlnUTF8("调用 startAddUser")
						startAddUser(chatID)
					} else {
						// 兼容旧格式
						printlnUTF8("调用 handleAddCommand (旧格式)")
						handleAddCommand(chatID, text)
					}
				} else if strings.HasPrefix(trimmedText, "/remove") {
					// 检查是否是单独的 /remove 命令
					if trimmedText == "/remove" {
						// 开始一步一步删除用户
						startRemoveUser(chatID)
					} else {
						// 兼容旧格式
						handleRemoveCommand(chatID, text)
					}
				} else if trimmedText == "/list" {
					printlnUTF8("调用 handleListCommand")
					handleListCommand(chatID)
				} else if trimmedText == "/help" {
					handleHelpCommand(chatID)
				} else {
					sendTelegramMessage(chatID, "请使用 /help 查看可用命令")
				}
			}
		}
		
		// 短暂休眠，避免请求过于频繁
		time.Sleep(1 * time.Second)
	}
}
