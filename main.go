package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func main() {
	// Создаем папку для загрузок, если нет
	os.MkdirAll(UploadDir, os.ModePerm)

	fs := http.FileServer(http.Dir("./public"))
	http.Handle("/", fs)
	
	// WebSocket
	http.HandleFunc("/ws", handleConnections)
	
	// File Upload Handler
	http.HandleFunc("/upload", uploadHandler)

	initCommands()
	go StartMathGame()

	fmt.Println("🚀 KubikChat v8.0 (File Uploads) started on :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		panic(err)
	}
}

// Обработчик загрузки файлов
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Ограничение размера
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadSize)
	if err := r.ParseMultipartForm(MaxUploadSize); err != nil {
		http.Error(w, "File too big", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Invalid file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Генерируем уникальное имя (timestamp + original name)
	// В реальном проде лучше использовать UUID, но для нас это ок
	ext := filepath.Ext(header.Filename)
	uniqueName := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	dstPath := filepath.Join(UploadDir, uniqueName)

	// Создаем файл на диске
	dst, err := os.Create(dstPath)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	// Копируем данные
	if _, err := io.Copy(dst, file); err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	// Возвращаем JSON с ссылкой
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"url": "/uploads/" + uniqueName,
	})
}