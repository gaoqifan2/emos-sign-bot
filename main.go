package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
)

func main() {
	// 初始化随机种子
	rand.Seed(time.Now().UnixNano())
	
	// 初始化北京时间时区
	var err error
	beijingLoc, err = time.LoadLocation("Asia/Shanghai")
	if err != nil {
		fmt.Printf("加载时区失败: %v，使用本地时区\n", err)
		beijingLoc = time.Local
	} else {
		fmt.Println("成功加载北京时间时区 (Asia/Shanghai)")
	}
	
	// 初始化配置
	initConfig()
	
	// 网络设置：强制使用IPv4并支持代理
	setupNetwork()
	
	// 加载数据
	loadData()

	// 打印系统信息
	fmt.Println("=== 自动签到系统 (Go版本) ===")
	fmt.Println("启动时间:", time.Now().In(beijingLoc).Format(time.RFC3339))
	fmt.Println("服务器运行在:", fmt.Sprintf("http://localhost:%d", config.Port))
	fmt.Println("API接口:")
	fmt.Println("  POST /api/register - 注册用户 (token, time, random)")
	fmt.Println("  POST /api/remove - 删除用户 (token)")
	fmt.Println("  GET /api/users - 获取用户列表")
	fmt.Println("  POST /webhook - Telegram Bot webhook")
	fmt.Println("系统启动成功，等待命令...")
	fmt.Printf("已加载 %d 个用户，%d 个聊天ID\n", len(users), len(chatIds))

	// 设置路由
	http.HandleFunc("/api/register", registerUser)
	http.HandleFunc("/api/remove", removeUser)
	http.HandleFunc("/api/users", getUsers)
	http.HandleFunc("/webhook", handleWebhook)

	// 启动服务器（可选，用于API访问）
	go func() {
		if err := http.ListenAndServe(fmt.Sprintf(":%d", config.Port), nil); err != nil {
			log.Printf("服务器启动失败: %v", err)
		}
	}()

	// 启动轮询获取Telegram消息（如果启用）
	if config.EnableTelegram {
		go telegramPolling()
		fmt.Println("Telegram轮询已启用")
	} else {
		fmt.Println("Telegram轮询已禁用")
	}

	// 定期检查签到
	go checkinScheduler()

	// 定期输出系统状态
	go statusScheduler()

	// 保持程序运行
	select {}
}

// 初始化配置
func initConfig() {
	config = Config{
		Port:             3000,
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
			fmt.Println("数据文件不存在，使用空数据")
			return
		}
		fmt.Printf("读取数据文件失败: %v，使用空数据\n", err)
		return
	}
	
	// 解析数据
	var storage DataStorage
	if err := json.Unmarshal(data, &storage); err != nil {
		fmt.Printf("解析数据文件失败: %v，使用空数据\n", err)
		return
	}
	
	// 加载数据
	users = storage.Users
	if storage.ChatIds != nil {
		chatIds = storage.ChatIds
	}
	fmt.Printf("成功加载数据: %d 个用户，%d 个聊天ID\n", len(users), len(chatIds))
}

// 保存数据
func saveData() {
	fmt.Printf("开始保存数据: %d 个用户，%d 个聊天ID\n", len(users), len(chatIds))
	
	// 准备数据
	storage := DataStorage{
		Users:   users,
		ChatIds: chatIds,
	}
	
	// 序列化数据
	data, err := json.MarshalIndent(storage, "", "  ")
	if err != nil {
		fmt.Printf("序列化数据失败: %v\n", err)
		return
	}
	
	fmt.Printf("序列化成功，数据长度: %d 字节\n", len(data))
	
	// 写入文件
	if err := os.WriteFile(config.DataFile, data, 0644); err != nil {
		fmt.Printf("写入数据文件失败: %v\n", err)
		return
	}
	
	fmt.Printf("成功保存数据到 %s: %d 个用户，%d 个聊天ID\n", config.DataFile, len(users), len(chatIds))
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
			fmt.Printf("使用配置的代理: %s\n", config.ProxyURL)
		} else {
			fmt.Printf("无效的代理URL，使用系统代理: %v\n", err)
			proxyFunc = http.ProxyFromEnvironment
		}
	} else {
		fmt.Println("使用系统环境变量中的代理")
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
	
	fmt.Println("网络设置完成: 使用IPv4和代理支持")
}

