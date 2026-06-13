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
	Port                int    `json:"port"`
	ApiBaseURL          string `json:"api_base_url"`
	PublicBaseURL       string `json:"public_base_url"`
	EmosProviderBot     string `json:"emos_provider_bot"`
	EmosUserID          string `json:"emos_user_id"`
	EmosServiceToken    string `json:"emos_service_token"`
	TelegramBotToken    string `json:"telegram_bot_token"`
	TelegramBotUsername string `json:"telegram_bot_username"`
	TelegramApiURL      string `json:"telegram_api_url"`
	ProxyURL            string `json:"proxy_url"`       // 代理URL
	EnableTelegram      bool   `json:"enable_telegram"` // 是否启用Telegram
	DataFile            string `json:"data_file"`       // 数据文件路径
}

// 用户信息
type User struct {
	Token         string `json:"token"`
	Time          string `json:"time"`
	Random        bool   `json:"random"`
	RandomTime    string `json:"random_time"` // 当天的随机签到时间
	Remark        string `json:"remark"`      // 用户备注
	Username      string `json:"username"`    // 用户名
	ChatID        int64  `json:"chat_id,omitempty"`
	LastCheckDate string `json:"last_check_date,omitempty"`
}

// 签到结果
type CheckinResult struct {
	SignIndex      int `json:"sign_index"`
	EarnPoint      int `json:"earn_point"`
	ContinuousDays int `json:"continuous_days"`
}

// 用户信息
type UserInfo struct {
	Username string `json:"username"`
}

type EMOSProfile struct {
	EMOSID   string
	Username string
	Avatar   string
}

type AuthUser struct {
	TelegramID        int64  `json:"telegram_id"`
	TelegramUsername  string `json:"telegram_username,omitempty"`
	TelegramFirstName string `json:"telegram_first_name,omitempty"`
	EMOSUsername      string `json:"emos_username,omitempty"`
	EMOSID            string `json:"emos_id,omitempty"`
	Avatar            string `json:"avatar,omitempty"`
	AuthToken         string `json:"auth_token,omitempty"`
	AuthStatus        string `json:"auth_status,omitempty"`
	UpdatedAt         string `json:"updated_at,omitempty"`
}

type TelegramProfile struct {
	ID        int64
	Username  string
	FirstName string
}

// 数据存储结构
type DataStorage struct {
	Users        []User             `json:"users"`
	ChatIds      map[int64]bool     `json:"chat_ids"`
	LoggedTokens map[int64]string   `json:"logged_tokens,omitempty"`
	AdminChatIds map[int64]bool     `json:"admin_chat_ids,omitempty"`
	AuthUsers    map[int64]AuthUser `json:"auth_users,omitempty"`
}

var (
	config                Config
	users                 []User
	chatIds               = make(map[int64]bool)
	loggedTokens          = make(map[int64]string)
	adminChatIds          = make(map[int64]bool)
	authUsers             = make(map[int64]AuthUser)
	usersMutex            sync.Mutex
	chatMutex             sync.Mutex
	beijingLoc            *time.Location // 北京时间时区
	telegramMessageSender = defaultSendTelegramMessage
	// 用户状态管理
	userStates = make(map[int64]UserState)
	stateMutex sync.Mutex
)

// 用户状态类型
type StateType string

