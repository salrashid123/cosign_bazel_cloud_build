FROM golang:1.19 as build

WORKDIR /go/src/app
COPY . .

RUN go mod download

RUN GOOS=linux GOARCH=amd64 go build -o /go/bin/server

FROM gcr.io/distroless/base@sha256:75f63d4edd703030d4312dc7528a349ca34d48bec7bd754652b2d47e5a0b7873

COPY --from=build /go/bin/server /
EXPOSE 8080
CMD ["/server"]
