# Build stage
FROM golang:1.26.5 AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -buildvcs=false -o /out/mimo-tts-adapter ./cmd/mimo-tts-adapter

# Runtime stage
FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/mimo-tts-adapter /mimo-tts-adapter
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/mimo-tts-adapter"]