// 测试代理连接
func testProxyConnection() {
	fmt.Println("测试代理连接...")
	
	// 创建一个带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// 测试连接到Telegram API
	url := "https://api.telegram.org/bot" + config.TelegramBotToken + "/getMe"
	
	// 创建请求
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		fmt.Printf("代理测试失败: %v\n", err)
		fmt.Println("请检查您的代理设置")
		fmt.Println("系统将继续运行，但Telegram功能可能受限")
		return
	}
	
	// 使用配置的Transport
	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: http.DefaultTransport,
	}
	
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("代理测试失败: %v\n", err)
		fmt.Println("请检查您的代理设置")
		fmt.Println("系统将继续运行，但Telegram功能可能受限")
	} else {
		defer resp.Body.Close()
		fmt.Printf("代理测试成功! 状态码: %d\n", resp.StatusCode)
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
			fmt.Printf("更新用户: %s\n", req.Token)
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
		fmt.Printf("添加新用户: %s\n", req.Token)
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
			fmt.Printf("删除用户: %s\n", req.Token)
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

	fmt.Println("获取用户列表请求")

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
		fmt.Printf("存储chat_id: %d\n", chatID)

		// 处理命令
		if strings.HasPrefix(text, "/add") {
			handleAddCommand(chatID, text)
		} else if strings.HasPrefix(text, "/remove") {
			handleRemoveCommand(chatID, text)
		} else if text == "/list" {
			handleListCommand(chatID)
		} else if text == "/help" {
			handleHelpCommand(chatID)
		} else {
			sendTelegramMessage(chatID, "请使用 /help 查看可用命令")
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// 处理添加用户命令
func handleAddCommand(chatID int64, text string) {
	// 调试日志：打印完整的文本
	fmt.Printf("=== handleAddCommand called ===\n")
	fmt.Printf("Full text: '%s'\n", text)
	fmt.Printf("Text length: %d\n", len(text))
	
	parts := strings.Split(text, " ")
	
	// 调试日志：打印分割后的部分
	fmt.Printf("Split parts: %v\n", parts)
	fmt.Printf("Parts count: %d\n", len(parts))
	
	// 过滤空字符串元素
	var filteredParts []string
	for i, part := range parts {
		fmt.Printf("Part %d: '%s' (empty: %t)\n", i, part, part == "")
		if part != "" {
			filteredParts = append(filteredParts, part)
		}
	}
	
	// 调试日志：打印过滤后的部分
	fmt.Printf("Filtered parts: %v\n", filteredParts)
	fmt.Printf("Filtered parts count: %d\n", len(filteredParts))
	
	if len(filteredParts) < 3 {
		sendTelegramMessage(chatID, "格式错误，请使用: /add token time [random] [remark]")
		return
	}

	token := filteredParts[1]
	timeStr := filteredParts[2]
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
	fmt.Printf("Parsed token: '%s'\n", token)
	fmt.Printf("Parsed time: '%s'\n", timeStr)
	fmt.Printf("Parsed random: %v\n", random)
	fmt.Printf("Parsed remark: '%s'\n", remark)

	usersMutex.Lock()
	defer usersMutex.Unlock()

	// 检查是否已存在
	found := false
	for i, user := range users {
		if user.Token == token {
			// 更新现有用户
			users[i] = User{
				Token:  token,
				Time:   timeStr,
				Random: random,
				Remark: remark,
			}
			found = true
			fmt.Printf("更新用户: %s, 备注: %s\n", token, remark)
			break
		}
	}

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
					randomTime = fmt.Sprintf("%02d:%02d", randomHour, randomMinute)
				} else {
					// 当前时间是23:59，只能选择23:59
					randomTime = "23:59"
				}
			} else {
				// 其他小时，生成任意分钟
				randomMinute := rand.Intn(60)
				randomTime = fmt.Sprintf("%02d:%02d", randomHour, randomMinute)
			}
		} else {
			// 当前时间是23点，只能选择23点
			minuteRange := 59 - minute
			if minuteRange > 0 {
				randomMinute := minute + 1 + rand.Intn(minuteRange)
				randomTime = fmt.Sprintf("23:%02d", randomMinute)
			} else {
				// 当前时间是23:59，只能选择23:59
				randomTime = "23:59"
			}
		}
		fmt.Printf("生成随机时间: %s\n", randomTime)
	}

	if !found {
		// 添加新用户
		users = append(users, User{
			Token:      token,
			Time:       timeStr,
			Random:     random,
			RandomTime: randomTime,
			Remark:     remark,
		})
		fmt.Printf("添加新用户: %s, 备注: %s\n", token, remark)
	} else {
		// 更新现有用户
		for i, user := range users {
			if user.Token == token {
				users[i] = User{
					Token:      token,
					Time:       timeStr,
					Random:     random,
					RandomTime: randomTime,
					Remark:     remark,
				}
				break
			}
		}
		fmt.Printf("更新用户: %s, 备注: %s\n", token, remark)
	}
	
	// 保存数据
	saveData()

	sendTelegramMessage(chatID, "用户添加成功!")
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
			fmt.Printf("删除用户: %s\n", token)
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
		message += fmt.Sprintf("%d. Token: %s...\n", i+1, truncateToken(user.Token))
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
			message += fmt.Sprintf("   备注: %s\n", user.Remark)
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
	fmt.Printf("发送消息到Telegram: %d, %s\n", chatID, text)

	payload := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("JSON编码失败: %v\n", err)
		return
	}

	url := fmt.Sprintf("%s%s/sendMessage", config.TelegramApiURL, config.TelegramBotToken)
	resp, err := http.Post(url, "application/json", strings.NewReader(string(data)))
	if err != nil {
		fmt.Printf("发送消息失败: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("发送消息失败，状态码: %d\n", resp.StatusCode)
	}
}

