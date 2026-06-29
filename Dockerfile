# syntax=docker/dockerfile:1
FROM golang:1.26.4-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/ohey-api ./cmd/api

FROM alpine:3.21
RUN adduser -D -H ohey
USER ohey
COPY --from=build /out/ohey-api /ohey-api
EXPOSE 8080
CMD ["/ohey-api"]
