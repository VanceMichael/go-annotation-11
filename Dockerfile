# 生态环境损害赔偿案件管理平台 —— 多阶段构建
# 同时支持 linux/amd64 与 linux/arm64。
FROM golang:1.22 AS build

ENV GOTOOLCHAIN=local \
    CGO_ENABLED=0

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .

RUN go build -trimpath -ldflags "-s -w" -o /out/ecoctl ./cmd/ecoctl

FROM gcr.io/distroless/static-debian12:nonroot AS runtime

WORKDIR /app

COPY --from=build /out/ecoctl /usr/local/bin/ecoctl

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/ecoctl"]
CMD ["selfcheck"]
