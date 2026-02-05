FROM golang:1.24.13 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/bin/hvac ./examples/hvac

FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache ca-certificates wget
COPY --from=build /app/bin/hvac /app/hvac
COPY --from=build /app/examples/hvac /app/examples/hvac
EXPOSE 8080
ENTRYPOINT ["/app/hvac"]
CMD ["--config", "/app/examples/hvac/config.yaml"]
