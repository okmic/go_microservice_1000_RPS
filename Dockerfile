FROM golang:1.25-alpine

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN ls -la

RUN go build -v -o wallet-service cmd/main.go

EXPOSE 9999

CMD ["./wallet-service"]