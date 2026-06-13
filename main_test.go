package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func resetTestState(t *testing.T) {
	t.Helper()
	beijingLoc = time.Local
	users = nil
	chatIds = make(map[int64]bool)
	loggedTokens = make(map[int64]string)
	adminChatIds = make(map[int64]bool)
	authUsers = make(map[int64]AuthUser)
	userStates = make(map[int64]UserState)
	config = Config{
		Port:                8081,
		ApiBaseURL:          "https://emos.best",
		PublicBaseURL:       "http://127.0.0.1:8081",
		EmosProviderBot:     "emospg_bot",
		EmosUserID:          "e0E446ZE6s",
		TelegramBotUsername: "emosCeshi_bot",
		TelegramApiURL:      "https://api.telegram.org/bot",
		DataFile:            filepath.Join(t.TempDir(), "data.json"),
	}
	telegramMessageSender = func(chatID int64, text string) {}
	telegramKeyboardSender = func(chatID int64, text string, keyboard [][]InlineKeyboardButton) {}
	telegramCallbackAnswerer = func(callbackID string, text string) {}
	t.Cleanup(func() {
		telegramMessageSender = defaultSendTelegramMessage
		telegramKeyboardSender = defaultSendTelegramMessageWithInlineKeyboard
		telegramCallbackAnswerer = defaultAnswerTelegramCallbackQuery
	})
}

