package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
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
	outputPath := flag.String("o", "", "将结果输出到指定文件") // 👈 找回 -o 参数
	filterCodesStr := flag.String("fc", "404", "过滤的状态码，用逗号分隔，例如: 404,500")
	flag.Parse()

	// 👈 解析 -fc 参数
	filterCodes := make(map[int]bool)
	if *filterCodesStr != "" {
		for _, codeStr := range strings.Split(*filterCodesStr, ",") {
			code, err := strconv.Atoi(strings.TrimSpace(codeStr))
			if err == nil {
				filterCodes[code] = true
			}
		}
	}

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

	// 👈 找回 -o 文件输出逻辑
	var resultFile *os.File
	var fileMutex sync.Mutex
	if *outputPath != "" {
		var err error
		resultFile, err = os.Create(*outputPath)
		if err != nil {
			fmt.Printf("创建输出文件失败: %v\n", err)
			return
		}
		defer resultFile.Close()
	}

	client := &http.Client{
		Timeout: 3 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	jobs := make(chan string, *concurrency)
	var wg sync.WaitGroup

	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				// 注意这里传的是 &fileMutex (指针)
				scanPath(client, *targetURL, path, resultFile, &fileMutex, filterCodes)
			}
		}()
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		path := scanner.Text()
		path = strings.TrimFunc(path, func(r rune) bool {
			return !unicode.IsPrint(r)
		})
		path = strings.TrimSpace(path)

		if path != "" {
			atomic.AddInt32(&totalCount, 1)
			jobs <- path
		}
	}

	close(jobs)
	wg.Wait()

	fmt.Printf("\n[*] 扫描结束！\n")
	fmt.Printf("[*] 总请求数: %d\n", totalCount)
	fmt.Printf("[+] 发现有效路径: %d\n", successCount)
	fmt.Printf("[-] 请求失败/超时: %d\n", failCount)
}

func scanPath(client *http.Client, targetURL string, path string, resultFile *os.File, fileMutex *sync.Mutex, filterCodes map[int]bool) {
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
		atomic.AddInt32(&failCount, 1)
		return
	}
	defer resp.Body.Close()

	bodyLen, _ := io.Copy(io.Discard, resp.Body)

	switch resp.StatusCode {
	case 200, 301, 302, 403:
		// 👈 找回过滤逻辑：如果命中过滤名单，直接跳过
		if filterCodes[resp.StatusCode] {
			return
		}

		atomic.AddInt32(&successCount, 1)
		resultMsg := fmt.Sprintf("[+] 发现路径: %-40s | 状态码: %d | 长度: %d\n", fullURL, resp.StatusCode, bodyLen)

		// 打印到屏幕
		fmt.Print(resultMsg)

		// 👈 找回写文件逻辑
		if resultFile != nil {
			fileMutex.Lock()
			resultFile.WriteString(resultMsg)
			fileMutex.Unlock()
		}
	}
}
