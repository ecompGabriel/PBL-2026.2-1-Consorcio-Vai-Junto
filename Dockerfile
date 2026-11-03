FROM golang:alpine AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/driver-client ./cmd/driver-client
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/passenger-client ./cmd/passenger-client
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/simulator ./cmd/simulator

FROM alpine:latest

WORKDIR /app
COPY --from=build /out/server /app/server
COPY --from=build /out/driver-client /app/driver-client
COPY --from=build /out/passenger-client /app/passenger-client
COPY --from=build /out/simulator /app/simulator

EXPOSE 3080 3090
CMD ["/app/server"]
