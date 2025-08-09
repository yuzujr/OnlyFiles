package main

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	code := genCode(8) // 生成 8 位随机码
	fmt.Println(code)

	// 文件名
	fileName := code + ".code"
	filePath := filepath.Join(".", fileName)

	// 写入文件
	err := os.WriteFile(filePath, []byte(code+"\n"), 0644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "写入验证码文件失败:", err)
		os.Exit(1)
	}
}

// genCode 生成长度为 n 的随机字母数字验证码
func genCode(n int) string {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	for i, b := range bytes {
		bytes[i] = letters[int(b)%len(letters)]
	}
	return string(bytes)
}
