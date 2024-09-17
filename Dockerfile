# Usa una imagen base de Golang versión 1.20.4
FROM golang:1.20.4-alpine

# Establece el directorio de trabajo
WORKDIR /app

# Copia los archivos necesarios al contenedor
COPY go.mod ./
COPY go.sum ./
RUN go mod download

# Copia el código fuente
COPY . .

# Compila el binario desde el archivo bot.go
RUN go build -o /app/bot bot.go

# Establece permisos de ejecución
RUN chmod +x /app/bot

# Ejecuta el bot de trading
CMD ["/app/bot"]