const (
	StateIdle           StateType = "idle"
	StateWaitAddAccount StateType = "wait_add_account"
	StateWaitToken      StateType = "wait_token"
	StateWaitMode       StateType = "wait_mode"
	StateWaitTime       StateType = "wait_time"
	StateWaitRemark     StateType = "wait_remark"
	StateWaitRemoveOpt  StateType = "wait_remove_opt"
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
	logFile, err := os.OpenFile("log.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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
	http.HandleFunc("/api/telegram-login", completeTelegramLogin)
	http.HandleFunc("/login", serveTelegramLoginPage)
	http.HandleFunc("/bot-login", serveTelegramLoginPage)
	http.HandleFunc("/webhook", handleWebhook)

	go func() {
		addr := fmt.Sprintf(":%d", config.Port)
		printlnUTF8(fmt.Sprintf("HTTP服务启动: http://localhost%s", addr))
		if err := http.ListenAndServe(addr, nil); err != nil {
			printlnUTF8(fmt.Sprintf("HTTP服务启动失败: %v", err))
		}
	}()

	// 启动轮询获取Telegram消息（如果启用）
	if config.EnableTelegram {
		go setTelegramBotCommands()
		go func() {
			// 捕获panic
			defer func() {
				if r := recover(); r != nil {
					printlnUTF8(fmt.Sprintf("Telegram轮询线程崩溃: %v", r))
				}
			}()
			printlnUTF8("Telegram轮询线程已启动")
			for {
				printlnUTF8("启动Telegram轮询...")
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

	// Token更换提醒调度器 - 每隔30分钟发送提醒
	go func() {
		defer func() {
			if r := recover(); r != nil {
				printlnUTF8(fmt.Sprintf("Token提醒调度器崩溃: %v", r))
			}
		}()
		tokenReminderScheduler()
	}()

	// 保持程序运行
	select {}
}

// 初始化配置
func initConfig() {
	config = Config{
		Port:                8081,
		ApiBaseURL:          "https://emos.best",
		PublicBaseURL:       "http://127.0.0.1:8081",
		EmosProviderBot:     "emospg_bot",
		EmosUserID:          "e0E446ZE6s",
		EmosServiceToken:    "",
		TelegramBotToken:    "",
		TelegramBotUsername: "EmosCheckinBot",
		TelegramApiURL:      "https://api.telegram.org/bot",
		ProxyURL:            "",          // 代理URL，留空表示不使用代理
		EnableTelegram:      true,        // 是否启用Telegram
		DataFile:            "data.json", // 数据文件路径
	}
	if v := strings.TrimSpace(os.Getenv("BOT_USERNAME")); v != "" {
		config.TelegramBotUsername = v
	}
	if v := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")); v != "" {
		config.TelegramBotToken = v
	}
	if v := strings.TrimSpace(os.Getenv("EMOS_PROVIDER_BOT")); v != "" {
		config.EmosProviderBot = v
	}
	if v := strings.TrimSpace(os.Getenv("EMOS_USER_ID")); v != "" {
		config.EmosUserID = v
	}
	if v := strings.TrimSpace(os.Getenv("EMOS_API_BASE_URL")); v != "" {
		config.ApiBaseURL = v
	}
	if v := strings.TrimSpace(os.Getenv("EMOS_SERVICE_TOKEN")); v != "" {
		config.EmosServiceToken = v
	}
	if v := strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")); v != "" {
		config.PublicBaseURL = v
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
	if storage.LoggedTokens != nil {
		loggedTokens = storage.LoggedTokens
	}
	if storage.AdminChatIds != nil {
		adminChatIds = storage.AdminChatIds
	}
	if storage.AuthUsers != nil {
		authUsers = storage.AuthUsers
	}

	ownerUpdateCount := applyTokenOwnersFromData()
	ownerUpdateCount += applyDefaultOwnerToUnownedUsers()

	// 重置所有随机签到用户的随机时间，以便在程序启动时重新生成
	resetCount := 0
	for i, user := range users {
		if user.Random {
			users[i].RandomTime = ""
			resetCount++
		}
	}

	// 只有当有用户数据并且重置了随机时间或修复了归属时，才保存数据
	if len(users) > 0 && (resetCount > 0 || ownerUpdateCount > 0) {
		saveData()
		printlnUTF8(fmt.Sprintf("已重置 %d 个随机签到用户的随机时间，修复 %d 个账号归属，将在启动后重新生成", resetCount, ownerUpdateCount))
	}

	printlnUTF8(fmt.Sprintf("成功加载数据: %d 个用户，%d 个聊天ID", len(users), len(chatIds)))
}

// 保存数据
func saveData() {
	chatMutex.Lock()
	chatIdsCopy := make(map[int64]bool, len(chatIds))
	for chatID, enabled := range chatIds {
		chatIdsCopy[chatID] = enabled
	}
	loggedTokensCopy := make(map[int64]string, len(loggedTokens))
	for chatID, token := range loggedTokens {
		loggedTokensCopy[chatID] = token
	}
	adminChatIdsCopy := make(map[int64]bool, len(adminChatIds))
	for chatID, enabled := range adminChatIds {
		adminChatIdsCopy[chatID] = enabled
	}
	authUsersCopy := make(map[int64]AuthUser, len(authUsers))
	for chatID, user := range authUsers {
		authUsersCopy[chatID] = user
	}
	chatCount := len(chatIdsCopy)
	chatMutex.Unlock()

	printlnUTF8(fmt.Sprintf("开始保存数据: %d 个用户，%d 个聊天ID", len(users), chatCount))

	// 准备数据
	storage := DataStorage{
		Users:        users,
		ChatIds:      chatIdsCopy,
		LoggedTokens: loggedTokensCopy,
		AdminChatIds: adminChatIdsCopy,
		AuthUsers:    authUsersCopy,
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

	printlnUTF8(fmt.Sprintf("成功保存数据到 %s: %d 个用户，%d 个聊天ID", config.DataFile, len(users), chatCount))
}

func rememberChatID(chatID int64) {
	chatMutex.Lock()
	chatIds[chatID] = true
	chatMutex.Unlock()
}

func isAdmin(chatID int64) bool {
	chatMutex.Lock()
	defer chatMutex.Unlock()
	if len(adminChatIds) == 0 {
		return true
	}
	return adminChatIds[chatID]
}

func getLoggedToken(chatID int64) string {
	chatMutex.Lock()
	defer chatMutex.Unlock()
	if user, ok := authUsers[chatID]; ok && isLoggedIn(user) {
		return serviceToken(user)
	}
	return loggedTokens[chatID]
}

func saveLoggedToken(chatID int64, token string) {
	chatMutex.Lock()
	loggedTokens[chatID] = token
	chatMutex.Unlock()
	saveData()
}

func addAdmin(chatID int64) {
	chatMutex.Lock()
	adminChatIds[chatID] = true
	chatMutex.Unlock()
	saveData()
}

func removeAdmin(chatID int64) {
	chatMutex.Lock()
	delete(adminChatIds, chatID)
	chatMutex.Unlock()
	saveData()
}

func broadcastTelegramMessage(text string) int {
	chatMutex.Lock()
	targets := make([]int64, 0, len(chatIds))
	for chatID := range chatIds {
		targets = append(targets, chatID)
	}
	chatMutex.Unlock()

	for _, chatID := range targets {
		sendTelegramMessage(chatID, text)
	}
	return len(targets)
}

func notifyUser(user User, message string) {
	if user.ChatID == 0 {
		printlnUTF8(fmt.Sprintf("用户 %s 没有绑定chat_id，跳过通知", truncateToken(user.Token)))
		return
	}
	sendTelegramMessage(user.ChatID, message)
}

func parseScheduleTime(value string) (int, int, int, error) {
	parts := strings.Split(value, ":")
	if len(parts) < 2 {
		return 0, 0, 0, fmt.Errorf("invalid time: %s", value)
	}

	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, 0, fmt.Errorf("invalid hour: %s", value)
	}

	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, 0, fmt.Errorf("invalid minute: %s", value)
	}

	second := 0
	if len(parts) >= 3 {
		second, err = strconv.Atoi(parts[2])
		if err != nil || second < 0 || second > 59 {
			return 0, 0, 0, fmt.Errorf("invalid second: %s", value)
		}
	}

	return hour, minute, second, nil
}

func shouldRunScheduledTime(now time.Time, hour, minute, second int, lastCheckDate string) bool {
	if lastCheckDate == now.Format("2006-01-02") {
		return false
	}
	return now.Hour() == hour && now.Minute() == minute && now.Second() >= second
}

func saveUserByToken(user User) {
	usersMutex.Lock()
	for i, existing := range users {
		if existing.Token == user.Token {
			users[i] = user
			break
		}
	}
	usersMutex.Unlock()
	saveData()
}

func applyTokenOwnersFromData() int {
	type tokenOwner struct {
		chatID    int64
		updatedAt time.Time
	}

	ownerByToken := make(map[string]tokenOwner)
	for chatID, token := range loggedTokens {
		if strings.TrimSpace(token) != "" {
			ownerByToken[token] = tokenOwner{chatID: chatID}
		}
	}
	for chatID, user := range authUsers {
		if strings.TrimSpace(user.AuthToken) != "" {
			updatedAt, _ := time.Parse(time.RFC3339, user.UpdatedAt)
			current, exists := ownerByToken[user.AuthToken]
			if !exists || updatedAt.After(current.updatedAt) || current.updatedAt.IsZero() {
				ownerByToken[user.AuthToken] = tokenOwner{chatID: chatID, updatedAt: updatedAt}
			}
		}
	}

	updated := 0
	for i, user := range users {
		owner, ok := ownerByToken[user.Token]
		if !ok || owner.chatID == 0 || user.ChatID == owner.chatID {
			continue
		}
		users[i].ChatID = owner.chatID
		chatIds[owner.chatID] = true
		updated++
	}
	return updated
}

func applyDefaultOwnerToUnownedUsers() int {
	var ownerChatID int64
	enabledAdminCount := 0
	for chatID, enabled := range adminChatIds {
		if !enabled {
			continue
		}
		ownerChatID = chatID
		enabledAdminCount++
	}
	if enabledAdminCount != 1 || ownerChatID == 0 {
		return 0
	}

	updated := 0
	for i, user := range users {
		if user.ChatID != 0 {
			continue
		}
		users[i].ChatID = ownerChatID
		chatIds[ownerChatID] = true
		updated++
	}
	if updated > 0 {
		printlnUTF8(fmt.Sprintf("已将 %d 个旧无归属账号绑定到唯一管理员: %d", updated, ownerChatID))
	}
	return updated
}

func filterUsersForChat(chatID int64) []User {
	result := make([]User, 0)
	for _, user := range users {
		if isAdmin(chatID) || user.ChatID == chatID {
			result = append(result, user)
		}
	}
	return result
}

func htmlEscape(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	text = strings.ReplaceAll(text, "\"", "&quot;")
	text = strings.ReplaceAll(text, "'", "&#39;")
	return text
}

func sanitizeTelegramText(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return text
	}
	if fields[0] == "/start" && len(fields) > 1 && strings.HasPrefix(fields[1], "emosLinkAgree-") {
		return "/start emosLinkAgree-***"
	}
	if fields[0] == "/add" && len(fields) > 2 && !looksLikeTime(fields[1]) {
		fields[1] = truncateToken(fields[1])
		return strings.Join(fields, " ")
	}
	if fields[0] == "/login" && len(fields) > 1 {
		fields[1] = truncateToken(fields[1])
		return strings.Join(fields, " ")
	}
	return text
}

func buildEMOSLoginURL() string {
	payload := fmt.Sprintf("link_%s-%s", config.EmosUserID, config.TelegramBotUsername)
	return fmt.Sprintf("https://t.me/%s?start=%s", config.EmosProviderBot, url.QueryEscape(payload))
}

func serviceToken(user AuthUser) string {
	if strings.TrimSpace(config.EmosServiceToken) != "" {
		return strings.TrimSpace(config.EmosServiceToken)
	}
	return strings.TrimSpace(user.AuthToken)
}

func isLoggedIn(user AuthUser) bool {
	return user.AuthStatus == "agreed" && strings.TrimSpace(user.AuthToken) != "" && strings.TrimSpace(user.EMOSUsername) != ""
}

func getAuthUser(chatID int64) (AuthUser, bool) {
	chatMutex.Lock()
	defer chatMutex.Unlock()
	user, ok := authUsers[chatID]
	return user, ok
}

func saveAuthUser(user AuthUser) {
	user.UpdatedAt = time.Now().In(beijingLoc).Format(time.RFC3339)
	chatMutex.Lock()
	authUsers[user.TelegramID] = user
	if user.AuthToken != "" {
		loggedTokens[user.TelegramID] = user.AuthToken
	}
	chatMutex.Unlock()
	saveData()
}

func saveRefusedAuthUser(profile TelegramProfile) {
	existing, _ := getAuthUser(profile.ID)
	existing.TelegramID = profile.ID
	existing.TelegramUsername = profile.Username
	existing.TelegramFirstName = profile.FirstName
	existing.AuthStatus = "refused"
	saveAuthUser(existing)
}

func sendLoginPrompt(chatID int64) {
	loginURL := buildEMOSLoginURL()
	message := fmt.Sprintf("请先完成EMOS授权登录：\n%s", loginURL)
	sendTelegramMessage(chatID, message)
}

func looksLikeTime(text string) bool {
	text = strings.ReplaceAll(text, "：", ":")
	parts := strings.Split(text, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return false
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return false
	}
	if len(parts) == 3 {
		second, err := strconv.Atoi(parts[2])
		if err != nil || second < 0 || second > 59 {
			return false
		}
	}
	return true
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
			printlnUTF8(fmt.Sprintf("更新用户: %s", truncateToken(req.Token)))
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
		printlnUTF8(fmt.Sprintf("添加新用户: %s", truncateToken(req.Token)))
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
			printlnUTF8(fmt.Sprintf("删除用户: %s", truncateToken(req.Token)))
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

func serveTelegramLoginPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	http.Redirect(w, r, buildEMOSLoginURL(), http.StatusFound)
}

func completeTelegramLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TelegramID        int64  `json:"telegram_id"`
		TelegramUsername  string `json:"telegram_username"`
		TelegramFirstName string `json:"telegram_first_name"`
		Token             string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if req.TelegramID == 0 || strings.TrimSpace(req.Token) == "" {
		http.Error(w, "telegram_id and token are required", http.StatusBadRequest)
		return
	}

	profile := TelegramProfile{
		ID:        req.TelegramID,
		Username:  req.TelegramUsername,
		FirstName: req.TelegramFirstName,
	}
	authUser, err := authorizeEMOSToken(profile, req.Token)
	if err != nil {
		http.Error(w, "login failed", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"telegram_id":   authUser.TelegramID,
		"emos_id":       authUser.EMOSID,
		"emos_username": authUser.EMOSUsername,
	})
}

