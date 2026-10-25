package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
)

var (
	successCount int32 // 扫描到的有效路径数
	failCount    int32 // 请求失败/超时数
	totalCount   int32 // 总任务数
)

func main() {
	targetURL := flag.String("u", "", "目标URL (例如: http://example.com)")
	dictPath := flag.String("w", "dict.txt", "字典文件路径")
	concurrency := flag.Int("c", 50, "并发数 (默认50)")
	flag.Parse()

	if *targetURL == "" {
		fmt.Println("请使用 -u 参数指定目标URL，例如: go run main.go -u http://example.com")
		return
	}

	file, err := os.Open(*dictPath)
	if err != nil {
		fmt.Printf("打开字典文件失败: %v\n", err)
		return
	}
	defer file.Close()

	fmt.Printf("[*] 开始对 %s 进行目录爆破 (并发数: %d)...\n", *targetURL, *concurrency)

	client := &http.Client{
		Timeout: 3 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // 不跟随重定向，保留原始 30x
		},
	}

	jobs := make(chan string, *concurrency)
	var wg sync.WaitGroup

	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				scanPath(client, *targetURL, path)
			}
		}()
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		path := scanner.Text()

		// 过滤不可见字符（解决你之前的 UTF-16 报错）
		path = strings.TrimFunc(path, func(r rune) bool {
			return !unicode.IsPrint(r)
		})
		path = strings.TrimSpace(path)

		if path != "" {
			atomic.AddInt32(&totalCount, 1) // 计数器+1
			jobs <- path
		}
	}

	close(jobs)
	wg.Wait()

	// 汇总统计输出
	fmt.Printf("\n[*] 扫描结束！\n")
	fmt.Printf("[*] 总请求数: %d\n", totalCount)
	fmt.Printf("[+] 发现有效路径: %d\n", successCount)
	fmt.Printf("[-] 请求失败/超时: %d\n", failCount)
}

func scanPath(client *http.Client, targetURL string, path string) {
	// P1 修复：处理斜杠拼接问题
	baseURL := strings.TrimRight(targetURL, "/")
	cleanPath := strings.TrimLeft(path, "/")
	fullURL := baseURL + "/" + cleanPath

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		atomic.AddInt32(&failCount, 1)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	resp, err := client.Do(req)
	if err != nil {
		atomic.AddInt32(&failCount, 1) // 记录失败次数
		return
	}
	defer resp.Body.Close()

	// P1 修复：实测长度
	bodyLen, _ := io.Copy(io.Discard, resp.Body)

	switch resp.StatusCode {
	case 200, 301, 302, 403:
		atomic.AddInt32(&successCount, 1)
		fmt.Printf("[+] 发现路径: %-40s | 状态码: %d | 长度: %d\n", fullURL, resp.StatusCode, bodyLen)
	}
}