// 签到调度器
func checkinScheduler() {
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
	
	// 如果是每天的00:00，重置所有用户的随机时间
	if currentHour == 0 && currentMinute == 0 {
		for i, user := range users {
			if user.Random && user.RandomTime == "checked" {
				users[i].RandomTime = ""
			}
		}
		saveData()
		fmt.Println("=== 新的一天开始，重置所有用户的随机时间 ===")
	}
	
	userCopy := make([]User, len(users))
	copy(userCopy, users)
	usersMutex.Unlock()

	// 调试日志：打印当前时间
	fmt.Printf("=== 签到调度器运行 ===\n")
	fmt.Printf("当前时间(北京时间): %02d:%02d\n", currentHour, currentMinute)
	fmt.Printf("用户数量: %d\n", len(userCopy))

	for i, user := range userCopy {
		// 调试日志：打印用户信息
		fmt.Printf("用户 %d: Token=%s, Time=%s, Random=%v\n", i+1, user.Token, user.Time, user.Random)
		
		if user.Random {
			// 随机签到，每天随机选择一个时间
			// 检查是否已经生成今天的随机时间
			randomHour := 0
			randomMinute := 0
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
				
				user.RandomTime = fmt.Sprintf("%02d:%02d", randomHour, randomMinute)
				needUpdate = true
			} else {
				// 使用已生成的随机时间
				parts := strings.Split(user.RandomTime, ":")
				if len(parts) == 2 {
					randomHour, _ = strconv.Atoi(parts[0])
					randomMinute, _ = strconv.Atoi(parts[1])
				}
			}
			
			fmt.Printf("随机签到: user=%s, 随机时间=%s, 当前时间=%02d:%02d\n", user.Token, user.RandomTime, currentHour, currentMinute)
			
			if randomHour == currentHour && randomMinute == currentMinute {
				fmt.Printf("开始随机签到用户: %s\n", user.Token)
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
			fmt.Printf("固定时间签到: user=%s, time=%s, split parts=%v\n", user.Token, user.Time, parts)
			if len(parts) == 2 {
				hour, err := strconv.Atoi(parts[0])
				if err != nil {
					fmt.Printf("用户 %s 的小时格式无效: %v\n", user.Token, err)
					continue
				}
				minute, err := strconv.Atoi(parts[1])
				if err != nil {
					fmt.Printf("用户 %s 的分钟格式无效: %v\n", user.Token, err)
					continue
				}
				fmt.Printf("检查用户 %s: 计划时间=%02d:%02d, 当前时间=%02d:%02d\n", user.Token, hour, minute, currentHour, currentMinute)
				if hour == currentHour && minute == currentMinute {
					fmt.Printf("开始签到用户: %s\n", user.Token)
					go performCheckin(user.Token)
				}
			} else {
				fmt.Printf("用户 %s 的时间格式无效: %s\n", user.Token, user.Time)
			}
		}
	}
}

