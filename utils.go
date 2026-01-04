package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand" // Криптографический рандом (для ключей)
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	mathrand "math/rand" // <--- ПСЕВДОНИМ (для игр)
	"os"
	"time"
	"unicode"
)

// Валидация ника
func validateNickname(n string) string {
	if len(n) < 3 || len(n) > 15 { return "Len 3-15" }
	for _, r := range n {
		if r > 127 { return "ASCII only" }
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) { return "Alphanumeric only" }
	}
	return ""
}

// Эффект глюка
func glitchText(text string) string {
	chars := []rune("¥Ø∑µ∂∆πΩ≈ç√∫≤≥÷åß∂ƒ©˙∆˚¬…æ")
	runes := []rune(text)
	n := len(runes)
	for i := 0; i < n/2; i++ {
		idx := mathrand.Intn(n) // Используем mathrand
		if runes[idx] != ' ' {
			runes[idx] = chars[mathrand.Intn(len(chars))]
		}
	}
	return string(runes)
}

// Криптография
func encrypt(data []byte) []byte {
	block, _ := aes.NewCipher(EncryptionKey)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	io.ReadFull(rand.Reader, nonce) // Используем crypto/rand
	return gcm.Seal(nonce, nonce, data, nil)
}

func decrypt(data []byte) ([]byte, error) {
	block, _ := aes.NewCipher(EncryptionKey)
	gcm, _ := cipher.NewGCM(block)
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize { return nil, fmt.Errorf("err") }
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func checkPassword(p string) bool {
	h := sha256.Sum256([]byte(p))
	return hex.EncodeToString(h[:]) == AdminPasswordHash
}

// Сохранение данных
func persistenceWorker() {
	for range saveQueue {
		time.Sleep(1 * time.Second)
		globalMutex.Lock()
		d, _ := json.MarshalIndent(userDatabase, "", "  ")
		globalMutex.Unlock()
		_ = ioutil.WriteFile(DbFile, encrypt(d), 0644)
		for len(saveQueue) > 0 { <-saveQueue }
	}
}

func triggerSave() {
	select { case saveQueue <- struct{}{}: default: }
}

func loadData() {
	if _, e := os.Stat(DbFile); os.IsNotExist(e) { return }
	d, _ := ioutil.ReadFile(DbFile)
	dec, err := decrypt(d)
	if err == nil {
		json.Unmarshal(dec, &userDatabase)
		fmt.Println("📂 Database loaded.")
	}
}