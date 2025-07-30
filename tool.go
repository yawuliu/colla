package main

import (
	"context"
	"fmt"
	"golang.org/x/time/rate"
	"kimi-chat/googlesearch"
	"strings"
)

// ------------------ 1. 定义工具 ------------------

/*
工具 schema：让模型知道我们有一个叫 search 的函数，可联网搜索。
*/
var tools = []map[string]any{
	{
		"type": "function",
		"function": map[string]any{
			"name":        "search",
			"description": "联网搜索，返回与查询最相关的网页摘要。",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "需要搜索的关键词",
					},
				},
				"required": []string{"query"},
			},
		},
	},
}

/*
实际执行搜索的地方。这里为了演示简单，直接调 Bing 的公开接口，不需要 key。
你可以换成自己的内部搜索、数据库查询等。
*/
func searchTool(ctx context.Context, query string) (string, error) {
	googlesearch.RateLimit = rate.NewLimiter(1, 1)
	opt := googlesearch.SearchOptions{CountryCode: "hk", LanguageCode: "en", Limit: 10, Start: 0, OverLimit: false, FollowNextPage: false,
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36"}
	serp, err := googlesearch.Search(ctx, query, opt)
	if err != nil {
		panic(err)
	}
	var lines []string
	for _, result := range serp {
		lines = append(lines, fmt.Sprintf("标题: %s\n摘要: %s\n链接: %s\n\n", result.Title, result.Description, result.URL))
	}
	if len(lines) == 0 {
		return "未找到相关内容", nil
	}
	return strings.Join(lines, "\n"), nil
}