func TestLoginRedirectsToEMOSProviderBot(t *testing.T) {
	resetTestState(t)

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()

	serveTelegramLoginPage(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	want := "https://t.me/emospg_bot?start=link_e0E446ZE6s-emosCeshi_bot"
	if got := rec.Header().Get("Location"); got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

func TestStartAgreeVerifiesTokenFetchesUserAndSavesAuth(t *testing.T) {
	resetTestState(t)

	var checkCalled, userCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token_xxx" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("Accept = %q", got)
		}
		switch r.URL.Path {
		case "/api/sign/check":
			checkCalled = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/user":
			userCalled = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"user_id":"emos-1","username":"alice","avatar_url":"https://img.test/a.png"}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	config.ApiBaseURL = server.URL

	profile := TelegramProfile{ID: 12345, Username: "tg_user", FirstName: "TG"}
	processTelegramCommand(12345, "/start emosLinkAgree-token_xxx", profile)

	if !checkCalled {
		t.Fatal("/api/sign/check was not called")
	}
	if !userCalled {
		t.Fatal("/api/user was not called")
	}

	authUser, ok := authUsers[12345]
	if !ok {
		t.Fatal("auth user was not saved")
	}
	if authUser.AuthToken != "token_xxx" {
		t.Fatalf("AuthToken = %q", authUser.AuthToken)
	}
	if authUser.EMOSID != "emos-1" {
		t.Fatalf("EMOSID = %q", authUser.EMOSID)
	}
	if authUser.EMOSUsername != "alice" {
		t.Fatalf("EMOSUsername = %q", authUser.EMOSUsername)
	}
	if authUser.Avatar != "https://img.test/a.png" {
		t.Fatalf("Avatar = %q", authUser.Avatar)
	}
	if authUser.AuthStatus != "agreed" {
		t.Fatalf("AuthStatus = %q", authUser.AuthStatus)
	}

	data, err := os.ReadFile(config.DataFile)
	if err != nil {
		t.Fatal(err)
	}
	var stored DataStorage
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.AuthUsers[12345].AuthToken != "token_xxx" {
		t.Fatal("auth token was not persisted")
	}
}

func TestLoggedInAccountDoesNotPromptLogin(t *testing.T) {
	resetTestState(t)

	authUsers[12345] = AuthUser{
		TelegramID:   12345,
		EMOSID:       "emos-1",
		EMOSUsername: "alice",
		AuthToken:    "token_xxx",
		AuthStatus:   "agreed",
	}

	var sent string
	telegramMessageSender = func(chatID int64, text string) {
		sent = text
	}

	handleAccountCommand(12345)

	if strings.Contains(sent, "授权登录") {
		t.Fatalf("account prompted login: %q", sent)
	}
	if !strings.Contains(sent, "alice") {
		t.Fatalf("account response missing username: %q", sent)
	}
}

func TestStartRefuseSavesRefusedStatus(t *testing.T) {
	resetTestState(t)

	profile := TelegramProfile{ID: 12345, Username: "tg_user", FirstName: "TG"}
	processTelegramCommand(12345, "/start emosLinkRefuse-12345", profile)

	authUser, ok := authUsers[12345]
	if !ok {
		t.Fatal("auth user was not saved")
	}
	if authUser.AuthStatus != "refused" {
		t.Fatalf("AuthStatus = %q", authUser.AuthStatus)
	}
	if authUser.TelegramUsername != "tg_user" {
		t.Fatalf("TelegramUsername = %q", authUser.TelegramUsername)
	}
}

func TestScheduledTimeAllowsDelayWithinSameMinute(t *testing.T) {
	now := time.Date(2026, 6, 9, 3, 34, 27, 0, time.Local)
	hour, minute, second, err := parseScheduleTime("3:34:00")
	if err != nil {
		t.Fatal(err)
	}

	if !shouldRunScheduledTime(now, hour, minute, second, "") {
		t.Fatal("expected delayed scheduler tick in the same minute to run")
	}
}

func TestScheduledTimeSkipsAlreadyCheckedToday(t *testing.T) {
	now := time.Date(2026, 6, 9, 3, 34, 27, 0, time.Local)
	hour, minute, second, err := parseScheduleTime("03:34:00")
	if err != nil {
		t.Fatal(err)
	}

	if shouldRunScheduledTime(now, hour, minute, second, "2026-06-09") {
		t.Fatal("expected already checked user to be skipped")
	}
}

func TestNormalizeSignContentLimitsToTenCharacters(t *testing.T) {
	content := normalizeSignContent("never give up")

	if len([]rune(content)) > 10 {
		t.Fatalf("content length = %d, want <= 10", len([]rune(content)))
	}
	if content == "" {
		t.Fatal("content should not be empty")
	}
}

func TestPickSignContentSkipsLongMessages(t *testing.T) {
	for i := 0; i < 20; i++ {
		content := pickSignContent([]string{"never give up", "let's do this", "签到"})
		if content != "签到" {
			t.Fatalf("content = %q, want only the valid short message", content)
		}
	}
}

func TestDreamSignMessagesAreValidShortMessages(t *testing.T) {
	if len(dreamSignMessages) < 200 {
		t.Fatalf("dream sign messages count = %d, want >= 200", len(dreamSignMessages))
	}
	for _, content := range dreamSignMessages {
		if !isValidSignContent(content) {
			t.Fatalf("dream sign message %q is invalid, length=%d", content, len([]rune(content)))
		}
	}
	for i := 0; i < 50; i++ {
		content := randomDreamSignMessage()
		if !isValidSignContent(content) {
			t.Fatalf("random dream sign message %q is invalid", content)
		}
	}
}

func TestApplyTokenOwnersFromDataOverwritesChatID(t *testing.T) {
	resetTestState(t)

	users = []User{{Token: "token_xxx", Username: "alice", ChatID: 111}}
	loggedTokens[333] = "token_xxx"
	authUsers[111] = AuthUser{
		TelegramID:   111,
		EMOSUsername: "alice",
		AuthToken:    "token_xxx",
		AuthStatus:   "agreed",
		UpdatedAt:    "2026-06-09T03:00:00+08:00",
	}
	authUsers[222] = AuthUser{
		TelegramID:   222,
		EMOSUsername: "alice",
		AuthToken:    "token_xxx",
		AuthStatus:   "agreed",
		UpdatedAt:    "2026-06-09T03:30:00+08:00",
	}

	updated := applyTokenOwnersFromData()

	if updated != 1 {
		t.Fatalf("updated = %d, want 1", updated)
	}
	if users[0].ChatID != 222 {
		t.Fatalf("ChatID = %d, want 222", users[0].ChatID)
	}
	if !chatIds[222] {
		t.Fatal("owner chat_id was not remembered")
	}
}

func TestApplyDefaultOwnerToUnownedUsersUsesSoleAdmin(t *testing.T) {
	resetTestState(t)

	users = []User{
		{Token: "old_token", Username: "old"},
		{Token: "owned_token", Username: "owned", ChatID: 222},
	}
	adminChatIds[7712965941] = true
	adminChatIds[123] = false

	updated := applyDefaultOwnerToUnownedUsers()

	if updated != 1 {
		t.Fatalf("updated = %d, want 1", updated)
	}
	if users[0].ChatID != 7712965941 {
		t.Fatalf("unowned ChatID = %d, want 7712965941", users[0].ChatID)
	}
	if users[1].ChatID != 222 {
		t.Fatalf("owned ChatID changed to %d", users[1].ChatID)
	}
}

func TestAdminListPaginatesFiveUsersPerPage(t *testing.T) {
	resetTestState(t)

	adminChatIds[99] = true
	users = []User{
		{Token: "1001_aa", Username: "user1", ChatID: 1, Time: "01:00:00"},
		{Token: "1002_bb", Username: "user2", ChatID: 2, Time: "02:00:00"},
		{Token: "1003_cc", Username: "user3", ChatID: 3, Time: "03:00:00"},
		{Token: "1004_dd", Username: "user4", ChatID: 4, Time: "04:00:00"},
		{Token: "1005_ee", Username: "user5", ChatID: 5, Time: "05:00:00"},
		{Token: "1006_ff", Username: "user6", ChatID: 6, Time: "06:00:00"},
	}

	var sent string
	var keyboard [][]InlineKeyboardButton
	telegramKeyboardSender = func(chatID int64, text string, inlineKeyboard [][]InlineKeyboardButton) {
		sent = text
		keyboard = inlineKeyboard
	}

	handleListCommand(99, "/list")

	if !strings.Contains(sent, "第 <b>1/2</b> 页") {
		t.Fatalf("page header missing: %q", sent)
	}
	if !strings.Contains(sent, "1. 👤 <b>用户名:</b> <b>user1</b>") || !strings.Contains(sent, "5. 👤 <b>用户名:</b> <b>user5</b>") {
		t.Fatalf("first page missing expected account numbers: %q", sent)
	}
	if strings.Contains(sent, "6. 👤 <b>用户名:</b> <b>user6</b>") {
		t.Fatalf("first page should not include sixth account: %q", sent)
	}
	if strings.Contains(sent, "Token") || strings.Contains(sent, "Owner") || strings.Contains(sent, "1001_aa") {
		t.Fatalf("compact list should not expose token or owner: %q", sent)
	}
	if !strings.Contains(sent, "---------") {
		t.Fatalf("compact separator missing: %q", sent)
	}
	if len(keyboard) != 2 ||
		len(keyboard[0]) != 2 ||
		keyboard[0][0].Text != "·1·" ||
		keyboard[0][1].CallbackData != "list_page:2" ||
		len(keyboard[1]) != 1 ||
		keyboard[1][0].CallbackData != "list_page:2" {
		t.Fatalf("numeric page buttons missing: %#v", keyboard)
	}
}

func TestAdminListSecondPageKeepsGlobalNumbers(t *testing.T) {
	resetTestState(t)

	adminChatIds[99] = true
	users = []User{
		{Token: "1001_aa", Username: "user1", ChatID: 1, Time: "01:00:00"},
		{Token: "1002_bb", Username: "user2", ChatID: 2, Time: "02:00:00"},
		{Token: "1003_cc", Username: "user3", ChatID: 3, Time: "03:00:00"},
		{Token: "1004_dd", Username: "user4", ChatID: 4, Time: "04:00:00"},
		{Token: "1005_ee", Username: "user5", ChatID: 5, Time: "05:00:00"},
		{Token: "1006_ff", Username: "user6", ChatID: 6, Time: "06:00:00"},
	}

	var sent string
	var keyboard [][]InlineKeyboardButton
	telegramKeyboardSender = func(chatID int64, text string, inlineKeyboard [][]InlineKeyboardButton) {
		sent = text
		keyboard = inlineKeyboard
	}

	handleListCommand(99, "/list 2")

	if !strings.Contains(sent, "第 <b>2/2</b> 页") {
		t.Fatalf("page header missing: %q", sent)
	}
	if !strings.Contains(sent, "6. 👤 <b>用户名:</b> <b>user6</b>") {
		t.Fatalf("second page missing global account number 6: %q", sent)
	}
	if strings.Contains(sent, "1. 👤 <b>用户名:</b> <b>user1</b>") {
		t.Fatalf("second page should not include first account: %q", sent)
	}
	if len(keyboard) != 2 ||
		len(keyboard[0]) != 2 ||
		keyboard[0][0].CallbackData != "list_page:1" ||
		keyboard[0][1].Text != "·2·" ||
		len(keyboard[1]) != 1 ||
		keyboard[1][0].CallbackData != "list_page:1" {
		t.Fatalf("numeric page buttons missing: %#v", keyboard)
	}
}

func TestCompactPageItemsUsesEllipsisAroundCurrentPage(t *testing.T) {
	items := compactPageItems(5, 7)
	want := []int{1, 0, 4, 5, 6, 7}

	if len(items) != len(want) {
		t.Fatalf("items = %#v, want %#v", items, want)
	}
	for i := range want {
		if items[i] != want[i] {
			t.Fatalf("items = %#v, want %#v", items, want)
		}
	}
}

func TestListPaginationCallbackSendsRequestedPage(t *testing.T) {
	resetTestState(t)

	adminChatIds[99] = true
	users = []User{
		{Token: "1001_aa", Username: "user1", ChatID: 1, Time: "01:00:00"},
		{Token: "1002_bb", Username: "user2", ChatID: 2, Time: "02:00:00"},
		{Token: "1003_cc", Username: "user3", ChatID: 3, Time: "03:00:00"},
		{Token: "1004_dd", Username: "user4", ChatID: 4, Time: "04:00:00"},
		{Token: "1005_ee", Username: "user5", ChatID: 5, Time: "05:00:00"},
		{Token: "1006_ff", Username: "user6", ChatID: 6, Time: "06:00:00"},
	}

	var sent string
	telegramKeyboardSender = func(chatID int64, text string, inlineKeyboard [][]InlineKeyboardButton) {
		sent = text
	}

	processTelegramCallback(TelegramCallbackQuery{
		ID:   "callback-1",
		From: TelegramUser{ID: 99},
		Message: TelegramMessage{
			Chat: TelegramChat{ID: 99},
		},
		Data: "list_page:2",
	})

	if !strings.Contains(sent, "第 <b>2/2</b> 页") {
		t.Fatalf("callback did not send second page: %q", sent)
	}
	if !strings.Contains(sent, "6. 👤 <b>用户名:</b> <b>user6</b>") {
		t.Fatalf("callback page missing global account number 6: %q", sent)
	}
}

func TestCheckinSuccessNotificationUsesTreeFormat(t *testing.T) {
	resetTestState(t)

	var sent string
	telegramMessageSender = func(chatID int64, text string) {
		sent = text
	}

	sendCheckinNotification(User{Username: "alice", ChatID: 123}, CheckinResult{
		ContinuousDays: 7,
		EarnPoint:      3,
		SignIndex:      21,
	}, "签到成功")

	if !strings.Contains(sent, "🎉 <b>签到成功！</b>") {
		t.Fatalf("success title missing: %q", sent)
	}
	if !strings.Contains(sent, "├ ✅ 签到状态：签到成功") ||
		!strings.Contains(sent, "├ 🌱 累计签到：7 天") ||
		!strings.Contains(sent, "├ 🥕 获得萝卜：3 个") ||
		!strings.Contains(sent, "╰ 🏆 今日排名：21") {
		t.Fatalf("tree details missing: %q", sent)
	}
}
