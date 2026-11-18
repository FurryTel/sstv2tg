FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags=-s -o /out/sstv2tg ./cmd/sstv2tg

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/sstv2tg /sstv2tg
EXPOSE 5060/udp 16384-16484/udp
ENTRYPOINT ["/sstv2tg"]