// 执行签到
func performCheckin(token string) {
	// 获取用户信息
	userInfo, err := getUserInfo(token)
	if err != nil {
		fmt.Printf("获取用户信息失败: %v\n", err)
		return
	}

	// 执行签到
	result, err, statusText := checkin(token)
	if err != nil {
		fmt.Printf("签到失败: %v\n", err)
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
	fmt.Printf("签到结果: %s - %s\n", userInfo.Username, statusText)
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
		"每日签到，希望获得更多萝卜！",
		"今天也要记得签到哦，萝卜在等我！",
		"签到打卡，快乐每一天！",
		"坚持签到，收获满满！",
		"又是充满希望的一天，签到啦！",
		"签到成功，开心每一天！",
		"每日一签，好运连连！",
		"签到打卡，记录生活点滴！",
	}
	
	// 随机选择一个签到内容
	randomIndex := rand.Intn(len(checkinContents))
	content := checkinContents[randomIndex]
	
	url := fmt.Sprintf("%s/api/user/sign", config.ApiBaseURL)
	
	// 准备请求体
	reqBody := map[string]string{
		"content": content,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return result, err, "JSON编码失败"
	}
	
	// 使用PUT方法
	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(string(body)))
	if err != nil {
		return result, err, "网络请求失败"
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return result, err, "网络连接失败"
	}
	defer resp.Body.Close()

	// 打印响应状态和头信息
	fmt.Printf("签到响应状态: %d\n", resp.StatusCode)
	fmt.Printf("签到响应头: %v\n", resp.Header)

	// 读取响应体
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return result, err, "读取响应失败"
	}

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
	message := fmt.Sprintf("📅 签到通知\n\n用户名: %s\n签到状态: %s\n连续签到: %d 天\n获得萝卜: %d 个\n今日签到排名: %d",
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
		fmt.Printf("[%s] 系统运行中 - 当前用户: %d, 当前chat_ids: %d\n", time.Now().In(beijingLoc).Format("15:04:05"), userCount, len(chatIds))
	}
}

// Telegram消息轮询
func telegramPolling() {
	fmt.Println("开始Telegram轮询...")
	
	// 存储最后处理的消息ID
	var offset int64 = 0
	
	for {
		// 调用getUpdates API
		url := fmt.Sprintf("%s%s/getUpdates?offset=%d&timeout=30", config.TelegramApiURL, config.TelegramBotToken, offset)
		resp, err := http.Get(url)
		if err != nil {
			fmt.Printf("获取更新失败: %v\n", err)
			time.Sleep(5 * time.Second)
			continue
		}
		
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
		}
		
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			fmt.Printf("解析更新失败: %v\n", err)
			resp.Body.Close()
			time.Sleep(5 * time.Second)
			continue
		}
		resp.Body.Close()
		
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
				fmt.Printf("存储chat_id: %d\n", chatID)
				
				// 处理命令
				if strings.HasPrefix(text, "/add") {
					handleAddCommand(chatID, text)
				} else if strings.HasPrefix(text, "/remove") {
					handleRemoveCommand(chatID, text)
				} else if text == "/list" {
					handleListCommand(chatID)
				} else if text == "/help" {
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