func authorizeEMOSToken(profile TelegramProfile, token string) (AuthUser, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return AuthUser{}, fmt.Errorf("empty token")
	}
	if err := verifyEMOSToken(token); err != nil {
		return AuthUser{}, err
	}

	emosProfile, err := fetchEMOSProfile(token)
	if err != nil {
		return AuthUser{}, err
	}
	if strings.TrimSpace(emosProfile.Username) == "" {
		return AuthUser{}, fmt.Errorf("missing emos username")
	}

	authUser := AuthUser{
		TelegramID:        profile.ID,
		TelegramUsername:  profile.Username,
		TelegramFirstName: profile.FirstName,
		EMOSUsername:      emosProfile.Username,
		EMOSID:            emosProfile.EMOSID,
		Avatar:            emosProfile.Avatar,
		AuthToken:         token,
		AuthStatus:        "agreed",
	}
	saveAuthUser(authUser)
	return authUser, nil
}

func verifyEMOSToken(token string) error {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/sign/check", config.ApiBaseURL), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("emos token check failed: %d", resp.StatusCode)
	}
	return nil
}

func fetchEMOSProfile(token string) (EMOSProfile, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/user", config.ApiBaseURL), nil)
	if err != nil {
		return EMOSProfile{}, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return EMOSProfile{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return EMOSProfile{}, fmt.Errorf("get emos user failed: %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return EMOSProfile{}, err
	}
	return parseEMOSProfile(body), nil
}

func parseEMOSProfile(body map[string]interface{}) EMOSProfile {
	source := body
	if data, ok := body["data"].(map[string]interface{}); ok {
		source = data
	} else if user, ok := body["user"].(map[string]interface{}); ok {
		source = user
	}

	return EMOSProfile{
		EMOSID:   firstString(source, "user_id", "id", "uuid"),
		Username: firstString(source, "username", "name", "nickname"),
		Avatar:   firstString(source, "avatar", "avatar_url"),
	}
}

func firstString(values map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			switch v := value.(type) {
			case string:
				if strings.TrimSpace(v) != "" {
					return strings.TrimSpace(v)
				}
			case float64:
				return strconv.FormatInt(int64(v), 10)
			case json.Number:
				return v.String()
			}
		}
	}
	return ""
}

// 处理Telegram webhook
func handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var update struct {
		Message struct {
			From struct {
				ID        int64  `json:"id"`
				Username  string `json:"username"`
				FirstName string `json:"first_name"`
			} `json:"from"`
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
		profile := TelegramProfile{
			ID:        chatID,
			Username:  update.Message.From.Username,
			FirstName: update.Message.From.FirstName,
		}

		// 存储chat_id
		rememberChatID(chatID)
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
		printlnUTF8(fmt.Sprintf("原始文本: '%s'", sanitizeTelegramText(text)))
		printlnUTF8(fmt.Sprintf("去除空格后: '%s'", trimmedText))

		processTelegramCommand(chatID, text, profile)
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func processTelegramCommand(chatID int64, text string, profile TelegramProfile) {
	trimmedText := strings.TrimSpace(text)
	command := strings.Fields(trimmedText)
	if len(command) == 0 {
		sendTelegramMessage(chatID, "请使用 /commands 查看可用命令")
		return
	}

	switch command[0] {
	case "/start":
		if len(command) > 1 {
			handleStartPayload(chatID, command[1], profile)
		} else {
			handleStartCommand(chatID)
		}
	case "/help", "/commands":
		handleHelpCommand(chatID)
	case "/myid":
		sendTelegramMessage(chatID, fmt.Sprintf("你的chat_id: %d", chatID))
	case "/account":
		handleAccountCommand(chatID)
	case "/add":
		if trimmedText == "/add" {
			printlnUTF8("调用 startAddUser")
			startAddUser(chatID)
		} else {
			printlnUTF8("调用 handleAddCommand")
			handleAddCommand(chatID, text)
		}
	case "/remove":
		if trimmedText == "/remove" {
			startRemoveUser(chatID)
		} else {
			handleRemoveCommand(chatID, text)
		}
	case "/cancel":
		handleCancelCommand(chatID, text)
	case "/list":
		handleListCommand(chatID, text)
	case "/broadcast":
		handleBroadcastCommand(chatID, text)
	case "/admin":
		handleAdminHelpCommand(chatID)
	case "/admin_add":
		handleAdminAddCommand(chatID, text)
	case "/admin_remove":
		handleAdminRemoveCommand(chatID, text)
	case "/admin_list":
		handleAdminListCommand(chatID)
	case "/setowner":
		handleSetOwnerCommand(chatID, text)
	default:
		sendTelegramMessage(chatID, "请使用 /commands 查看可用命令")
	}
}

func handleStartCommand(chatID int64) {
	if user, ok := getAuthUser(chatID); ok && isLoggedIn(user) {
		sendTelegramMessage(chatID, fmt.Sprintf("你已登录EMOS账号: <b>%s</b>\n发送 /add 可添加自动签到，发送 /account 查看账户。", htmlEscape(user.EMOSUsername)))
		return
	}
	sendLoginPrompt(chatID)
}

func handleStartPayload(chatID int64, payload string, profile TelegramProfile) {
	payload = strings.TrimSpace(payload)
	if strings.HasPrefix(payload, "emosLinkAgree-") {
		token := strings.TrimPrefix(payload, "emosLinkAgree-")
		handleEMOSAgree(chatID, token, profile)
		return
	}
	if strings.HasPrefix(payload, "emosLinkRefuse-") {
		saveRefusedAuthUser(profile)
		sendTelegramMessage(chatID, "你已拒绝EMOS授权。需要重新登录时发送 /start。")
		return
	}

	handleStartCommand(chatID)
}

func handleEMOSAgree(chatID int64, token string, profile TelegramProfile) {
	if strings.TrimSpace(token) == "" {
		sendTelegramMessage(chatID, "登录失败，EMOS授权Token为空。请重新发送 /start 登录。")
		return
	}

	authUser, err := authorizeEMOSToken(profile, token)
	if err != nil {
		printlnUTF8(fmt.Sprintf("EMOS授权失败: chat_id=%d, err=%v", chatID, err))
		sendTelegramMessage(chatID, "登录失败，EMOS授权无效或已过期。请重新发送 /start 登录。")
		return
	}

	sendTelegramMessage(chatID, fmt.Sprintf("EMOS登录成功，当前账号: <b>%s</b>\n现在发送 /add 就可以免输入Token添加自动签到。", htmlEscape(authUser.EMOSUsername)))
}

func handleLoginCommand(chatID int64, text string) {
	handleStartCommand(chatID)
}

func handleAccountCommand(chatID int64) {
	user, ok := getAuthUser(chatID)
	if !ok || !isLoggedIn(user) {
		sendLoginPrompt(chatID)
		return
	}
	message := fmt.Sprintf("EMOS账号: <code>%s</code>\nEMOS ID: <code>%s</code>\nTG ID: <code>%d</code>\n授权状态: <code>%s</code>",
		htmlEscape(user.EMOSUsername), htmlEscape(user.EMOSID), chatID, htmlEscape(user.AuthStatus))
	sendTelegramMessage(chatID, message)
}

func handleBroadcastCommand(chatID int64, text string) {
	if !isAdmin(chatID) {
		sendTelegramMessage(chatID, "没有权限使用广播命令。")
		return
	}

	message := strings.TrimSpace(strings.TrimPrefix(text, "/broadcast"))
	if message == "" {
		sendTelegramMessage(chatID, "格式错误，请使用: /broadcast 通知内容")
		return
	}

	count := broadcastTelegramMessage("管理员广播:\n\n" + message)
	sendTelegramMessage(chatID, fmt.Sprintf("广播已发送给 %d 个聊天。", count))
}

func handleAdminHelpCommand(chatID int64) {
	if !isAdmin(chatID) {
		sendTelegramMessage(chatID, "你当前不是管理员。")
		return
	}
	message := "管理员模式:\n"
	message += "/broadcast 内容 - 向所有已联系Bot的聊天广播\n"
	message += "/setowner 用户名或Token chat_id - 绑定账号通知归属\n"
	message += "/admin_add chat_id - 添加管理员\n"
	message += "/admin_remove chat_id - 移除管理员\n"
	message += "/admin_list - 查看管理员列表"
	sendTelegramMessage(chatID, message)
}

func handleAdminAddCommand(chatID int64, text string) {
	if !isAdmin(chatID) {
		sendTelegramMessage(chatID, "没有权限修改管理员。")
		return
	}

	parts := strings.Fields(text)
	if len(parts) != 2 {
		sendTelegramMessage(chatID, "格式错误，请使用: /admin_add chat_id")
		return
	}

	targetChatID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		sendTelegramMessage(chatID, "chat_id格式错误。")
		return
	}

	addAdmin(targetChatID)
	sendTelegramMessage(chatID, fmt.Sprintf("已添加管理员: %d", targetChatID))
}

func handleAdminRemoveCommand(chatID int64, text string) {
	if !isAdmin(chatID) {
		sendTelegramMessage(chatID, "没有权限修改管理员。")
		return
	}

	parts := strings.Fields(text)
	if len(parts) != 2 {
		sendTelegramMessage(chatID, "格式错误，请使用: /admin_remove chat_id")
		return
	}

	targetChatID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		sendTelegramMessage(chatID, "chat_id格式错误。")
		return
	}

	removeAdmin(targetChatID)
	sendTelegramMessage(chatID, fmt.Sprintf("已移除管理员: %d", targetChatID))
}

