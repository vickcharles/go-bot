FROM golang:1.20.4-alpine

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .

RUN go build -o /app/bot bot.go

RUN chmod +x /app/bot

CMD ["/app/bot"]
