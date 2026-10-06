FROM golang:1.27-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ecommerce-api . \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ecommerce-seed ./cmd/seed

FROM alpine:3.22

RUN addgroup -S app && adduser -S -G app app \
    && mkdir -p /data \
    && chown app:app /data
COPY --from=build /out/ecommerce-api /usr/local/bin/ecommerce-api
COPY --from=build /out/ecommerce-seed /usr/local/bin/ecommerce-seed

ENV PORT=8080 \
    DB_PATH=/data/ecommerce.db
EXPOSE 8080
USER app
ENTRYPOINT ["/usr/local/bin/ecommerce-api"]
