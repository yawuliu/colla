package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const (
	apiKey   = "YOUR_API_KEY"
	endpoint = "https://api.moonshot.cn/v1/chat/completions"
	model    = "moonshot-v1-8k"
	dbFile   = "chat_wails.db"
)

type Dialog struct {
	ID        uint      `gorm:"primarykey"`
	Title     string    `gorm:"index"`
	CreatedAt time.Time `gorm:"index"`
}

type Message struct {
	ID       uint   `gorm:"primarykey"`
	DialogID uint   `gorm:"index"`
	Role     string // user / assistant
	Content  string
}

type chatReq struct {
	Model    string   `json:"model"`
	Messages []msgDTO `json:"messages"`
}

type SendResp struct {
	NewDid int    `json:"newDid"`
	Reply  string `json:"reply"`
}

type msgDTO struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type chatResp struct {
	Choices []struct {
		Message msgDTO `json:"message"`
	} `json:"choices"`
}

// App struct
type App struct {
	ctx context.Context
	db  *gorm.DB
}

// NewApp creates a new App application struct
// NewApp 创建一个新的 App 应用程序
func NewApp() *App {
	db, err := gorm.Open(sqlite.Open(dbFile), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	db.AutoMigrate(&Dialog{}, &Message{})
	return &App{db: db}
}

// startup is called at application startup
// startup 在应用程序启动时调用
func (a *App) startup(ctx context.Context) {
	// Perform your setup here
	// 在这里执行初始化设置
	a.ctx = ctx
}

/* API 供前端调用 */

func (a *App) GetDialogs() []Dialog {
	var ds []Dialog
	a.db.Order("created_at desc").Find(&ds)
	return ds
}

func (a *App) GetMessages(did uint) []Message {
	var ms []Message
	a.db.Where("dialog_id = ?", did).Order("id asc").Find(&ms)
	return ms
}

func (a *App) SendMessage(did int, content string) SendResp {
	// 内存里追加
	if did <= 0 {
		did = 0
	}
	// 加载已有消息
	var msgs []Message
	if did > 0 {
		a.db.Where("dialog_id = ?", did).Order("id asc").Find(&msgs)
	}
	msgs = append(msgs, Message{Role: "user", Content: content})

	// 请求 AI
	var dto []msgDTO
	for _, m := range msgs {
		dto = append(dto, msgDTO{Role: m.Role, Content: m.Content})
	}
	body, _ := json.Marshal(chatReq{Model: model, Messages: dto})
	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != 200 {
		return SendResp{int(did), "网络错误"}
	}
	defer resp.Body.Close()
	var cr chatResp
	json.NewDecoder(resp.Body).Decode(&cr)
	if len(cr.Choices) == 0 {
		return SendResp{int(did), "无回复"}
	}
	reply := cr.Choices[0].Message.Content
	msgs = append(msgs, Message{Role: "assistant", Content: reply})

	// 持久化：只有 ≥1 轮才落库
	if did <= 0 {
		title := titleOf(content)
		d := Dialog{Title: title}
		a.db.Create(&d)
		did = int(d.ID)
	}
	for _, m := range msgs {
		m.DialogID = uint(did)
		a.db.Create(&m)
	}
	return SendResp{did, reply}
}

func (a *App) DeleteDialog(id uint) {
	a.db.Delete(&Dialog{}, id)
	a.db.Where("dialog_id = ?", id).Delete(&Message{})
}

func titleOf(s string) string {
	r := []rune(s)
	if len(r) > 15 {
		return string(r[:15]) + "…"
	}
	return string(r)
}

// domReady is called after the front-end dom has been loaded
// domReady 在前端Dom加载完毕后调用
func (a *App) domReady(ctx context.Context) {
	// Add your action here
	// 在这里添加你的操作
}

// beforeClose is called when the application is about to quit,
// either by clicking the window close button or calling runtime.Quit.
// Returning true will cause the application to continue,
// false will continue shutdown as normal.
// beforeClose在单击窗口关闭按钮或调用runtime.Quit即将退出应用程序时被调用.
// 返回 true 将导致应用程序继续，false 将继续正常关闭。
func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	return false
}

// shutdown is called at application termination
// 在应用程序终止时被调用
func (a *App) shutdown(ctx context.Context) {
	// Perform your teardown here
	// 在此处做一些资源释放的操作
}
