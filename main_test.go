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
	t.Cleanup(func() {
		telegramMessageSender = defaultSendTelegramMessage
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