func handleAdminListCommand(chatID int64) {
	if !isAdmin(chatID) {
		sendTelegramMessage(chatID, "没有权限查看管理员。")
		return
	}

	chatMutex.Lock()
	defer chatMutex.Unlock()
	if len(adminChatIds) == 0 {
		sendTelegramMessage(chatID, "当前未设置管理员，所有用户临时拥有管理员权限。请先使用 /admin_add chat_id 添加管理员。")
		return
	}

	message := "管理员列表:\n"
	for adminID := range adminChatIds {
		message += fmt.Sprintf("- %d\n", adminID)
	}
	sendTelegramMessage(chatID, message)
}

func handleSetOwnerCommand(chatID int64, text string) {
	if !isAdmin(chatID) {
		sendTelegramMessage(chatID, "没有权限绑定账号归属。")
		return
	}

	parts := strings.Fields(text)
	if len(parts) != 3 {
		sendTelegramMessage(chatID, "格式错误，请使用: /setowner 用户名或Token chat_id")
		return
	}

	targetChatID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		sendTelegramMessage(chatID, "chat_id格式错误。")
		return
	}

	keyword := parts[1]
	usersMutex.Lock()
	defer usersMutex.Unlock()
	for i, user := range users {
		if user.Token == keyword || user.Username == keyword {
			users[i].ChatID = targetChatID
			rememberChatID(targetChatID)
			saveData()
			sendTelegramMessage(chatID, fmt.Sprintf("已将账号 %s 绑定到 chat_id %d。", htmlEscape(user.Username), targetChatID))
			return
		}
	}

	sendTelegramMessage(chatID, "未找到对应账号。")
}

