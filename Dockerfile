# syntax=docker/dockerfile:1
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/ohey-api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/ohey-notification-worker ./cmd/notification_worker

FROM alpine:3.21
RUN adduser -D -H ohey
USER ohey
COPY --from=build /out/ohey-api /ohey-api
COPY --from=build /out/ohey-notification-worker /ohey-notification-worker
EXPOSE 8080
CMD ["/ohey-api"]
