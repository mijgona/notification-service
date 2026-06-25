FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api && \
    CGO_ENABLED=0 go build -o /out/relay ./cmd/relay && \
    CGO_ENABLED=0 go build -o /out/worker ./cmd/worker

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/api /bin/api
COPY --from=build /out/relay /bin/relay
COPY --from=build /out/worker /bin/worker
CMD ["/bin/api"]