// 开始添加用户流程
func startAddUser(chatID int64) {
	if token := getLoggedToken(chatID); token != "" {
		userInfo, err := getUserInfo(token)
		if err != nil {
			sendTelegramMessage(chatID, "已保存的授权失效，请发送 /start 重新进行EMOS授权。")
		} else {
			stateMutex.Lock()
			userStates[chatID] = UserState{
				Type: StateWaitAddAccount,
				Data: map[string]string{
					"token":    token,
					"username": userInfo.Username,
				},
				CreateTime: time.Now(),
			}
			stateMutex.Unlock()
			sendTelegramMessage(chatID, "请选择要添加的账号:\n1. 添加当前登录账号\n2. 添加小号Token\n输入0取消")
			return
		}
	}

	sendLoginPrompt(chatID)
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
		handleWaitToken(chatID, text, state.Data)
	case StateWaitAddAccount:
		handleWaitAddAccount(chatID, text, state.Data)
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
func handleWaitToken(chatID int64, text string, data map[string]string) {
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
	if data == nil {
		data = make(map[string]string)
	}
	if data["small_account"] != "true" {
		saveLoggedToken(chatID, text)
	}

	// 保存token和username并进入下一步
	stateMutex.Lock()
	userStates[chatID] = UserState{
		Type: StateWaitMode,
		Data: map[string]string{
			"token":    text,
			"username": userInfo.Username,
		},
		CreateTime: time.Now(),
	}
	stateMutex.Unlock()

	// 发送提示消息
	sendTelegramMessage(chatID, "请选择签到模式:\n1. 固定时间签到\n2. 随机时间签到\n输入0取消")
}

func handleWaitAddAccount(chatID int64, text string, data map[string]string) {
	if text == "0" {
		stateMutex.Lock()
		delete(userStates, chatID)
		stateMutex.Unlock()
		sendTelegramMessage(chatID, "已取消添加用户")
		return
	}
	if text != "1" && text != "2" {
		sendTelegramMessage(chatID, "请输入正确的选项(1或2):")
		return
	}

	if text == "2" {
		stateMutex.Lock()
		userStates[chatID] = UserState{
			Type: StateWaitToken,
			Data: map[string]string{
				"small_account": "true",
			},
			CreateTime: time.Now(),
		}
		stateMutex.Unlock()
		sendTelegramMessage(chatID, "请输入小号Token:\n输入0取消")
		return
	}

	stateMutex.Lock()
	userStates[chatID] = UserState{
		Type:       StateWaitMode,
		Data:       data,
		CreateTime: time.Now(),
	}
	stateMutex.Unlock()
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

	if random {
		data["time"] = "00:00:00"
		stateMutex.Lock()
		userStates[chatID] = UserState{
			Type:       StateWaitRemark,
			Data:       data,
			CreateTime: time.Now(),
		}
		stateMutex.Unlock()
		sendTelegramMessage(chatID, "已选择随机时间签到，请输入备注信息(如不需要备注，输入0):")
		return
	}

	stateMutex.Lock()
	userStates[chatID] = UserState{
		Type:       StateWaitTime,
		Data:       data,
		CreateTime: time.Now(),
	}
	stateMutex.Unlock()

	// 发送提示消息
	sendTelegramMessage(chatID, "请输入固定签到时间(格式: HH:MM:SS，例如: 08:30:00):\n输入0取消")
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
	sendTelegramMessage(chatID, "请输入备注信息(如不需要备注，输入0):")
}

// 处理等待备注状态
func handleWaitRemark(chatID int64, text string, data map[string]string) {
	// 检查是否输入0表示不需要备注
	var remark string
	if text == "0" {
		// 输入0表示不需要备注
		remark = ""
	} else {
		// 处理备注
		remark = text
	}

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
			if user.ChatID != chatID && !isAdmin(chatID) {
				sendTelegramMessage(chatID, "没有权限删除这个账号。")
				return
			}
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
	printlnUTF8(fmt.Sprintf("Full text: '%s'", sanitizeTelegramText(text)))
	printlnUTF8(fmt.Sprintf("Text length: %d", len(text)))

	parts := strings.Split(text, " ")

	// 调试日志：打印分割后的部分
	printlnUTF8(fmt.Sprintf("Split parts count: %d", len(parts)))
	printlnUTF8(fmt.Sprintf("Parts count: %d", len(parts)))

	// 过滤空字符串元素
	var filteredParts []string
	for i, part := range parts {
		logPart := part
		if i == 1 && len(parts) > 2 && !looksLikeTime(part) {
			logPart = truncateToken(part)
		}
		printlnUTF8(fmt.Sprintf("Part %d: '%s' (empty: %t)", i, logPart, part == ""))
		if part != "" {
			filteredParts = append(filteredParts, part)
		}
	}

	// 调试日志：打印过滤后的部分
	printlnUTF8(fmt.Sprintf("Filtered parts count: %d", len(filteredParts)))
	printlnUTF8(fmt.Sprintf("Filtered parts count: %d", len(filteredParts)))

	if len(filteredParts) < 2 {
		sendTelegramMessage(chatID, "格式错误，请使用: /add token time [random] [remark]，或先 /start 完成EMOS授权后使用 /add time [random] [remark]")
		return
	}

	token := ""
	timeIndex := 2
	if len(filteredParts) >= 3 && looksLikeTime(filteredParts[1]) {
		token = getLoggedToken(chatID)
		timeIndex = 1
	} else if len(filteredParts) >= 3 {
		token = filteredParts[1]
	} else {
		token = getLoggedToken(chatID)
		timeIndex = 1
	}
	if token == "" {
		sendTelegramMessage(chatID, "请先使用 /start 完成EMOS授权，或使用: /add token time [random] [remark]")
		return
	}

	isSmallAccount := false
	stateMutex.Lock()
	if state, exists := userStates[chatID]; exists && state.Data != nil {
		isSmallAccount = state.Data["small_account"] == "true"
	}
	stateMutex.Unlock()
	if !isSmallAccount {
		saveLoggedToken(chatID, token)
	}

	timeStr := filteredParts[timeIndex]
	// 替换中文冒号为英文冒号
	timeStr = strings.ReplaceAll(timeStr, "：", ":")
	random := false
	remark := ""

	// 检查是否有random参数
	if len(filteredParts) > timeIndex+1 {
		nextIndex := timeIndex + 1
		if filteredParts[nextIndex] == "random" {
			random = true
			// 检查是否有备注参数
			if len(filteredParts) > nextIndex+1 {
				// 合并剩余的所有部分作为备注
				remark = strings.Join(filteredParts[nextIndex+1:], " ")
			}
		} else {
			// 剩余的所有部分作为备注
			remark = strings.Join(filteredParts[nextIndex:], " ")
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
			randomHour := hour + rand.Intn(hourRange+1)
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
				ChatID:     chatID,
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
			ChatID:     chatID,
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
			if user.ChatID != chatID && !isAdmin(chatID) {
				sendTelegramMessage(chatID, "没有权限删除这个账号。")
				return
			}
			// 删除用户
			users = append(users[:i], users[i+1:]...)
			found = true
			printlnUTF8(fmt.Sprintf("删除用户: %s", truncateToken(token)))
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

func handleCancelCommand(chatID int64, text string) {
	parts := strings.Fields(text)
	target := ""
	if len(parts) > 1 {
		target = parts[1]
	}

	usersMutex.Lock()
	defer usersMutex.Unlock()

	admin := isAdmin(chatID)
	matchIndex := -1
	visibleIndices := make([]int, 0, len(users))
	for i, user := range users {
		if !admin && user.ChatID != chatID {
			continue
		}
		visibleIndices = append(visibleIndices, i)
		if target == "" || user.Token == target || user.Username == target {
			if matchIndex != -1 && target == "" {
				sendTelegramMessage(chatID, "你有多个自动签到账号，请使用 /cancel 编号、/cancel 用户名 或 /cancel token 指定要取消的账号。")
				return
			}
			matchIndex = i
		}
	}

	if len(visibleIndices) == 0 {
		sendTelegramMessage(chatID, "你当前没有自动签到账号。")
		return
	}
	if target != "" {
		if listNumber, err := strconv.Atoi(target); err == nil {
			if listNumber < 1 || listNumber > len(visibleIndices) {
				sendTelegramMessage(chatID, fmt.Sprintf("编号不存在。请发送 /list 查看可取消的编号，当前可用范围: 1-%d。", len(visibleIndices)))
				return
			}
			matchIndex = visibleIndices[listNumber-1]
		}
	}
	if matchIndex == -1 {
		sendTelegramMessage(chatID, "未找到要取消的自动签到账号。")
		return
	}

	removedUser := users[matchIndex]
	users = append(users[:matchIndex], users[matchIndex+1:]...)
	saveData()
	sendTelegramMessage(chatID, fmt.Sprintf("已取消账号 <b>%s</b> 的自动签到。", htmlEscape(removedUser.Username)))
}

// 转义MarkdownV2特殊字符
func escapeMarkdownV2(text string) string {
	// 转义MarkdownV2中的特殊字符
	text = strings.ReplaceAll(text, "_", "\\_")
	text = strings.ReplaceAll(text, "*", "\\*")
	text = strings.ReplaceAll(text, "[", "\\[")
	text = strings.ReplaceAll(text, "]", "\\]")
	text = strings.ReplaceAll(text, "(", "\\(")
	text = strings.ReplaceAll(text, ")", "\\)")
	text = strings.ReplaceAll(text, "~", "\\~")
	text = strings.ReplaceAll(text, "`", "\\`")
	text = strings.ReplaceAll(text, "#", "\\#")
	text = strings.ReplaceAll(text, "$", "\\$")
	text = strings.ReplaceAll(text, "+", "\\+")
	text = strings.ReplaceAll(text, "-", "\\-")
	text = strings.ReplaceAll(text, "=", "\\=")
	text = strings.ReplaceAll(text, "|", "\\|")
	text = strings.ReplaceAll(text, "{", "\\{")
	text = strings.ReplaceAll(text, "}", "\\}")
	text = strings.ReplaceAll(text, ".", "\\.")
	text = strings.ReplaceAll(text, "!", "\\!")
	return text
}

// 处理列出用户命令
const listPageSize = 5

func handleListCommand(chatID int64, text string) {
	admin := isAdmin(chatID)
	page := parseListPage(text)

	usersMutex.Lock()
	defer usersMutex.Unlock()

	visibleUsers := make([]User, 0, len(users))
	for _, user := range users {
		if admin || user.ChatID == chatID {
			visibleUsers = append(visibleUsers, user)
		}
	}

	if len(visibleUsers) == 0 {
		sendTelegramMessage(chatID, "当前没有可管理的签到用户!")
		return
	}

	totalPages := (len(visibleUsers) + listPageSize - 1) / listPageSize
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * listPageSize
	end := start + listPageSize
	if end > len(visibleUsers) {
		end = len(visibleUsers)
	}

	message := fmt.Sprintf("📋 <b>自动签到账号列表</b>\n\n🔹 第 <b>%d/%d</b> 页 · 共 <b>%d</b> 个账号\n\n", page, totalPages, len(visibleUsers))
	for i, user := range visibleUsers[start:end] {
		message += formatListUser(start+i+1, user, admin)
		if i < end-start-1 {
			message += "\n"
		}
	}
	message += "\n💡 取消签到：<code>/cancel 编号</code>"
	if page < totalPages {
		message += fmt.Sprintf("\n➡️ 下一页：<code>/list %d</code>", page+1)
	}
	if page > 1 {
		message += fmt.Sprintf("\n⬅️ 上一页：<code>/list %d</code>", page-1)
	}
	sendTelegramMessage(chatID, message)
}

func parseListPage(text string) int {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) < 2 {
		return 1
	}
	page, err := strconv.Atoi(fields[1])
	if err != nil || page < 1 {
		return 1
	}
	return page
}

func formatListUser(index int, user User, admin bool) string {
	username := user.Username
	if username == "" {
		username = "未知用户"
	}
	message := fmt.Sprintf("🔹 <b>账号 #%d</b>\n", index)
	message += fmt.Sprintf(" ├ 👤 用户名：<b>%s</b>\n", htmlEscape(username))
	message += fmt.Sprintf(" ├ 🔑 Token：<code>%s</code>\n", htmlEscape(truncateToken(user.Token)))
	message += fmt.Sprintf(" ├ ⏰ 签到时间：%s\n", htmlEscape(formatScheduleTime(user)))
	if user.Random {
		message += fmt.Sprintf(" ├ 🎲 今日随机：%s\n", htmlEscape(formatRandomTime(user.RandomTime)))
	}
	if user.Remark != "" {
		message += fmt.Sprintf(" ├ 📝 备注：%s\n", htmlEscape(user.Remark))
	}
	if admin {
		message += fmt.Sprintf(" ╰ 🧑‍💼 Owner：<code>%d</code>\n", user.ChatID)
	} else {
		message += " ╰ 🗑 删除：<code>/cancel 编号</code>\n"
	}
	return message
}

func formatScheduleTime(user User) string {
	if user.Random {
		return "随机"
	}
	if strings.TrimSpace(user.Time) == "" {
		return "未设置"
	}
	return user.Time
}

func formatRandomTime(randomTime string) string {
	if strings.TrimSpace(randomTime) == "" {
		return "未生成"
	}
	return randomTime
}

// 处理帮助命令
func handleHelpCommand(chatID int64) {
	message := "命令列表:\n\n"
	message += "登录/账户:\n"
	message += "/start - 登录入口/查看登录状态\n"
	message += "/account - 查看当前EMOS账户\n\n"
	message += "自动签到:\n"
	message += "/add - 按提示选择本账号或小号添加自动签到\n"
	message += "/add token time [random] [remark] - 兼容旧格式添加\n"
	message += "/add time [random] [remark] - 登录后免Token快速添加\n"
	message += "/cancel [编号/用户名/Token] - 取消自动签到\n"
	message += "/remove token - 删除自动签到账号\n"
	message += "/list - 查看自己的签到账号\n"
	message += "/myid - 查看自己的chat_id\n"
	message += "/commands - 显示命令列表\n"
	if isAdmin(chatID) {
		message += "\n管理员模式: 已开启\n"
		message += "/admin - 查看管理员命令\n"
		message += "/broadcast 内容 - 向所有已联系Bot的聊天广播\n"
		message += "/setowner 用户名或Token chat_id - 绑定账号通知归属\n"
		message += "/admin_add chat_id - 添加管理员\n"
		message += "/admin_remove chat_id - 移除管理员\n"
		message += "/admin_list - 查看管理员列表"
	}

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
	telegramMessageSender(chatID, text)
}

func defaultSendTelegramMessage(chatID int64, text string) {
	printlnUTF8(fmt.Sprintf("发送消息到Telegram: %d, %s", chatID, text))

	// 使用HTML格式，确保用户名显示为粗体
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		printlnUTF8(fmt.Sprintf("JSON编码失败: %v", err))
		return
	}

	apiURL := fmt.Sprintf("%s%s/sendMessage", config.TelegramApiURL, config.TelegramBotToken)
	printlnUTF8(fmt.Sprintf("发送消息URL: %s", apiURL))
	printlnUTF8(fmt.Sprintf("发送消息数据: %s", string(data)))

	// 创建一个带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(string(data)))
	if err != nil {
		printlnUTF8(fmt.Sprintf("创建请求失败: %v", err))
		return
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")

	// 使用与telegramPolling函数相同的HTTP客户端设置
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// 发送请求
	resp, err := client.Do(req)
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

func setTelegramBotCommands() {
	commands := []map[string]string{
		{"command": "start", "description": "开始使用/登录入口"},
		{"command": "account", "description": "查看当前EMOS账户"},
		{"command": "add", "description": "添加自动签到账号"},
		{"command": "cancel", "description": "取消自动签到"},
		{"command": "list", "description": "查看签到账号列表"},
		{"command": "myid", "description": "查看自己的chat_id"},
		{"command": "commands", "description": "显示完整指令列表"},
		{"command": "admin", "description": "管理员模式"},
	}

	payload := map[string]interface{}{
		"commands": commands,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		printlnUTF8(fmt.Sprintf("设置Telegram指令列表失败: %v", err))
		return
	}

	apiURL := fmt.Sprintf("%s%s/setMyCommands", config.TelegramApiURL, config.TelegramBotToken)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(string(data)))
	if err != nil {
		printlnUTF8(fmt.Sprintf("创建设置指令列表请求失败: %v", err))
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		printlnUTF8(fmt.Sprintf("设置Telegram指令列表失败: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		printlnUTF8(fmt.Sprintf("设置Telegram指令列表失败，状态码: %d", resp.StatusCode))
		return
	}
	printlnUTF8("Telegram指令列表设置成功")
}

// 签到调度器
func checkinScheduler() {
	// 启动时立即执行一次
	checkinUsers()

	// 初始使用分钟级检查
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		<-ticker.C
		// 检查是否需要切换到秒级检查
		if needSecondLevelCheck() {
			// 切换到秒级检查
			ticker.Reset(1 * time.Second)
			printlnUTF8("切换到秒级检查模式")
		} else {
			// 切换到分钟级检查
			ticker.Reset(1 * time.Minute)
			printlnUTF8("切换到分钟级检查模式")
		}
		checkinUsers()
	}
}

// 检查是否需要秒级检查
func needSecondLevelCheck() bool {
	usersMutex.Lock()
	defer usersMutex.Unlock()

	now := time.Now().In(beijingLoc)
	currentHour := now.Hour()
	currentMinute := now.Minute()

	// 检查是否有用户的签到时间在接下来的1分钟内
	for _, user := range users {
		var targetHour, targetMinute int
		var targetTime string

		if user.Random && user.RandomTime != "" && user.RandomTime != "checked" {
			targetTime = user.RandomTime
		} else {
			targetTime = user.Time
		}

		parts := strings.Split(targetTime, ":")
		if len(parts) >= 2 {
			targetHour, _ = strconv.Atoi(parts[0])
			targetMinute, _ = strconv.Atoi(parts[1])

			// 计算时间差（分钟）
			timeDiff := (targetHour*60 + targetMinute) - (currentHour*60 + currentMinute)
			if timeDiff < 0 {
				timeDiff += 24 * 60 // 跨天
			}

			// 如果在1分钟内，需要秒级检查
			if timeDiff <= 1 {
				return true
			}
		}
	}

	return false
}

// 检查用户签到
func checkinUsers() {
	usersMutex.Lock()
	// 检查是否是新的一天，如果是，重置所有用户的随机时间
	now := time.Now().In(beijingLoc)
	currentHour := now.Hour()
	currentMinute := now.Minute()
	currentSecond := now.Second()
	today := now.Format("2006-01-02")

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
			if user.LastCheckDate == today {
				if user.RandomTime != "checked" {
					user.RandomTime = "checked"
					saveUserByToken(user)
				}
				continue
			}

			// 随机签到，每天随机选择一个时间
			// 检查是否已经生成今天的随机时间
			randomHour := 0
			randomMinute := 0
			randomSecond := 0
			needUpdate := false

			if user.RandomTime == "" || user.RandomTime == "checked" {
				// 生成今天的随机时间，确保是当前时间之后的时间
				// 使用外部的currentHour和currentMinute和currentSecond，确保时间一致性
				hourRange := 23 - currentHour
				if hourRange > 0 {
					randomHour = currentHour + rand.Intn(hourRange+1)
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
						// 当前分钟是59，只能选择59
						randomMinute = 59
					}
				} else {
					// 其他小时，生成任意分钟
					randomMinute = rand.Intn(60)
				}

				// 如果是当前分钟，生成当前秒之后的秒
				if randomHour == currentHour && randomMinute == currentMinute {
					secondRange := 59 - currentSecond
					if secondRange > 0 {
						randomSecond = currentSecond + 1 + rand.Intn(secondRange)
					} else {
						// 当前秒是59，等待下一分钟
						randomSecond = 0
						randomMinute++
						if randomMinute > 59 {
							randomMinute = 0
							randomHour++
							if randomHour > 23 {
								randomHour = 23
							}
						}
					}
				} else {
					// 其他小时或分钟，生成任意秒数
					randomSecond = rand.Intn(60)
				}

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

			// 恢复秒级检查
			printlnUTF8(fmt.Sprintf("随机签到: user=%s, 随机时间=%s, 当前时间=%02d:%02d:%02d", truncateToken(user.Token), user.RandomTime, currentHour, currentMinute, currentSecond))

			if shouldRunScheduledTime(now, randomHour, randomMinute, randomSecond, user.LastCheckDate) {
				printlnUTF8(fmt.Sprintf("开始随机签到用户: %s", truncateToken(user.Token)))
				// 签到后清空随机时间，并且当天不再重新生成
				user.RandomTime = "checked"
				user.LastCheckDate = today
				saveUserByToken(user)
				go performCheckin(user)
				continue
			}

			// 跳过已经签到的用户，不再生成随机时间
			if user.RandomTime == "checked" {
				continue
			}

			// 如果需要更新用户信息，保存到数据中
			if needUpdate {
				saveUserByToken(user)
			}
		} else {
			parts := strings.Split(user.Time, ":")
			printlnUTF8(fmt.Sprintf("固定时间签到: user=%s, time=%s, split parts=%v", truncateToken(user.Token), user.Time, parts))
			hour, minute, second, err := parseScheduleTime(user.Time)
			if err != nil {
				printlnUTF8(fmt.Sprintf("用户 %s 的时间格式无效: %s", truncateToken(user.Token), user.Time))
				continue
			}

			printlnUTF8(fmt.Sprintf("检查用户 %s: 计划时间=%02d:%02d:%02d, 当前时间=%02d:%02d:%02d", truncateToken(user.Token), hour, minute, second, currentHour, currentMinute, currentSecond))
			if shouldRunScheduledTime(now, hour, minute, second, user.LastCheckDate) {
				printlnUTF8(fmt.Sprintf("开始签到用户: %s", truncateToken(user.Token)))
				user.LastCheckDate = today
				saveUserByToken(user)
				go performCheckin(user)
			}
		}
	}
}

// 执行签到
func performCheckin(user User) {
	token := user.Token
	// 获取用户信息
	userInfo, err := getUserInfo(token)
	if err != nil {
		printlnUTF8(fmt.Sprintf("获取用户信息失败: %v", err))
		message := fmt.Sprintf("📅 签到通知\n\n用户名: %s\n签到状态: 获取用户信息失败\n错误信息: %v", user.Username, err)
		notifyUser(user, message)
		return
	}
	if user.Username == "" {
		user.Username = userInfo.Username
	}

	// 执行签到
	result, err, statusText := checkin(token)
	if err != nil {
		printlnUTF8(fmt.Sprintf("签到失败: %v", err))
		// 发送失败通知
		message := fmt.Sprintf("📅 签到通知\n\n用户名: %s\n签到状态: %s\n错误信息: %v", userInfo.Username, statusText, err)
		notifyUser(user, message)
		return
	}

	// 发送通知
	if statusText == "签到成功" {
		sendCheckinNotification(user, result, statusText)
	} else {
		message := fmt.Sprintf("📅 签到通知\n\n用户名: %s\n签到状态: %s", userInfo.Username, statusText)
		notifyUser(user, message)
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

	// 使用与setupNetwork函数中相同的代理设置
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
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

func normalizeSignContent(content string) string {
	content = strings.TrimSpace(content)
	if isValidSignContent(content) {
		return content
	}

	shortContent := []string{
		"签到", "打卡", "到", "在", "好", "棒", "冲",
		"1", "2", "3", "ok", "hi", "go", "yes",
		"✨", "🎉", "👍",
	}
	return shortContent[rand.Intn(len(shortContent))]
}

func isValidSignContent(content string) bool {
	content = strings.TrimSpace(content)
	return content != "" && len([]rune(content)) <= 10
}

func pickSignContent(contents []string) string {
	validContents := make([]string, 0, len(contents))
	for _, content := range contents {
		if isValidSignContent(content) {
			validContents = append(validContents, strings.TrimSpace(content))
		}
	}
	if len(validContents) == 0 {
		return normalizeSignContent("")
	}
	return validContents[rand.Intn(len(validContents))]
}

// 执行签到
func checkin(token string) (CheckinResult, error, string) {
	var result CheckinResult
	var statusText string

	// 准备多种签到内容，随机生成模拟真实用户输入
	// 定义各种类型的签到内容
	pureNumbers := []string{
		"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15",
		"16", "17", "18", "19", "20", "21", "22", "23", "24", "25", "26", "27", "28", "29", "30",
		"66", "88", "99", "100", "111", "222", "333", "444", "555", "666", "777", "888", "999",
		"520", "1314", "2024", "2025", "1234", "4321", "6666", "8888", "9999",
		"168", "668", "988", "518", "888", "666", "189", "288", "388", "488",
	}

	chineseText := []string{
		// 基础签到语
		"签到", "打卡", "报到", "来了", "到", "在", "好", "到！",
		// 日常问候
		"你好", "早上好", "中午好", "下午好", "晚上好", "晚安",
		"早", "午", "晚", "安", "嗨", "嘿", "哈喽", "嘿呀",
		// 签到动作
		"每日签到", "日常打卡", "我来啦", "我来签到了", "准时打卡", "准时报到",
		"签到成功", "报到成功", "打卡成功", "已签到", "已打卡", "已报到",
		// 鼓励语
		"加油", "努力", "冲鸭", "冲", "奋斗", "拼搏", "坚持", "努力奋斗",
		"今天也要加油", "元气满满", "新的一天", "新的一天开始啦",
		"又是元气满满的一天", "充满干劲", "干劲十足", "冲冲冲",
		// 天数相关
		"第一天", "第二天", "第三天", "第四天", "第五天", "第六天", "第七天",
		"坚持第1天", "坚持第2天", "坚持第3天", "坚持第4天", "坚持第5天",
		"打卡第1天", "打卡第2天", "打卡第3天", "连续签到", "连续打卡",
		// 网络用语
		"奥利给", "铁汁", "老铁", "集美", "家人们", "兄弟们", "姐妹们",
		"绝绝子", "YYDS", "永远的神", "666", "针不戳", "喜大普奔",
		"emo", "打工人", "搬砖", "社畜", "干饭人", "干饭魂",
		// 表情动作
		"比心", "点赞", "撒花", "鼓掌", "开心", "高兴", "快乐", "愉快",
		"摸摸头", "握爪", "勾搭", "飞吻", "拥抱", "挥手",
		// 其他有趣的中文
		"哈哈哈哈", "嘿嘿", "嘻嘻", "么么哒", "棒棒哒", "美滋滋", "乐呵呵",
		"收到", "明白", "了解", "好的", "OK", "yes", "sure", "来了来了",
		"我来了", "报到报到", "打卡打卡", "签到签到", "日常日常",
	}

	emojiText := []string{
		// 单个表情
		"👍", "✨", "🌟", "💪", "🎉", "✅", "📝", "🏃", "💯", "🔥",
		"😊", "😄", "😎", "🤞", "✌", "🤝", "👏", "🙌", "💫", "⭐",
		"🌈", "☀", "🌙", "🌸", "🌺", "🍀", "🎊", "🎁", "🎈", "🎂",
		"❤️", "🧡", "💛", "�", "💙", "💜", "🖤", "🤍", "💕", "💖",
		// 双个表情
		"�👍👍", "✨✨", "🌟🌟", "💪💪", "🎉🎉", "✅✅", "📝📝", "🏃🏃", "💯💯", "🔥🔥",
		"😊😊", "😄😄", "😎😎", "🤞🤞", "✌✌", "🤝🤝", "👏👏", "🙌🙌", "💫💫", "⭐⭐",
		"🌈🌈", "☀☀", "🌙🌙", "🌸🌸", "🌺🌺", "🍀🍀", "🎊🎊", "🎁🎁", "🎈🎈", "🎂🎂",
		"❤️❤️", "🧡🧡", "💛💛", "💚💚", "💙💙", "💜💜", "🖤🖤", "🤍🤍", "💕💕", "💖💖",
		// 三个表情
		"👍👍👍", "✨✨✨", "🌟🌟🌟", "💪💪💪", "🎉🎉🎉",
		"✅✅✅", "📝📝📝", "🏃🏃🏃", "💯💯💯", "🔥🔥🔥",
	}

	englishText := []string{
		// 基础英语
		"checkin", "sign", "here", "yes", "ok", "good", "great", "nice",
		"check", "sign in", "let's go", "let's do this", "gogogo", "on my way",
		// 简短英语
		"hi", "hey", "yo", "sup", "yep", "yeah", "ya", "ok", "okay",
		"good", "nice", "cool", "wow", "omg", "lol", "haha", "hehe",
		// 动作英语
		"let's go", "go go go", "start", "begin", "ready", "set", "go",
		"work hard", "keep going", "never give up", "stay strong", "stay positive",
		// 特殊表达
		"day 1", "day 2", "day 3", "day 100", "day 1000",
		"count me in", "i'm in", "counting", "1st day", "2nd day",
		"morning", "afternoon", "evening", "night",
		"love it", "so cool", "so nice", "so good", "very good",
	}

	mixedContent := []string{
		// day系列
		"day1", "day2", "day3", "day4", "day5", "day6", "day7", "day8", "day9", "day10",
		"day11", "day12", "day13", "day14", "day15", "day20", "day30", "day50", "day100", "day666",
		// 日期系列
		"1/1", "1/2", "2/2", "3/3", "4/4", "5/5", "6/6", "7/7", "8/8", "9/9", "10/10",
		"today", "tomorrow", "yesterday", "now", "just now", "right now",
		// 数字+表情
		"👍1", "👍2", "👍3", "✨1", "✨2", "💪1", "💪2", "🔥1", "🔥2", "✅1", "✅2",
		"1👍", "2👍", "3👍", "1✨", "2✨", "1💪", "2💪", "1🔥", "2🔥", "1✅", "2✅",
		// 英文+数字
		"day1!", "day2!", "day3!", "day4!", "day5!", "check1", "check2", "sign1", "sign2",
		"test1", "test2", "try1", "try2", "go1", "go2", "start1", "start2",
		// 混合文字
		"hello2024", "hello2025", "check2024", "sign2024", "签到1", "签到2", "打卡1", "打卡2",
		"加油1", "加油2", "冲鸭1", "冲鸭2", "好1", "好2", "到1", "到2",
		"1打卡", "2打卡", "1签到", "2签到", "1加油", "2加油", "1冲", "2冲",
		// 有趣组合
		"👍👍👍", "�💪💪", "🔥🔥🔥", "✅✅✅", "✨✨✨", "🎉🎉🎉",
		"12345", "54321", "67890", "09876", "11111", "22222", "33333",
		"100分", "120分", "180分", "�分", "100分!", "120分!",
	}

	// 随机选择内容类型，然后从该类型中随机选择
	contentType := rand.Intn(5)
	var content string

	switch contentType {
	case 0:
		// 纯数字
		content = pickSignContent(pureNumbers)
	case 1:
		// 中文文字
		content = pickSignContent(chineseText)
	case 2:
		// Emoji
		content = pickSignContent(emojiText)
	case 3:
		// 英文
		content = pickSignContent(englishText)
	case 4:
		// 混合内容
		content = pickSignContent(mixedContent)
	}
	content = normalizeSignContent(content)

	// 将content作为查询参数添加到URL中
	url := fmt.Sprintf("%s/api/user/sign?content=%s", config.ApiBaseURL, url.QueryEscape(content))

	// 打印请求信息
	printlnUTF8("=== 签到请求 ===")
	printlnUTF8(fmt.Sprintf("URL: %s", url))
	printlnUTF8("Method: PUT")
	printlnUTF8(fmt.Sprintf("Token: %s", truncateToken(token)))

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
func sendCheckinNotification(user User, result CheckinResult, statusText string) {
	username := user.Username
	if username == "" {
		username = "未知用户"
	}
	message := fmt.Sprintf("🎉 <b>签到成功！</b>\n\n🔹 <b>%s</b>\n ├ ✅ 签到状态：%s\n ├ 🌱 累计签到：%d 天\n ├ 🥕 获得萝卜：%d 个\n ╰ 🏆 今日排名：%d\n\n💡 明天也记得来 emos 签到吧！",
		htmlEscape(username), htmlEscape(statusText), result.ContinuousDays, result.EarnPoint, result.SignIndex)
	notifyUser(user, message)
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
		chatMutex.Lock()
		chatCount := len(chatIds)
		chatMutex.Unlock()
		printlnUTF8(fmt.Sprintf("[%s] 系统运行中 - 当前用户: %d, 当前chat_ids: %d", time.Now().In(beijingLoc).Format("15:04:05"), userCount, chatCount))
	}
}

// Token更换提醒调度器 - 每隔30分钟提醒用户更换token，在指定时间结束
func tokenReminderScheduler() {
	// 设置提醒结束时间：2026年5月22日00:00:00
	endTime := time.Date(2026, 5, 22, 0, 0, 0, 0, beijingLoc)

	// 检查是否已经过了结束时间
	now := time.Now().In(beijingLoc)
	if now.After(endTime) {
		printlnUTF8("Token提醒已在2026-05-22 00:00结束")
		return
	}

	// 发送初始提醒
	sendTokenReminder()

	// 每隔30分钟发送一次提醒
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		<-ticker.C

		// 检查是否已经过了结束时间
		now = time.Now().In(beijingLoc)
		if now.After(endTime) {
			printlnUTF8("Token提醒已在2026-05-22 00:00结束")
			return
		}

		sendTokenReminder()
	}
}

// 发送Token更换提醒消息
func sendTokenReminder() {
	message := `🔔 重要提醒！

Token即将重置，请及时更换您的签到Token！

为了避免断签，请尽快使用 /add 命令重新添加您的账户。

操作步骤：
1. 发送 /add 命令
2. 按照提示输入新的Token
3. 设置签到时间（固定时间或随机）
4. 添加备注（可选）

如果Token过期导致断签，之前的连续签到天数将会清零，请务必及时更新！

如有疑问，请随时联系管理员。`

	printlnUTF8("发送Token更换提醒通知...")

	usersMutex.Lock()
	targets := make(map[int64]bool)
	for _, user := range users {
		if user.ChatID != 0 {
			targets[user.ChatID] = true
		}
	}
	usersMutex.Unlock()

	for chatID := range targets {
		sendTelegramMessage(chatID, message)
	}
}

// Telegram消息轮询
func getInitialTelegramOffset() int64 {
	apiURL := fmt.Sprintf("%s%s/getUpdates?offset=-1&timeout=0", config.TelegramApiURL, config.TelegramBotToken)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		printlnUTF8(fmt.Sprintf("初始化Telegram offset失败: %v", err))
		return 0
	}

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		printlnUTF8(fmt.Sprintf("初始化Telegram offset失败: %v", err))
		return 0
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		printlnUTF8(fmt.Sprintf("读取初始化Telegram offset失败: %v", err))
		return 0
	}

	var result struct {
		Ok     bool `json:"ok"`
		Result []struct {
			UpdateID int64 `json:"update_id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil || !result.Ok || len(result.Result) == 0 {
		return 0
	}

	offset := result.Result[len(result.Result)-1].UpdateID + 1
	printlnUTF8(fmt.Sprintf("Telegram offset初始化为: %d", offset))
	return offset
}

func telegramPolling() {
	printlnUTF8("开始Telegram轮询...")

	// 存储最后处理的消息ID
	offset := getInitialTelegramOffset()

	for {
		// 捕获循环中的panic
		defer func() {
			if r := recover(); r != nil {
				printlnUTF8(fmt.Sprintf("Telegram轮询循环崩溃: %v", r))
			}
		}()

		// 调用getUpdates API
		apiURL := fmt.Sprintf("%s%s/getUpdates?offset=%d&timeout=10", config.TelegramApiURL, config.TelegramBotToken, offset)
		printlnUTF8(fmt.Sprintf("获取Telegram更新: %s", apiURL))

		// 创建一个带超时的上下文
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)

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
			Timeout: 15 * time.Second,
		}
		resp, err := client.Do(req)
		if err != nil {
			printlnUTF8(fmt.Sprintf("获取更新失败: %v", err))
			cancel()
			time.Sleep(5 * time.Second)
			continue
		}

		printlnUTF8(fmt.Sprintf("获取更新成功，状态码: %d", resp.StatusCode))

		// 读取响应体
		responseBody, err := io.ReadAll(resp.Body)
		// 关闭响应体
		resp.Body.Close()
		// 取消上下文
		cancel()

		if err != nil {
			printlnUTF8(fmt.Sprintf("读取响应体失败: %v", err))
			time.Sleep(5 * time.Second)
			continue
		}

		printlnUTF8(fmt.Sprintf("Telegram响应长度: %d", len(responseBody)))

		// 解析响应
		var result struct {
			Ok     bool `json:"ok"`
			Result []struct {
				UpdateID int64 `json:"update_id"`
				Message  struct {
					From struct {
						ID        int64  `json:"id"`
						Username  string `json:"username"`
						FirstName string `json:"first_name"`
					} `json:"from"`
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
				profile := TelegramProfile{
					ID:        chatID,
					Username:  update.Message.From.Username,
					FirstName: update.Message.From.FirstName,
				}

				// 存储chat_id
				rememberChatID(chatID)
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
				printlnUTF8(fmt.Sprintf("原始文本: '%s'", sanitizeTelegramText(text)))
				printlnUTF8(fmt.Sprintf("去除空格后: '%s'", trimmedText))

				processTelegramCommand(chatID, text, profile)
			}
		}

		// 短暂休眠，避免请求过于频繁
		time.Sleep(1 * time.Second)
	}
}
