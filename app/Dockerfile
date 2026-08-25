# ---- build stage ----
FROM golang:1.23-alpine AS build
WORKDIR /src

# Cache module downloads separately from source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Static build so the final image can be scratch-based.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /demoapi .

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /demoapi /demoapi
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/demoapi"]
