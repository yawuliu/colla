package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/labstack/gommon/log"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/glebarez/sqlite"
	_ "github.com/joho/godotenv/autoload"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	endpoint = "https://api.moonshot.cn/v1/chat/completions"
	model    = "moonshot-v1-8k"
	dbFile   = "chat_wails.db"
)

type Dialog struct {
	ID        uint      `gorm:"primarykey"`
	Title     string    `gorm:"index"`
	CreatedAt time.Time `gorm:"index"`
}

type ToolDescQuery struct {
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
}

type ToolDescProperties struct {
	Query ToolDescQuery `json:"query,omitempty"`
}

type ToolDescParameters struct {
	Type       string             `json:"type,omitempty"`
	Properties ToolDescProperties `json:"properties,omitempty"`
	Required   []string           `json:"required,omitempty"`
}

type ToolDescFunction struct {
	Name        string             `json:"name,omitempty"`
	Description string             `json:"description,omitempty"`
	Parameters  ToolDescParameters `json:"parameters,omitempty"`
}

type ToolDesc struct {
	Type     string           `json:"type,omitempty"`
	Function ToolDescFunction `json:"function,omitempty"`
}

type chatReq struct {
	Model    string     `json:"model,omitempty"`
	Messages []Message  `json:"messages,omitempty"`
	Tools    []ToolDesc `json:"tools,omitempty"`
}

type SendResp struct {
	NewDid  int    `json:"newDid,omitempty"`
	Reply   string `json:"reply,omitempty"`
	ErrCode int    `json:"errcode"`
}

type ToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

type MessageViewItem struct {
	ID       uint   `json:"id,omitempty" gorm:"primarykey"`
	DialogID uint   `json:"dialog_id,omitempty" gorm:"index"`
	Role     string `json:"role,omitempty"` // user / assistant
	Content  string `json:"content,omitempty"`
	// dto
	ToolCalls  []byte `json:"tool_calls,omitempty"`
	ToolCallId string `json:"tool_call_id,omitempty"`
	Name       string `json:"name,omitempty"`
}

type Message struct {
	ID       uint   `json:"_" gorm:"primarykey"`
	DialogID uint   `json:"_" gorm:"index"`
	Role     string `json:"role,omitempty"` // user / assistant
	Content  string `json:"content,omitempty"`
	// dto
	ToolCalls  datatypes.JSON `json:"tool_calls,omitempty"`
	ToolCallId string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
}

//	type msgDTO struct {
//		Role       string     `json:"role,omitempty"`
//		Content    string     `json:"content,omitempty"`
//		ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
//		ToolCallId string     `json:"tool_call_id,omitempty"`
//		Name       string     `json:"name,omitempty"`
//	}
/*
{
  "id" : "chatcmpl-688873601e91f778fa91efbd",
  "object" : "chat.completion",
  "created" : 1753772896,
  "model" : "moonshot-v1-8k",
  "choices" : [ {
    "index" : 0,
    "message" : {
      "role" : "assistant",
      "content" : "",
      "tool_calls" : [ {
        "index" : 0,
        "id" : "search:0",
        "type" : "function",
        "function" : {
          "name" : "search",
          "arguments" : "{\n  \"query\": \"Golang 1.23 新特性\"\n}"
        }
      } ]
    },
    "finish_reason" : "tool_calls"
  } ],
  "usage" : {
    "prompt_tokens" : 74,
    "completion_tokens" : 27,
    "total_tokens" : 101
  }
}
*/

type chatResp struct {
	Id      string `json:"id"`
	Object  string `json:"object"`
	Created int    `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int            `json:"index"`
		Message      Message        `json:"message,omitempty"`
		Delta        map[string]any `json:"delta,omitempty"` // 流式用，这里忽略
		FinishReason string         `json:"finish_reason,omitempty"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// App struct
type App struct {
	ctx context.Context
	db  *gorm.DB
	//
	allocatorCtx    context.Context
	allocatorCancel context.CancelFunc
	//
	browserCtx    context.Context
	browserCancel context.CancelFunc
}

// NewApp creates a new App application struct
// NewApp 创建一个新的 App 应用程序
func NewApp() *App {
	db, err := gorm.Open(sqlite.Open(dbFile), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	db.AutoMigrate(&Dialog{}, &Message{})
	//
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.DisableGPU,
		chromedp.Flag("enable-automation", false),
		// chromedp.UserDataDir(userdata),
	)
	allocatorCtx, allocatorCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	browserCtx, browserCancel := chromedp.NewContext(allocatorCtx)
	// it depends how will you manage the lifecycle of the browser, maybe you don't want to call browserCancel() here
	err = chromedp.Run(browserCtx, chromedp.ActionFunc(func(cxt context.Context) error {
		_, err := page.AddScriptToEvaluateOnNewDocument("Object.defineProperty(navigator, 'webdriver', { get: () => false, });").Do(cxt)
		if err != nil {
			return err
		}
		return nil
	}))
	if err != nil {
		fmt.Println(err)
	}
	//
	return &App{db: db, allocatorCtx: allocatorCtx, allocatorCancel: allocatorCancel,
		browserCtx: browserCtx, browserCancel: browserCancel}
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

func (a *App) GetMessages(did uint) []MessageViewItem {
	var ms []Message
	a.db.Where("dialog_id = ?", did).Order("id asc").Find(&ms)
	var ret []MessageViewItem
	for _, m := range ms {
		ret = append(ret, MessageViewItem{
			m.ID,
			m.DialogID,
			m.Role,
			m.Content,
			m.ToolCalls,
			m.ToolCallId,
			m.Name,
		})
	}
	return ret
}

func (a *App) doRequest(dto []Message, toolDescs []ToolDesc) (*chatResp, error) {
	body, _ := json.Marshal(chatReq{
		Model:    model,
		Messages: dto,
		Tools:    toolDescs,
	})
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("网络错误")
	}
	req.Header.Set("Authorization", "Bearer "+os.Getenv("API_KEY"))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		buf, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Error(err)
		}
		return nil, fmt.Errorf("code=%d,resp=%s", resp.StatusCode, string(buf))
	}
	defer resp.Body.Close()

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var cr chatResp
	err = json.Unmarshal(buf, &cr)
	if err != nil {
		return nil, err
	}
	if len(cr.Choices) == 0 {
		return nil, errors.New("无回复")
	}
	return &cr, nil
}

