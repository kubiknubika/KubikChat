# 1. Билд-стадия (Собираем Go в бинарник)
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
# Скачиваем зависимости и компилируем
RUN go mod download
RUN go build -o kubikchat .

# 2. Финальная стадия (Маленький образ для запуска)
FROM alpine:latest
WORKDIR /root/
# Копируем бинарник из первой стадии
COPY --from=builder /app/kubikchat .
# Копируем папку с HTML
COPY --from=builder /app/public ./public

# Открываем порт
EXPOSE 8080

# Запускаем
CMD ["./kubikchat"]