func (a *App) SendMessage(did int, content string) SendResp {
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		return SendResp{did, "API_KEY配置缺失", 1}
	}
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
	//var dtos []msgDTO
	//for _, m := range msgs {
	//	if m.MsgDTO != "" {
	//		var dto msgDTO
	//		err := json.Unmarshal([]byte(m.MsgDTO), &dto)
	//		if err != nil {
	//			log.Error(err)
	//			return SendResp{did, err.Error(), 1}
	//		}
	//		dtos = append(dtos, dto)
	//	} else {
	//		dtos = append(dtos, msgDTO{Role: m.Role, Content: m.Content})
	//	}
	//}
	var toolDescs []ToolDesc
	toolDescs = append(toolDescs, ToolDesc{
		Type: "function",
		Function: ToolDescFunction{Name: "search",
			Description: "联网搜索，返回与查询最相关的网页摘要。",
			Parameters: ToolDescParameters{
				Type: "object",
				Properties: ToolDescProperties{
					Query: ToolDescQuery{
						Type:        "string",
						Description: "需要搜索的关键词",
					},
				},
				Required: []string{"query"},
			}},
	})
	reply := ""
	loop := true
	for loop {
		cr, err := a.doRequest(msgs, toolDescs)
		if err != nil {
			log.Error(err)
			return SendResp{did, err.Error(), 1}
		}
		choice := cr.Choices[0]
		msg := choice.Message
		msgs = append(msgs, msg)
		switch choice.FinishReason {
		case "stop":
			// 正常结束，把最终回复打印给用户
			reply = msg.Content
			loop = false
			// msgs = append(msgs, Message{Role: "assistant", Content: reply})
			break // 跳出for循环
		case "tool_calls":
			// 4.2 解析 tool_calls
			var calls []ToolCall
			err = json.Unmarshal(msg.ToolCalls, &calls)
			if err != nil {
				log.Errorf("err:%v, content: %s\n", err, msg.ToolCalls)
				return SendResp{did, err.Error(), 1}
			}
			fmt.Printf("ToolCalls: %#v\n", calls)
			// 4.3 依次执行工具
			for _, call := range calls {
				if call.Function.Name != "search" {
					continue
				}
				var args struct {
					Query string `json:"query"`
				}
				_ = json.Unmarshal([]byte(call.Function.Arguments), &args)
				fmt.Printf("\n>>> 正在执行工具 search(%q)...\n", args.Query)
				result, err := searchTool(a.browserCtx, args.Query)
				if err != nil {
					result = "工具执行失败：" + err.Error()
				}
				// 4.4 把工具返回追加进 messages
				//messages = append(messages, map[string]any{
				//	"role":         "tool",
				//	"tool_call_id": call.ID,
				//	"name":         "search",
				//	"content":      result,
				//})
				msgs = append(msgs, Message{Role: "tool", ToolCallId: call.ID, Name: "search", Content: result})
			}
		default:
			loop = false
			return SendResp{did,
				fmt.Sprintf("unknown finish_reason:%s", choice.FinishReason),
				1}
		}
	}
	//msgs = append(msgs, Message{Role: "assistant", Content: reply})
	// 持久化：只有 ≥1 轮才落库
	if did <= 0 {
		title := titleOf(content)
		d := Dialog{Title: title}
		a.db.Create(&d)
		did = int(d.ID)
	}
	for _, m := range msgs {
		if m.ID == 0 {
			m.DialogID = uint(did)
			a.db.Create(&m)
		}
	}
	return SendResp{did, reply, 0}
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
	a.allocatorCancel()
	a.browserCancel()
}
